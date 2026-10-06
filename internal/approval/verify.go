package approval

import (
	"fmt"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// CodeSelf is AR716: an approval that arrived in the same change as the content
// it approves. An approval in the committed lock is an assertion, not
// authentication, so the control is a second pair of eyes on the change; this
// finding is what lets CI demand one.
const CodeSelf = "AR716"

// SelfApproval is one approval that was added after the base revision for
// content that was itself added or changed after it.
type SelfApproval struct {
	Approval lockfile.Approval
	Ref      string
	// BaseDigest is the digest the base lock pinned ("" when it did not pin the item).
	BaseDigest string
}

// Message is a complete sentence for a finding.
func (s SelfApproval) Message() string {
	if s.BaseDigest == "" {
		return fmt.Sprintf("the approval of %s by %s was added in the same change that introduced %s; content and its approval must not arrive together",
			s.Ref, s.Approval.Reviewer, short(s.Approval.Digest))
	}
	return fmt.Sprintf("the approval of %s by %s was added in the same change that moved it from %s to %s; content and its approval must not arrive together",
		s.Ref, s.Approval.Reviewer, short(s.BaseDigest), short(s.Approval.Digest))
}

// SelfApprovals compares the lock at the base revision with the current lock and
// returns the approvals that are new and whose content is also new or changed:
// the approval of an item whose digest the base lock already pinned at the same
// value is a later review of old content and is fine. It reads pins only, so it
// needs no checkout of the base tree. The result is sorted by reference.
func SelfApprovals(base, cur *lockfile.File) []SelfApproval {
	if cur == nil {
		return nil
	}
	baseDigest := map[string]string{}
	had := map[string]bool{}
	if base != nil {
		for _, s := range SubjectsOf(base, base.Item) {
			baseDigest[s.Key()] = s.Digest
		}
		for _, a := range base.Approval {
			had[recordKey(a)] = true
		}
	}
	var out []SelfApproval
	for _, a := range cur.Approval {
		if had[recordKey(a)] {
			continue
		}
		d, pinned := baseDigest[a.ItemKey()]
		if pinned && d == a.Digest {
			continue
		}
		s := Subject{Kind: a.Kind, Domain: a.Domain, ID: a.ID}
		out = append(out, SelfApproval{Approval: a, Ref: s.Ref(), BaseDigest: d})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func recordKey(a lockfile.Approval) string {
	return a.ItemKey() + "\x00" + a.Digest + "\x00" + NormalizeReviewer(a.Reviewer)
}
