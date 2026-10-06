package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
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
	data, ok := git.ShowFile(top, base, rel)
	if !ok {
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
	base, _, err := baseLockOf(cfg, rev)
	if err != nil {
		return nil, err
	}
	return approval.SelfApprovals(base, lock), nil
}

// verifyBase is `approve --verify-base <rev>`: it prints each approval added
// after rev for content that also changed after it (AR716) and returns exit code
// 2 when there is one. It writes nothing.
func (e *approveEnv) verifyBase(out io.Writer, rev string) (int, error) {
	found, err := selfApprovalsAgainst(e.cfg, e.lock, rev)
	if err != nil {
		return 1, err
	}
	for _, s := range found {
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
	for _, s := range found {
		out = append(out, lint.ApprovalFinding{Code: approval.CodeSelf, Path: lockRel, Message: s.Message()})
	}
	return out
}
