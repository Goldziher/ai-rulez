package lockfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
)

// Assurance levels of an approval, weakest first. AssuranceAsserted is a
// free-form reviewer string backed by review of the lock change itself.
// AssuranceReviewLinked points at a code-review record on a forge that
// `verify --approvals --online` re-checks. AssuranceSigned carries a DSSE
// attestation (Attestation) verified against [[signing.trust]].
const (
	AssuranceAsserted     = "asserted"
	AssuranceReviewLinked = "review-linked"
	AssuranceSigned       = "signed"
)

// AssuranceRank orders the levels; an unknown level ranks below asserted.
func AssuranceRank(level string) int {
	switch level {
	case AssuranceAsserted:
		return 1
	case AssuranceReviewLinked:
		return 2
	case AssuranceSigned:
		return 3
	}
	return 0
}

// Approval records that a reviewer read the content whose digest is Digest and
// accepted it (docs/approvals.md). Approvals sit outside the tree digest: adding
// one changes no pin. They are bound to the digest, so a changed digest leaves the
// record in place but no longer applies to the content.
type Approval struct {
	// Kind is an item kind (rule, skill, hook, settings, ...) or a remote kind
	// (include, skill, source, served); ID and Domain name the item as in Item.
	// For a served skill Domain is the serve view.
	Kind   string `toml:"kind"`
	ID     string `toml:"id"`
	Domain string `toml:"domain,omitempty"`
	// Digest is the exact "sha256:<hex>" digest that was approved.
	Digest   string `toml:"digest"`
	Reviewer string `toml:"reviewer"`
	// Assurance is AssuranceAsserted, AssuranceReviewLinked or AssuranceSigned.
	Assurance string `toml:"assurance"`
	// ApprovedAt is an RFC 3339 UTC time supplied by `approve`; it is stored once and
	// re-emitted verbatim, so rewriting the lock keeps the diff deterministic.
	ApprovedAt string `toml:"approved_at"`
	// Expires is an optional "YYYY-MM-DD" date; the approval no longer applies after it.
	Expires string `toml:"expires,omitempty"`
	Note    string `toml:"note,omitempty"`
	// AcceptedFindings are the scan findings the reviewer saw and accepted.
	AcceptedFindings []string `toml:"accepted_findings,omitempty"`
	// Ref is the review a review-linked approval points at (a pull request review URL).
	Ref string `toml:"ref,omitempty"`
	// Attestation is the "sha256:<hex>" digest of the DSSE bundle of a signed
	// approval, kept under AttestationDir in the configuration directory.
	Attestation string `toml:"attestation,omitempty"`
}

// AttestationDir is where the bundles of signed approvals live, relative to the
// configuration directory.
const AttestationDir = "attestations"

// Deny records a digest that must never be approved or used (docs/approvals.md):
// content known to be malicious, say. A deny entry outlives the content: it
// names the digest, not an item.
type Deny struct {
	// Digest is the "sha256:<hex>" digest to refuse.
	Digest string `toml:"digest"`
	Reason string `toml:"reason,omitempty"`
}

// DenySet maps each denied digest to its reason.
func (f *File) DenySet() map[string]string {
	if f == nil || len(f.Deny) == 0 {
		return nil
	}
	out := make(map[string]string, len(f.Deny))
	for _, d := range f.Deny {
		out[d.Digest] = d.Reason
	}
	return out
}

// SetDeny adds d, or replaces the entry of the same digest.
func (f *File) SetDeny(d Deny) {
	for i := range f.Deny {
		if f.Deny[i].Digest == d.Digest {
			f.Deny[i] = d
			return
		}
	}
	f.Deny = append(f.Deny, d)
}

func sortedDeny(in []Deny) []Deny {
	if len(in) == 0 {
		return nil
	}
	out := append([]Deny(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Digest < out[j].Digest })
	return out
}

// ApprovalsLabel is the domain-separation label of the approvals digest.
const ApprovalsLabel = "ai-rulez/approvals/v1"

// ApprovalsDigest is the digest of the approval set the lock subject commits to
// (docs/lockfile.md): every [[approval]] and [[deny]] record in the order Save
// writes them, each field length-prefixed. It is "" when the lock holds neither,
// so a lock without approvals keeps the subject it always had.
func (f *File) ApprovalsDigest() string {
	if f == nil || (len(f.Approval) == 0 && len(f.Deny) == 0) {
		return ""
	}
	var buf bytes.Buffer
	put := func(parts ...string) {
		for _, p := range parts {
			var n [8]byte
			binary.BigEndian.PutUint64(n[:], uint64(len(p)))
			buf.Write(n[:])
			buf.WriteString(p)
		}
	}
	put(ApprovalsLabel)
	approvals := sortedApprovals(f.Approval)
	for i := range approvals {
		a := &approvals[i]
		put("approval", a.Kind, a.Domain, a.ID, a.Digest, a.Reviewer, a.Assurance, a.ApprovedAt, a.Expires, a.Note,
			strings.Join(a.AcceptedFindings, ","), a.Ref, a.Attestation)
	}
	for _, d := range sortedDeny(f.Deny) {
		put("deny", d.Digest, d.Reason)
	}
	sum := sha256.Sum256(buf.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ItemKey is "kind\x00domain\x00id", the key of Item.Key.
func (a Approval) ItemKey() string { return a.Kind + "\x00" + a.Domain + "\x00" + a.ID }

// SetApproval adds a, or replaces the record with the same kind, domain, id,
// digest and reviewer.
func (f *File) SetApproval(a Approval) {
	for i := range f.Approval {
		b := f.Approval[i]
		if b.ItemKey() == a.ItemKey() && b.Digest == a.Digest && strings.EqualFold(strings.TrimSpace(b.Reviewer), strings.TrimSpace(a.Reviewer)) {
			f.Approval[i] = a
			return
		}
	}
	f.Approval = append(f.Approval, a)
}

func sortedApprovals(in []Approval) []Approval {
	if len(in) == 0 {
		return nil
	}
	out := append([]Approval(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		case a.Domain != b.Domain:
			return a.Domain < b.Domain
		case a.ID != b.ID:
			return a.ID < b.ID
		case a.Digest != b.Digest:
			return a.Digest < b.Digest
		}
		return a.Reviewer < b.Reviewer
	})
	return out
}
