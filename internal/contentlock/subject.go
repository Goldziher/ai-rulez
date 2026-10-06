package contentlock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/samber/oops"
)

// HashVersion is the version of the hashing scheme the lock subject commits to.
// It equals lockfile.Version: the scheme is part of the lock format, so a change
// to either is a new lock version.
const HashVersion = lockfile.Version

// SubjectLabel is the domain-separation label of the lock subject.
const SubjectLabel = "ai-rulez/lock-subject/v1"

// Subject is what a signature over the lock commits to.
type Subject struct {
	// Tree is the lock's top-level "sha256:<hex>" digest, recomputed from its entries.
	Tree string
	// ApprovalsDigest is the digest of the approval and deny records
	// (lockfile.File.ApprovalsDigest); empty while the lock holds none.
	ApprovalsDigest string
	HashVersion     int
	// Scope is the recorded [lock] scope ("all" when the lock records none).
	Scope         string
	OutputsPinned bool
}

// SubjectOf builds the subject of a lock. The tree is recomputed from the
// entries, never read from the file, so a hand-edited pin changes the subject.
func SubjectOf(f *lockfile.File) Subject {
	scope := f.Scope
	if scope == "" {
		scope = config.LockScopeAll
	}
	return Subject{Tree: TreeOf(f), ApprovalsDigest: f.ApprovalsDigest(), HashVersion: HashVersion, Scope: scope, OutputsPinned: f.OutputsPinned}
}

// Digest returns the lock-subject digest:
//
//	sha256( lp("ai-rulez/lock-subject/v1") || lp(tree) || lp(approvals_digest)
//	        || u64(hash_version) || lp(scope) || u8(outputs_pinned) )
func (s Subject) Digest() string {
	var buf bytes.Buffer
	lps(&buf, SubjectLabel)
	lps(&buf, s.Tree)
	lps(&buf, s.ApprovalsDigest)
	u64(&buf, s.HashVersion)
	lps(&buf, s.Scope)
	if s.OutputsPinned {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	sum := sha256.Sum256(buf.Bytes())
	return Algorithm + ":" + hex.EncodeToString(sum[:])
}

// SubjectSchemaVersion versions the JSON statement of `lock --subject --format json`.
const SubjectSchemaVersion = 1

// SubjectStatement is the document a signer signs and a verifier recomputes:
// the lock-subject digest and the fields it commits to. Its encoding is
// deterministic (fixed field order, two-space indent, trailing newline), so two
// runs over the same lock produce the same bytes and `cosign verify-blob` can
// check a signature against a freshly recomputed file.
type SubjectStatement struct {
	SchemaVersion   int    `json:"schema_version"`
	Type            string `json:"type"`
	Subject         string `json:"subject"`
	Tree            string `json:"tree"`
	ApprovalsDigest string `json:"approvals_digest"`
	HashVersion     int    `json:"hash_version"`
	Scope           string `json:"scope"`
	OutputsPinned   bool   `json:"outputs_pinned"`
}

// Statement returns the signable statement for s.
func (s Subject) Statement() SubjectStatement {
	return SubjectStatement{
		SchemaVersion: SubjectSchemaVersion, Type: SubjectLabel, Subject: s.Digest(), Tree: s.Tree,
		ApprovalsDigest: s.ApprovalsDigest, HashVersion: s.HashVersion, Scope: s.Scope, OutputsPinned: s.OutputsPinned,
	}
}

// JSON encodes the statement deterministically.
func (st SubjectStatement) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode lock subject")
	}
	return append(data, '\n'), nil
}

// Text is the one-line human form printed by `lock --subject`.
func (st SubjectStatement) Text() string {
	approvals := st.ApprovalsDigest
	if approvals == "" {
		approvals = "none"
	}
	return fmt.Sprintf("%s  (lock-subject/v1; tree %s, approvals %s, hash_version %d, scope %s, outputs_pinned %t)",
		st.Subject, st.Tree, approvals, st.HashVersion, st.Scope, st.OutputsPinned)
}
