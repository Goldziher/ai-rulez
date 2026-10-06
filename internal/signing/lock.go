package signing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"time"

	"github.com/samber/oops"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// SubjectLock is the subject kind of a lock attestation, the name trust entries
// and [signing] require use.
const SubjectLock = "lock"

// LockSubjectName is the statement subject name of a lock attestation.
const LockSubjectName = "ai-rulez.lock"

// LockItem is one pinned item embedded in a lock predicate (--embed-items).
type LockItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Domain string `json:"domain,omitempty"`
	Digest string `json:"digest"`
}

// LockPredicate is the predicate of a lock attestation: the fields the
// lock-subject digest commits to (docs/lockfile.md), plus provenance claims.
// Everything except the committed fields is informational.
type LockPredicate struct {
	HashVersion     int        `json:"hash_version"`
	Tree            string     `json:"tree"`
	ApprovalsDigest string     `json:"approvals_digest"`
	Scope           string     `json:"scope"`
	OutputsPinned   bool       `json:"outputs_pinned"`
	AIRulezVersion  string     `json:"ai_rulez_version"`
	Repository      string     `json:"repository,omitempty"`
	Ref             string     `json:"ref,omitempty"`
	IssuedAt        time.Time  `json:"issued_at"`
	Items           []LockItem `json:"items,omitempty"`
}

// LockMeta is what SignLock records besides the lock.
type LockMeta struct {
	Version    string
	Repository string
	Ref        string
	Now        time.Time
	// EmbedItems adds the pinned item ids and digests to the predicate. Item ids
	// can be sensitive in a private repository, so it is opt-in.
	EmbedItems bool
}

// lockSubjectOf recomputes the subject of lock and refuses a lock whose stored
// tree differs from its entries.
func lockSubjectOf(lock *lockfile.File) (contentlock.Subject, error) {
	if lock == nil || !lock.HasContentPins() {
		return contentlock.Subject{}, oops.Hint("run `ai-rulez lock` first").Errorf("%s has no content pins to sign", lockfile.FileName)
	}
	subject := contentlock.SubjectOf(lock)
	if lock.Tree != subject.Tree {
		return contentlock.Subject{}, Errorf(CodeSubjectMismatch, "the tree digest of %s does not match its entries (edited by hand?); run `ai-rulez lock`", lockfile.FileName)
	}
	return subject, nil
}

// LockStatement builds the in-toto statement that attests the lock subject.
func LockStatement(lock *lockfile.File, meta LockMeta) (*Statement, error) {
	subject, err := lockSubjectOf(lock)
	if err != nil {
		return nil, err
	}
	pred := LockPredicate{
		HashVersion: subject.HashVersion, Tree: subject.Tree, ApprovalsDigest: subject.ApprovalsDigest,
		Scope: subject.Scope, OutputsPinned: subject.OutputsPinned, AIRulezVersion: meta.Version,
		Repository: meta.Repository, Ref: meta.Ref, IssuedAt: meta.Now.UTC().Truncate(time.Second),
	}
	if meta.EmbedItems {
		for _, it := range lock.Item {
			pred.Items = append(pred.Items, LockItem{Kind: it.Kind, ID: it.ID, Domain: it.Domain, Digest: it.Digest})
		}
	}
	return NewStatement(PredicateLock, []Subject{{Name: LockSubjectName, Digest: map[string]string{"sha256": hexDigest(subject.Digest())}}}, pred)
}

func hexDigest(d string) string { return strings.TrimPrefix(d, contentlock.Algorithm+":") }

// SignLock signs the lock-subject statement of lock and returns the bundle JSON.
func SignLock(ctx context.Context, s Signer, lock *lockfile.File, meta LockMeta) ([]byte, error) {
	st, err := LockStatement(lock, meta)
	if err != nil {
		return nil, err
	}
	return SignStatement(ctx, s, st)
}

// LockPolicy is how a lock attestation is judged.
type LockPolicy struct {
	Verifier Verifier
	Trust    TrustSet
	// MaxAge is the oldest acceptable signature; 0 disables freshness.
	MaxAge time.Duration
	// MinHashVersion rejects an attestation of an older hashing scheme; 0 accepts
	// any version equal to the lock's.
	MinHashVersion int
	// State, when set, is consulted for rollback (AR727).
	State *State
	// Now is the clock for freshness; required (the package reads no ambient clock).
	Now time.Time
}

// LockReport is a verified lock attestation.
type LockReport struct {
	Result    *Result
	Predicate LockPredicate
	// Age is how long ago the signature was made.
	Age time.Duration
	// SigningTime is SignedAt, or the claimed issue time when Weak.
	SigningTime time.Time
}

// VerifyLock verifies a lock attestation against lock under policy, offline. It
// runs, in order: the cryptographic check (AR721, AR725, AR726), the signer
// check (AR722), the subject check (AR724), the minimum hash version (AR724),
// freshness (AR723) and rollback (AR727). It does not update the rollback state:
// call Commit after the caller has accepted the result.
func VerifyLock(data []byte, lock *lockfile.File, p LockPolicy) (*LockReport, error) {
	now := p.Now
	if now.IsZero() {
		return nil, oops.Errorf("LockPolicy.Now is required: the caller supplies the clock")
	}
	res, pred, err := verifyLockBundle(data, lock, p)
	if err != nil {
		return nil, err
	}
	if err := p.Trust.Check(res, SubjectLock, now); err != nil {
		return nil, err
	}
	if err := CheckFresh(res, pred.IssuedAt, p.MaxAge, now); err != nil {
		return nil, err
	}
	at := res.SignedAt
	if at.IsZero() {
		at = pred.IssuedAt
	}
	if p.State != nil && !at.IsZero() {
		if err := p.State.Check(repoKey(pred), at); err != nil {
			return nil, err
		}
	}
	return &LockReport{Result: res, Predicate: pred, Age: now.Sub(at), SigningTime: at}, nil
}

// verifyLockBundle verifies a DSSE bundle (`ai-rulez sign --lock`) or a
// message-signature bundle over the lock-subject statement (`cosign sign-blob`,
// docs/lockfile.md) and checks it covers this lock.
func verifyLockBundle(data []byte, lock *lockfile.File, p LockPolicy) (*Result, LockPredicate, error) {
	if !IsBlobBundle(data) {
		res, err := p.Verifier.Verify(data)
		if err != nil {
			return nil, LockPredicate{}, err
		}
		pred, err := checkLockSubject(res, lock, p.MinHashVersion)
		return res, pred, err
	}
	subject, err := lockSubjectOf(lock)
	if err != nil {
		return nil, LockPredicate{}, err
	}
	artifact, err := subject.Statement().JSON()
	if err != nil {
		return nil, LockPredicate{}, oops.Wrap(err)
	}
	if err := blobCovers(data, artifact); err != nil {
		return nil, LockPredicate{}, err
	}
	if p.MinHashVersion > 0 && subject.HashVersion < p.MinHashVersion {
		return nil, LockPredicate{}, Errorf(CodeSubjectMismatch, "hash_version %d is older than [signing] min_hash_version %d", subject.HashVersion, p.MinHashVersion)
	}
	res, err := p.Verifier.VerifyBlob(data, artifact)
	if err != nil {
		return nil, LockPredicate{}, err
	}
	return res, LockPredicate{HashVersion: subject.HashVersion, Tree: subject.Tree, ApprovalsDigest: subject.ApprovalsDigest, Scope: subject.Scope, OutputsPinned: subject.OutputsPinned}, nil
}

// blobCovers reports AR724 when the digest a message-signature bundle signed is
// not the SHA-256 of artifact, so a lock that changed after signing is told
// apart from a forged signature.
func blobCovers(data, artifact []byte) error {
	b, err := parseBundle(data)
	if err != nil {
		return err
	}
	md := b.GetMessageSignature().GetMessageDigest()
	sum := sha256.Sum256(artifact)
	if md != nil && md.Algorithm == protocommon.HashAlgorithm_SHA2_256 && !bytes.Equal(md.Digest, sum[:]) {
		return Errorf(CodeSubjectMismatch, "%s changed since it was signed: the signed statement differs from the one recomputed from the lock", lockfile.FileName)
	}
	return nil
}

// Commit records the signing time of an accepted report in the rollback state.
func (r *LockReport) Commit(s *State) error {
	if s == nil || r.SigningTime.IsZero() {
		return nil
	}
	return s.Advance(repoKey(r.Predicate), r.SigningTime)
}

func repoKey(p LockPredicate) string {
	if p.Repository == "" {
		return "unknown"
	}
	return p.Repository
}

func checkLockSubject(res *Result, lock *lockfile.File, minHash int) (LockPredicate, error) {
	var pred LockPredicate
	if res.Statement == nil {
		return pred, Errorf(CodeSubjectMismatch, "the signed payload is not an in-toto statement")
	}
	if res.Statement.PredicateType != PredicateLock {
		return pred, Errorf(CodeSubjectMismatch, "the signed statement is a %q attestation, not a lock attestation", res.Statement.PredicateType)
	}
	subject, err := lockSubjectOf(lock)
	if err != nil {
		return pred, err
	}
	if err := res.Statement.RequireSubject("sha256", hexDigest(subject.Digest())); err != nil {
		return pred, Errorf(CodeSubjectMismatch, "%s changed since it was signed (its subject is %s)", lockfile.FileName, subject.Digest())
	}
	if err := res.Statement.DecodePredicate(&pred); err != nil {
		return pred, err
	}
	if pred.HashVersion != subject.HashVersion || pred.Tree != subject.Tree {
		return pred, Errorf(CodeSubjectMismatch, "the predicate says hash_version %d and tree %s; the lock has %d and %s", pred.HashVersion, pred.Tree, subject.HashVersion, subject.Tree)
	}
	if minHash > 0 && pred.HashVersion < minHash {
		return pred, Errorf(CodeSubjectMismatch, "hash_version %d is older than [signing] min_hash_version %d", pred.HashVersion, minHash)
	}
	return pred, nil
}
