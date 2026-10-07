package lockrun

import (
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// CarryApprovals keeps the approvals of the lock being replaced: `lock` re-pins
// content but never forgets who reviewed it. A full refresh drops, with a
// warning to log, the approvals of content that no longer exists (AR715); a
// refresh limited to some sources keeps every record. Records of changed content
// stay, stale, until the content is approved again or `approve --prune` removes
// them.
func CarryApprovals(log logger.Logger, current, next *lockfile.File, full bool) {
	if current != nil {
		next.Deny = current.Deny // a deny entry outlives the content it names
	}
	if current == nil || len(current.Approval) == 0 {
		return
	}
	next.Approval = current.Approval
	if !full {
		return
	}
	subjects := approval.SubjectsOf(next, next.Item)
	orphans := approval.Orphans(current.Approval, subjects)
	if len(orphans) == 0 {
		return
	}
	gone := map[string]bool{}
	for i := range orphans {
		a := &orphans[i]
		gone[a.ItemKey()] = true
		log.Warn("Dropped an approval of content that no longer exists", "code", approval.CodeOrphan, "kind", a.Kind, "id", a.ID, "reviewer", a.Reviewer)
	}
	kept := next.Approval[:0:0]
	for i := range next.Approval {
		a := &next.Approval[i]
		if !gone[a.ItemKey()] {
			kept = append(kept, *a)
		}
	}
	next.Approval = kept
}

// DeniedPinsError is the refusal of `lock` to write a lock that pins content on
// the deny list (AR717): a denied digest can be neither pinned nor approved.
func DeniedPinsError(lock *lockfile.File) error {
	deny := lock.DenySet()
	if len(deny) == 0 {
		return nil
	}
	var refs []string
	for _, s := range approval.SubjectsOf(lock, lock.Item) {
		if reason, denied := deny[s.Digest]; denied {
			refs = append(refs, s.Ref()+ReasonSuffix(reason))
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return oops.Hint("remove or replace the content; `ai-rulez approve --revoke <item> --deny` adds entries").
		Errorf("%s refusing to pin content on the deny list: %s", approval.CodeDenied, strings.Join(refs, "; "))
}

// ReasonSuffix is " (reason)" for a deny reason, "" for none.
func ReasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}
