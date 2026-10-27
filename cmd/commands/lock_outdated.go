package commands

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/pinupdate"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/samber/oops"
)

var (
	lockOutdated       bool
	lockFailOnOutdated bool
	lockOffline        bool
)

// versionSources lists the sources of cfg that use a version constraint (git
// only), limited to kind and names when given.
func versionSources(cfg *config.Config, kind string, names map[string]bool) []versionSrc {
	return pinupdate.VersionSources(cfg, kind, names, GetGitToken())
}

// requireOnline is the --offline refusal of the commands that read remote tags.
func requireOnline(command string) error {
	return pinupdate.RequireOnline(command, lockOffline || cliLockPolicy.Offline)
}

// outdatedAt prints which sources have newer tags than their pins.
func outdatedAt(path string, kind string, names []string) error {
	if err := requireOnline("lock --outdated"); err != nil {
		return fail(err)
	}
	report, err := pinupdate.Outdated(cmdContext(), updateEnv(path), kind, names)
	if err != nil {
		return fail(err)
	}
	if lockFormat == formatJSON {
		err = report.WriteJSON(os.Stdout)
	} else {
		err = report.WriteText(os.Stdout)
	}
	if err != nil {
		return fail(oops.Wrapf(err, "write the report"))
	}
	return outdatedExit(report)
}

// outdatedExit is the verdict of --outdated: 2 for an error finding (a moved tag, an unresolvable constraint)
// and, with --fail-on-outdated, for any allowed update; else 0.
func outdatedExit(r *tagresolve.Report) error {
	if r.Failing() || (lockFailOnOutdated && r.Summary.Updatable > 0) {
		return exitStatus(exitFindings)
	}
	return nil
}
