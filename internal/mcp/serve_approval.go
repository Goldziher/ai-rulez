package mcp

import (
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// approvalNow is the clock approval expiry is judged by: SOURCE_DATE_EPOCH, else the wall clock.
func approvalNow() time.Time { return config.ResolveGenerationTime() }

// admitApproval applies [governance] to one served skill. A skill the policy
// requires approval for gets Approved and Approvers set from the lock's records;
// under [governance] enforce a skill without a valid approval of its served digest
// is refused with the AR71x code of the reason. Without a lock there is nothing
// to approve against (the lock command builds without one), so nothing is checked.
func (a Admission) admitApproval(s *CatalogSkill) *Refusal {
	if a.Lock == nil {
		return nil
	}
	policy := approval.PolicyOf(a.Config)
	if !policy.Active() {
		return nil
	}
	class := approval.ClassServedLocal
	if s.Imported || approval.RemoteServed(s.Source, s.Ref, s.Commit) {
		class = approval.ClassRemote
	}
	res := policy.Evaluate(a.Lock.Approval, approval.Subject{
		Kind: approval.KindServed, Domain: a.View, ID: s.Name, Digest: s.LockDigest, Class: class,
	}, approvalNow())
	if !res.Required {
		return nil
	}
	s.Approved, s.Approvers = res.Status == approval.StatusOK, res.Reviewers
	if s.Approved || !policy.Enforce {
		return nil
	}
	return &Refusal{Name: s.Name, Code: approval.CodeOf(res.Status), Reason: "[governance] enforce is on: " + res.Message()}
}
