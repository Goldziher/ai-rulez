package mcp

import (
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// approvalNow is the clock approval expiry is judged by: always the wall clock.
// SOURCE_DATE_EPOCH only stamps generated files; letting it decide expiry would
// let an environment variable revive an expired approval.
func approvalNow() time.Time { return ambient.Clock(nil).Now() }

// now is the time the admission judges approvals and signatures at: Now, or
// the wall clock.
func (a Admission) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return approvalNow()
}

// admitApproval applies [governance] to one served skill. A skill the policy
// requires approval for gets Approved and Approvers set from the lock's records;
// under [governance] enforce a skill without a valid approval of its served digest
// is refused with the AR71x code of the reason. Under enforce a missing lock
// refuses every skill the policy selects: with no lock there is no record to
// approve against, and that must not read as "approved".
func (a Admission) admitApproval(s *CatalogSkill) *Refusal {
	policy := approval.PolicyOf(a.Config)
	if a.Lock != nil {
		policy = policy.WithLock(a.Lock)
	}
	if reason, denied := policy.Deny[s.LockDigest]; denied && s.LockDigest != "" {
		// A denied digest is never served, whether or not [governance] selects the skill.
		return &Refusal{Name: s.Name, Code: approval.CodeDenied, Reason: approval.Result{Subject: approval.Subject{Kind: approval.KindServed, ID: s.Name, Digest: s.LockDigest}, Status: approval.StatusDenied, Detail: reason}.Message()}
	}
	if !policy.Active() {
		return nil
	}
	subject := approval.Subject{
		Kind: approval.KindServed, Domain: a.View, ID: s.Name, Digest: s.LockDigest,
		Class: approval.ServedClass(s.Source, s.Ref, s.Commit),
	}
	if a.Lock == nil {
		if policy.Enforce && !a.Pinning && policy.Requires(subject) {
			return &Refusal{Name: s.Name, Code: approval.CodeMissing, Reason: "[governance] enforce is on and there is no " + lockfile.FileName + " to hold approvals: " + approval.Result{Subject: subject, Status: approval.StatusMissing}.Message()}
		}
		return nil
	}
	res := policy.Evaluate(a.Lock.Approval, subject, a.now())
	if !res.Required {
		return nil
	}
	s.Approved, s.Approvers = res.Status == approval.StatusOK, res.Reviewers
	if s.Approved || !policy.Enforce {
		return nil
	}
	return &Refusal{Name: s.Name, Code: approval.CodeOf(res.Status), Reason: "[governance] enforce is on: " + res.Message()}
}
