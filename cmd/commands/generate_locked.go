package commands

import (
	"errors"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/samber/oops"
)

// errLockedSourceDrift marks a `generate --locked` refusal because authored
// content no longer matches ai-rulez.lock.
var errLockedSourceDrift = errors.New("authored content differs from " + "ai-rulez.lock")

// lockMissingError is the refusal of `generate --locked` or `--frozen` without a
// lock. It is lock drift (exit 2) but not a content difference, so it does not
// say "authored content differs" or suggest `lock --diff`, which needs a lock.
type lockMissingError struct{}

func (lockMissingError) Error() string        { return lockMissingLine }
func (lockMissingError) Is(target error) bool { return target == errLockedSourceDrift }

// errLockedSignature marks a `generate --locked` refusal because [signing]
// require is not met by the lock attestation (AR720 to AR727). Re-locking does not
// fix it: it changes the lock and invalidates the signature.
var errLockedSignature = errors.New("the lock attestation does not satisfy [signing] require")

// errLockedApproval marks a `generate --locked` refusal because content the
// lock pins lacks an approval [governance] requires, with nothing drifted.
var errLockedApproval = errors.New("required approvals are missing")

// splitApprovalLines separates the approval lines of a locked-content check (an
// approval change, or the policy's own AR710 lock problem) from the drift lines.
func splitApprovalLines(lines []string) (approvals, drift []string) {
	for _, l := range lines {
		if strings.HasPrefix(l, contentlock.ScopeApproval+": ") || strings.HasPrefix(l, approval.CodeMissing+" ") {
			approvals = append(approvals, l)
		} else {
			drift = append(drift, l)
		}
	}
	return approvals, drift
}

// isLockedDrift reports whether err is a lock refusal that exits with exitDrift:
// authored content or a remote disagreeing with the lock, or a missing or invalid
// attestation.
func isLockedDrift(err error) bool {
	return errors.Is(err, errLockedSourceDrift) || errors.Is(err, errLockedSignature) || errors.Is(err, errLockedApproval) ||
		errors.Is(err, config.ErrLockViolation)
}

// lockedDriftError is the failure of a run whose authored content no longer
// matches the lock (exit 2, drift) or that failed another way (exit 1); nil when
// err is nil.
func lockedDriftError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errLockedSourceDrift) || errors.Is(err, errLockedSignature) || errors.Is(err, errLockedApproval) {
		return failWithCode(exitDrift, err)
	}
	return failWithCode(exitFailure, err)
}

// enforceLockedContent is the content half of --locked and --frozen: when the
// lock pins authored content, every source must still match it. generate never
// writes the lock.
func enforceLockedContent(cfg *config.Config) error {
	return enforceLockedContentFor(cfg, false)
}

// enforceLockedContentFor is enforceLockedContent for a run that may also be
// `generate --check`, which verifies the content whenever the lock is enforced,
// not only under --locked or --frozen.
func enforceLockedContentFor(cfg *config.Config, check bool) error {
	if !generateLocked && !generateFrozen && (!check || !cfg.LockEnforced()) {
		return nil
	}
	lines, err := verifyLockedSources(cfg)
	if err != nil {
		return err
	}
	signLines, err := signingRequiredLines(cfg)
	if err != nil {
		return err
	}
	approvals, drift := splitApprovalLines(lines)
	switch {
	case len(lines) == 0 && len(signLines) == 0:
		return nil
	case len(lines) == 1 && lines[0] == lockMissingLine && len(signLines) == 0:
		return oops.Hint("Create it with `ai-rulez lock`").Wrap(lockMissingError{})
	case len(drift) == 0 && len(approvals) > 0:
		// Nothing drifted: running `lock` would not help, approving does.
		return oops.Hint("Review each item named above, then run `ai-rulez approve <item>`").
			Wrapf(errLockedApproval, "%s lacks approvals [governance] requires:\n  %s", "ai-rulez.lock", strings.Join(append(approvals, signLines...), "\n  "))
	case len(lines) == 0:
		return oops.Hint("Sign the current lock with `ai-rulez sign --lock` (running `ai-rulez lock` again would invalidate the signature)").
			Wrapf(errLockedSignature, "%s lacks the attestation [signing] require asks for:\n  %s", "ai-rulez.lock", strings.Join(signLines, "\n  "))
	}
	lines = append(lines, signLines...)
	return oops.Hint("Review the change with `ai-rulez lock --diff`, then run `ai-rulez lock` to accept it (and `ai-rulez sign --lock` when [signing] require is set)").
		Wrapf(errLockedSourceDrift, "%s does not match the sources:\n  %s", "ai-rulez.lock", strings.Join(lines, "\n  "))
}

// lockDriftError reports whether a failed `generate --locked` or `--frozen` was
// the lock disagreeing with the sources (content drift, or a remote the lock
// does not cover), which exits with exitDrift rather than 1.
func lockDriftError(err error) bool {
	if errors.Is(err, errMovedTag) {
		return true // --verify-tags: the remote disagrees with the lock, drift whatever the flags
	}
	if !generateLocked && !generateFrozen {
		return false
	}
	return isLockedDrift(err)
}
