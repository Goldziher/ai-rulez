package commands

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// baseLockOf reads the lock as it was at the merge base of rev and HEAD. A base
// without a lock is no lock (nil): everything approved since is new. An unknown
// rev, or a configuration outside a git work tree, is an error, so the check
// never passes by comparing against nothing.
func baseLockOf(cfg *config.Config, rev string) (*lockfile.File, string, error) {
	rev = strings.TrimSpace(rev)
	git := gitutil.Git{}
	top := git.TopLevel(cfg.ConfigDir)
	if top == "" {
		return nil, "", oops.Errorf("%s is not inside a git work tree: cannot read the lock at %q", cfg.ConfigDir, rev)
	}
	base, err := git.MergeBase(top, rev)
	if err != nil {
		return nil, "", err //nolint:wrapcheck // already contextual
	}
	rel := gitutil.RepoRelative(top, lockfile.Path(cfg.ConfigDir))
	if rel == "" {
		return nil, "", oops.Errorf("%s is outside the git work tree", lockfile.Path(cfg.ConfigDir))
	}
	data, found, err := workspace.ReadFileAt(cmdContext(), top, base, rel, nil)
	if err != nil {
		return nil, base, oops.With("rev", base).Wrapf(err, "read %s at the base revision", lockfile.FileName)
	}
	if !found {
		return nil, base, nil
	}
	lock, err := lockfile.Parse(data)
	if err != nil {
		return nil, base, oops.With("rev", base).Wrapf(err, "read %s at the base revision", lockfile.FileName)
	}
	return lock, base, nil
}

// selfApprovalsAgainst lists the approvals of lock that arrived together with
// the content they approve, relative to rev.
func selfApprovalsAgainst(cfg *config.Config, lock *lockfile.File, rev string) ([]approval.SelfApproval, error) {
	base, mergeBase, err := baseLockOf(cfg, rev)
	if err != nil {
		return nil, err
	}
	found := approval.SelfApprovals(base, lock)
	changes, err := approval.OwnershipChanges(cfg, mergeBase)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	for _, note := range changes {
		found = append(found, approval.SelfApproval{Note: note})
	}
	found = append(found, unownedAtBase(cfg, lock, base, found, mergeBase)...)
	authored, err := authorSelfApprovals(cfg, lock, rev)
	if err != nil {
		return nil, err
	}
	return append(found, authored...), nil
}

// unownedAtBase flags the approvals added in the range (absent from base) whose reviewer does not
// own the item according to CODEOWNERS as it was at the merge base, so editing
// CODEOWNERS in the change cannot make an approval authorized. It applies only
// with approvers_from set.
func unownedAtBase(cfg *config.Config, lock, base *lockfile.File, added []approval.SelfApproval, mergeBase string) []approval.SelfApproval {
	if lock == nil || cfg.Governance == nil || cfg.Governance.ApproversFrom == "" || mergeBase == "" {
		return nil
	}
	policy := approval.PolicyOf(cfg)
	owners := approval.LoadOwnerSetAt(cfg.BaseDir, cfg.ConfigDir, cfg.Governance.ApproversFrom, mergeBase)
	subjects := map[string]approval.Subject{}
	for _, s := range approval.SubjectsOf(lock, lock.Item) {
		subjects[s.Key()] = s
	}
	already := map[string]bool{}
	for i := range added {
		s := &added[i]
		already[s.Ref+"\x00"+approval.NormalizeReviewer(s.Approval.Reviewer)] = true
	}
	had := map[string]bool{}
	if base != nil {
		for i := range base.Approval {
			a := &base.Approval[i]
			had[a.ItemKey()+"\x00"+a.Digest+"\x00"+approval.NormalizeReviewer(a.Reviewer)] = true
		}
	}
	var out []approval.SelfApproval
	for i := range lock.Approval {
		a := &lock.Approval[i]
		s, ok := subjects[a.ItemKey()]
		if !ok || s.Digest != a.Digest || had[a.ItemKey()+"\x00"+a.Digest+"\x00"+approval.NormalizeReviewer(a.Reviewer)] {
			continue
		}
		if o, covered := owners.OwnersOf(s); covered && policy.Teams.Matches(o, a.Reviewer) {
			continue
		}
		if already[s.Ref()+"\x00"+approval.NormalizeReviewer(a.Reviewer)] {
			continue
		}
		note := fmt.Sprintf("the approval of %s by %s is not by an owner of it in CODEOWNERS at %s (the base of the range)", s.Ref(), a.Reviewer, mergeBase[:min(len(mergeBase), 12)])
		out = append(out, approval.SelfApproval{Note: note})
	}
	return out
}

// authorSelfApprovals is forbid_self_approval: the approvals of the current
// content whose reviewer authored a commit that touched the item since the
// merge base of rev and HEAD. It is best effort and heuristic: it matches
// commit author emails (and GitHub noreply addresses) against the reviewer
// string, so a reviewer recorded under another name is not caught.
func authorSelfApprovals(cfg *config.Config, lock *lockfile.File, rev string) ([]approval.SelfApproval, error) {
	policy := approval.PolicyOf(cfg)
	if lock == nil || len(lock.Approval) == 0 || !policy.ForbidSelf {
		return nil, nil
	}
	g, err := newApproveGit(cmdContext(), cfg)
	if err != nil {
		return nil, err
	}
	subjects := map[string]approval.Subject{}
	for _, s := range approval.SubjectsOf(lock, lock.Item) {
		subjects[s.Key()] = s
	}
	cache := map[string][]string{}
	var out []approval.SelfApproval
	for i := range lock.Approval {
		a := lock.Approval[i]
		s, ok := subjects[a.ItemKey()]
		if !ok || s.Digest != a.Digest {
			continue
		}
		who := policy.IdentityOf(signerOf(policy, a, s))
		if a.Assurance == lockfile.AssuranceSigned && strings.HasPrefix(approval.Identity(who), "key:") {
			note := fmt.Sprintf("the signed approval of %s is by %s, a key that names no author: [governance] forbid_self_approval cannot tell whether the signer wrote the change; sign with a keyless identity", s.Ref(), who)
			out = append(out, approval.SelfApproval{Approval: a, Ref: s.Ref(), Note: note})
			continue
		}
		emails, seen := cache[s.Key()]
		if !seen {
			if emails, err = g.authors(rev, g.subjectPaths(s)); err != nil {
				return nil, err
			}
			cache[s.Key()] = emails
		}
		for _, email := range emails {
			if approval.AuthorIs(who, email) {
				out = append(out, approval.SelfApproval{Approval: a, Ref: s.Ref(), Author: email})
				break
			}
		}
	}
	return out, nil
}

// signerOf is the identity a record stands for: the verified signer of a signed
// approval (the record's own reviewer string is not trusted), else the reviewer.
func signerOf(policy approval.Policy, a lockfile.Approval, s approval.Subject) string {
	if a.Assurance == lockfile.AssuranceSigned {
		if who, err := policy.VerifyAssurance(a, s, time.Now()); err == nil && who != "" {
			return who
		}
	}
	return a.Reviewer
}

// verifyBase is `approve --verify-base <rev>`: it prints each approval added
// after rev for content that also changed after it (AR716) and returns exit code
// 2 when there is one. It writes nothing.
func (e *approveEnv) verifyBase(out io.Writer, rev string) (int, error) {
	found, err := selfApprovalsAgainst(e.cfg, e.lock, rev)
	if err != nil {
		return 1, err
	}
	for i := range found {
		s := &found[i]
		if _, werr := fmt.Fprintf(out, "%s %s\n", approval.CodeSelf, safeText(s.Message())); werr != nil {
			return 1, oops.Wrapf(werr, "write output")
		}
	}
	if len(found) == 0 {
		_, err = fmt.Fprintf(out, "no approval was added together with the content it approves (compared with %s)\n", safeText(rev))
		return 0, err
	}
	return 2, nil
}

// selfApprovalFindings are the AR716 findings of `validate --strict --approvals-base`.
// A base that cannot be read is a finding, never a silent pass.
func selfApprovalFindings(cfg *config.Config, lock *lockfile.File, rev, lockRel string) []lint.ApprovalFinding {
	found, err := selfApprovalsAgainst(cfg, lock, rev)
	if err != nil {
		return []lint.ApprovalFinding{{Code: approval.CodeSelf, Path: lockRel, Message: fmt.Sprintf("cannot compare approvals with %q: %v", rev, err)}}
	}
	var out []lint.ApprovalFinding
	for i := range found {
		s := &found[i]
		out = append(out, lint.ApprovalFinding{Code: approval.CodeSelf, Path: lockRel, Message: s.Message()})
	}
	return out
}
