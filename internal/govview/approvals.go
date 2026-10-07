package govview

import (
	"context"
	"fmt"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// ApprovalNow is the clock approval expiry is judged by: the wall clock, never
// SOURCE_DATE_EPOCH (which only stamps generated files and must not revive an
// expired approval).
func ApprovalNow() time.Time { return ambient.Clock(nil).Now() }

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
	return EvaluateApprovalsContext(context.Background(), cfg, lock, items, now)
}

// EvaluateApprovalsContext is EvaluateApprovals with the policy's CODEOWNERS
// lookup bounded by ctx.
func EvaluateApprovalsContext(ctx context.Context, cfg *config.Config, lock *lockfile.File, items []lockfile.Item, now time.Time) *ApprovalState {
	st := &ApprovalState{Policy: approval.PolicyOfContext(ctx, cfg)}
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
	return ApprovalChangesContext(context.Background(), cfg, lock, items, now)
}

// ApprovalChangesContext is ApprovalChanges with the policy's CODEOWNERS lookup
// bounded by ctx.
func ApprovalChangesContext(ctx context.Context, cfg *config.Config, lock *lockfile.File, items []lockfile.Item, now time.Time) (changes []contentlock.Change, notes []string) {
	if msg := approval.PolicyOfContext(ctx, cfg).LockProblem(lock); msg != "" {
		return []contentlock.Change{{Scope: contentlock.ScopeApproval, Change: contentlock.Added, Detail: msg}}, nil
	}
	st := EvaluateApprovalsContext(ctx, cfg, lock, items, now)
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
	lock   *lockfile.File
	recs   []lockfile.Approval
	now    time.Time
}

// newApprovalIndex reads the approval records of cfg's lock. A missing or
// unreadable lock has none, so every item's approval is null: the catalog is a
// view, and `lock --check` and `validate --strict` report an unreadable lock.
func newApprovalIndex(ctx context.Context, cfg *config.Config) *approvalIndex {
	idx := &approvalIndex{policy: approval.PolicyOfContext(ctx, cfg), now: ApprovalNow()}
	if lock, err := lockfile.Load(cfg.ConfigDir); err == nil && lock != nil {
		idx.lock, idx.recs = lock, lock.Approval
	}
	return idx
}

// forItem is the approval state of a catalog item. A skill is also served, and
// the served skill is a subject of its own (kind served, same name): when the
// policy selects that, the item carries its state too, and the worst of the
// subjects that apply is the one reported.
func (x *approvalIndex) forItem(kind, domain, id, digest string) *ItemApproval {
	if digest == "" {
		return nil
	}
	results := []approval.Result{x.policy.Evaluate(x.recs, approval.Subject{Kind: kind, Domain: domain, ID: id, Digest: digest, Class: approval.ClassLocal}, x.now)}
	if kind == "skill" && x.lock != nil {
		for i := range x.lock.Served {
			if e := &x.lock.Served[i]; e.Name == id {
				results = append(results, x.policy.Evaluate(x.recs, approval.Subject{
					Kind: approval.KindServed, Domain: e.View, ID: e.Name, Digest: e.Digest, Class: approval.ServedClass(e.Source, e.Ref, e.Commit),
				}, x.now))
			}
		}
	}
	r := pickApproval(results)
	if !r.Required && len(r.Reviewers) == 0 {
		return nil
	}
	out := &ItemApproval{Required: r.Required, Status: r.Status, Reviewers: r.Reviewers, Expires: r.Expires}
	if out.Reviewers == nil {
		out.Reviewers = []string{}
	}
	if len(r.Reviewers) > 0 {
		out.Assurance = r.Assurance
	}
	return out
}

// pickApproval chooses the result that describes a group of subjects: the first
// required one that fails, else the first required one, else the first result.
func pickApproval(results []approval.Result) approval.Result {
	for i := range results {
		if results[i].Failing() {
			return results[i]
		}
	}
	for i := range results {
		if results[i].Required {
			return results[i]
		}
	}
	return results[0]
}
