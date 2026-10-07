package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// carryApprovals keeps the approvals of the lock being replaced (see
// lockrun.CarryApprovals).
func carryApprovals(current, next *lockfile.File, full bool) {
	lockrun.CarryApprovals(logger.Std(), current, next, full)
}

// approvalFindingsFor returns the AR710 to AR715 findings for strict validation,
// and AR716 when --approvals-base names a revision.
// It reports nothing without a lock, and nothing for a project that sets no
// [governance] policy and has no approval records. A policy that cannot be
// evaluated is itself a finding: enforcement never fails open.
func approvalFindingsFor(ctx context.Context, cfg *config.Config) []lint.ApprovalFinding {
	out := approvalStatusFindings(ctx, cfg)
	if validateApprovalsBase == "" {
		return out
	}
	lockRel := filepath.ToSlash(filepath.Join(relToBase(cfg, cfg.ConfigDir), lockfile.FileName))
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return append(out, lint.ApprovalFinding{Code: approval.CodeSelf, Path: lockRel, Message: fmt.Sprintf("cannot compare approvals with %q: %v", validateApprovalsBase, err)})
	}
	return append(out, selfApprovalFindings(ctx, cfg, lock, validateApprovalsBase, lockRel)...)
}

func approvalStatusFindings(ctx context.Context, cfg *config.Config) []lint.ApprovalFinding {
	policy := approval.PolicyOfContext(ctx, cfg)
	lock, err := lockfile.Load(cfg.ConfigDir)
	lockRel := filepath.ToSlash(filepath.Join(relToBase(cfg, cfg.ConfigDir), lockfile.FileName))
	unverifiable := func(err error) []lint.ApprovalFinding {
		return []lint.ApprovalFinding{{Code: approval.CodeMissing, Path: lockRel, Message: fmt.Sprintf("cannot verify approvals: %v", err)}}
	}
	if err != nil {
		if policy.Active() {
			return unverifiable(err)
		}
		return nil
	}
	if msg := policy.LockProblem(lock); msg != "" {
		return []lint.ApprovalFinding{{Code: approval.CodeMissing, Path: lockRel, Message: msg}}
	}
	if lock == nil && policy.Active() {
		return []lint.ApprovalFinding{{Code: approval.CodeMissing, Path: lockRel, Message: "[governance] require_approval is set and there is no " + lockfile.FileName + " to hold approvals; run `ai-rulez lock`, then `ai-rulez approve`"}}
	}
	if lock == nil || (!policy.Active() && len(lock.Approval) == 0 && len(lock.Deny) == 0) {
		return nil
	}
	shared, err := sharedConfig(ctx, cfg)
	if err == nil {
		var snapItems []lockfile.Item
		if lock.HasContentPins() || len(lock.Approval) > 0 {
			snap, snapErr := lockSnapshot(ctx, shared, lock.Profile, true)
			if snapErr != nil {
				return unverifiable(snapErr)
			}
			snapItems = snap.Items
		}
		st := govview.EvaluateApprovalsContext(ctx, shared, lock, snapItems, govview.ApprovalNow())
		var out []lint.ApprovalFinding
		failures := approval.Failures(st.Results)
		for i := range failures {
			r := &failures[i]
			out = append(out, lint.ApprovalFinding{Code: approval.CodeOf(r.Status), Path: lockRel, Message: r.Message()})
		}
		for i := range st.Orphans {
			a := &st.Orphans[i]
			out = append(out, lint.ApprovalFinding{Code: approval.CodeOrphan, Path: lockRel,
				Message: fmt.Sprintf("the approval by %s names %s:%s, which no longer exists; run `ai-rulez approve --prune`", a.Reviewer, a.Kind, a.ID)})
		}
		return append(out, unresolvedFindings(st.Policy, st.Subjects, lockRel)...)
	}
	if policy.Active() {
		return unverifiable(err)
	}
	return nil
}

// approvalLockedLines are the lines `generate --locked` adds to the drift it
// reports: content that needs approval under an enforced [governance] policy and
// lacks it. items are the current digests (what verifyLockedSources computed).
func approvalLockedLines(cfg *config.Config, lock *lockfile.File, items []lockfile.Item) []string {
	changes, _ := govview.ApprovalChanges(cfg, lock, items, govview.ApprovalNow())
	lines := make([]string, 0, len(changes))
	for i := range changes {
		lines = append(lines, changes[i].Line())
	}
	return lines
}

// unresolvedFindings is AR719: an approvers_from that cannot be read, and teams
// among the approvers or CODEOWNERS owners that have no member list. Both make
// authorization fail closed, so they are reported instead of left as a quiet
// "unauthorized". Nothing is reported unless the policy requires approvals.
func unresolvedFindings(policy approval.Policy, subjects []approval.Subject, lockRel string) []lint.ApprovalFinding {
	if !policy.Active() {
		return nil
	}
	var out []lint.ApprovalFinding
	if problem := policy.OwnersProblem(); problem != "" {
		out = append(out, lint.ApprovalFinding{Code: approval.CodeUnresolved, Path: lockRel, Message: problem})
	}
	if teams := policy.UnresolvedTeams(subjects); len(teams) > 0 {
		out = append(out, lint.ApprovalFinding{Code: approval.CodeUnresolved, Path: lockRel,
			Message: fmt.Sprintf("no members are known for %s: list them in [governance.teams], or resolve them with `ai-rulez approve --resolve-teams`; until then nobody is authorized through them", strings.Join(teams, ", "))})
	}
	return out
}

func reasonSuffix(reason string) string { return lockrun.ReasonSuffix(reason) }
