package govview

import (
	"fmt"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// ApprovalNow is the clock approvals are evaluated against: SOURCE_DATE_EPOCH,
// else the wall clock.
func ApprovalNow() time.Time { return config.ResolveGenerationTime() }

// ApprovalState is the approval status of every pinned subject of a project.
type ApprovalState struct {
	Policy   approval.Policy
	Subjects []approval.Subject
	Results  []approval.Result
	Orphans  []lockfile.Approval
}

// EvaluateApprovals evaluates the approvals of lock against the governance
// policy of cfg. items are the current content digests (a fresh snapshot of the
// working tree); the remote entries come from the lock, which load-time
// verification already compared with the fetched trees. A nil lock has no
// approvals and no subjects.
func EvaluateApprovals(cfg *config.Config, lock *lockfile.File, items []lockfile.Item, now time.Time) *ApprovalState {
	st := &ApprovalState{Policy: approval.PolicyOf(cfg)}
	if lock == nil {
		return st
	}
	st.Subjects = approval.SubjectsOf(lock, items)
	st.Results = st.Policy.EvaluateAll(lock.Approval, st.Subjects, now)
	st.Orphans = approval.Orphans(lock.Approval, st.Subjects)
	return st
}

// ApprovalChanges reports the content that needs approval and lacks it, as
// approval-scope changes of a lock diff. It reports nothing unless [governance]
// enforce is set: a policy without enforcement is reported by `validate --strict`
// and `approve --list`, and mentioned in the diff notes.
func ApprovalChanges(cfg *config.Config, lock *lockfile.File, items []lockfile.Item, now time.Time) (changes []contentlock.Change, notes []string) {
	st := EvaluateApprovals(cfg, lock, items, now)
	failing := approval.Failures(st.Results)
	if len(failing) > 0 && !st.Policy.Enforce {
		notes = append(notes, fmt.Sprintf("%d item(s) need approval; see `ai-rulez approve --list` ([governance] enforce is not set)", len(failing)))
	}
	if !st.Policy.Enforce {
		return nil, notes
	}
	for i := range failing {
		r := &failing[i]
		change := contentlock.Added
		if r.Status != approval.StatusMissing {
			change = contentlock.Changed
		}
		changes = append(changes, contentlock.Change{
			Scope: contentlock.ScopeApproval, Change: change, Kind: r.Kind, ID: r.ID, Domain: r.Domain, Path: r.Path,
			Old: r.ApprovedDigest, New: r.Digest, Detail: approval.CodeOf(r.Status) + " " + r.Status + ": " + r.Message(),
		})
	}
	return changes, notes
}

// approvalIndex answers the approval status of catalog items.
type approvalIndex struct {
	policy approval.Policy
	recs   []lockfile.Approval
	now    time.Time
}

// newApprovalIndex reads the approval records of cfg's lock. A missing or
// unreadable lock has none, so every item's approval is null: the catalog is a
// view, and `lock --check` and `validate --strict` report an unreadable lock.
func newApprovalIndex(cfg *config.Config) *approvalIndex {
	idx := &approvalIndex{policy: approval.PolicyOf(cfg), now: ApprovalNow()}
	if lock, err := lockfile.Load(cfg.ConfigDir); err == nil && lock != nil {
		idx.recs = lock.Approval
	}
	return idx
}

func (x *approvalIndex) forItem(kind, domain, id, digest string) *ItemApproval {
	if digest == "" {
		return nil
	}
	r := x.policy.Evaluate(x.recs, approval.Subject{Kind: kind, Domain: domain, ID: id, Digest: digest, Class: approval.ClassLocal}, x.now)
	if !r.Required && len(r.Reviewers) == 0 {
		return nil
	}
	out := &ItemApproval{Required: r.Required, Status: r.Status, Reviewers: r.Reviewers, Expires: r.Expires}
	if out.Reviewers == nil {
		out.Reviewers = []string{}
	}
	if len(r.Reviewers) > 0 {
		out.Assurance = lockfile.AssuranceAsserted
	}
	return out
}
