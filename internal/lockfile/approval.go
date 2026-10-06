package lockfile

import (
	"sort"
	"strings"
)

// Assurance levels of an approval. Only AssuranceAsserted is written today: a
// free-form reviewer string backed by review of the lock change itself.
const AssuranceAsserted = "asserted"

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
	// Assurance is AssuranceAsserted.
	Assurance string `toml:"assurance"`
	// ApprovedAt is an RFC 3339 UTC time supplied by `approve`; it is stored once and
	// re-emitted verbatim, so rewriting the lock keeps the diff deterministic.
	ApprovedAt string `toml:"approved_at"`
	// Expires is an optional "YYYY-MM-DD" date; the approval no longer applies after it.
	Expires string `toml:"expires,omitempty"`
	Note    string `toml:"note,omitempty"`
	// AcceptedFindings are the scan findings the reviewer saw and accepted.
	AcceptedFindings []string `toml:"accepted_findings,omitempty"`
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
