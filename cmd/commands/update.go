package commands

import (
	"context"
	"errors"
	"slices"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/pinupdate"
)

// UpdateSchemaVersion versions the JSON of `update --format json`.
const UpdateSchemaVersion = pinupdate.SchemaVersion

// Literals shared by the update, skill, plugin and marketplace commands.
const (
	useUpdate = "update [name...]"
	keyRef    = "ref"
)

var (
	updateDryRun         bool
	updateAllowDowngrade bool
	updateAcceptMoved    bool
	updateKind           string
	updateFormat         string
	updateOffline        bool
	updateMajor          bool
	updateWriteConfig    bool
	updateAcceptFindings bool
)

// UpdateCmd moves the pins of sources that use a version constraint.
var UpdateCmd = &cobra.Command{
	Use:   useUpdate,
	Short: "Move the pins of sources that use a version constraint to the newest allowed tag",
	Long: `Move the lock entries of remote includes, installed skills, skill sources,
installed marketplaces and installed plugins that ask for a version range (version = ^1.2) to the newest tag the range
allows, then record the tag, its commit and the tree digest in ai-rulez.lock.
Names limit the update to those sources; --kind limits it to include, skill,
source, marketplace or plugin. Only the lock changes: config.toml is edited only by --major --write-config.

Nothing else moves a range pin: "generate" never resolves a range and
"lock" keeps a pin that still satisfies its constraint. "update" is the one
deliberate step, and it prints what changes first:

  ai-rulez lock --outdated          which sources have newer tags
  ai-rulez update --dry-run         what update would change, writes nothing
  ai-rulez update shared            move one source
  ai-rulez update                   move every source that has an update

A source may hold tags back with min_release_age ("7d"): update takes the
newest tag that is old enough and says which newer tags it held back (AR733).
Before a pin is written, the new tree gets the security scan (AR001-AR009) and
a pin whose scan has error findings is refused unless --accept-findings.

Major versions: --major lists the sources that have a newer major version than
their constraint allows and the constraint that would take it (version = "^2.0").
--write-config applies that: it rewrites only the version line of that source in
config.toml (comments and layout stay) and moves the pin; without it nothing is
written. Other sources are left alone by --major.

Defenses:
  - A tag that moved since it was pinned (AR732) is refused. Review the new
    commit, then --accept-moved-tag re-pins that tag.
  - update never selects a tag below the pinned one (a truncated tag list can
    roll a pin back) unless --allow-downgrade.
  - A constraint no tag satisfies (AR730) is an error.

Plain "ref" sources are not touched: "ai-rulez lock <name>" and
"ai-rulez skill update" re-resolve those (a branch follows its tip). They keep
a range pin as it is; use "update" to move it.

Exit codes: 0 done (or nothing to do); 1 the command could not run (network,
tool error); 2 a source was refused (AR732, AR730, AR731, scan findings) and
nothing was written.`,
	RunE: func(_ *cobra.Command, args []string) error {
		return runUpdate(args)
	},
}

func init() {
	f := UpdateCmd.Flags()
	specDryRun.Bool(f, &updateDryRun, "Show what would change and write nothing")
	f.BoolVar(&updateAllowDowngrade, "allow-downgrade", false, "Allow a tag with lower precedence than the pinned one")
	f.BoolVar(&updateMajor, "major", false, "Handle only sources that have a newer major version: print the constraint that would take it")
	f.BoolVar(&updateWriteConfig, "write-config", false, "With --major, rewrite the version line of those sources in config.toml and move their pins")
	f.BoolVar(&updateAcceptFindings, "accept-findings", false, "Write a pin although the security scan of the new tree has error findings (review them first)")
	f.BoolVar(&updateAcceptMoved, "accept-moved-tag", false, "Re-pin a tag that now points to another commit (AR732) after you reviewed it")
	f.StringVar(&updateKind, "kind", "", "Limit the update to include, skill, source, marketplace or plugin")
	addFormatFlag(f, &updateFormat, "", formatText, formatText, formatJSON)
	f.BoolVar(&updateOffline, "offline", false, "Refuse to run: update reads the remote's tags")
	RootCmd.AddCommand(UpdateCmd)
}

// The report types live in internal/pinupdate, which `lock --outdated`, `update`
// and the MCP tools share.
type (
	updateItem   = pinupdate.Item
	updateReport = pinupdate.Report
	majorItem    = pinupdate.MajorItem
	versionSrc   = pinupdate.Source
)

func runUpdate(names []string) error {
	if updateKind != "" && !slices.Contains(updatableKinds(), updateKind) {
		return fail(oops.Errorf("unknown --kind %q (use include, skill, source, marketplace or plugin)", updateKind))
	}
	if updateWriteConfig && !updateMajor {
		return fail(oops.Hint("config.toml is edited only to take a new major version: `ai-rulez update --major --write-config`").
			Errorf("--write-config needs --major"))
	}
	if err := checkFormatFlag(updateFormat); err != nil {
		return fail(err)
	}
	lockOffline = updateOffline
	if err := requireOnline("update"); err != nil {
		return fail(err)
	}
	return updateAt("", updateKind, names)
}

// updatableKinds lists the entry kinds `update` can move.
func updatableKinds() []string {
	return []string{lockfile.KindInclude, lockfile.KindSkill, lockfile.KindSource, lockfile.KindMarketplace, lockfile.KindPlugin}
}

// updateEnv is the command line as the host of an update or outdated run over
// the root at path.
func updateEnv(path string) pinupdate.Env {
	policy := cliLockPolicy
	policy.Offline = policy.Offline || lockOffline
	return pinupdate.Env{
		Version: Version,
		Policy:  policy,
		Load: func(ctx context.Context, p config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
			return loadForLockContext(ctx, path, append(opts, config.WithLockPolicy(p))...)
		},
		GitToken:  GetGitToken(),
		NewForge:  func(cfg *config.Config, offline bool) forge.Client { return newForgeClient(cfg, offline) },
		Collector: lockWarnings,
		Cwd:       workingDir(),
	}
}

// updateOptions are the consents the flags give.
func updateOptions() pinupdate.Options {
	return pinupdate.Options{DryRun: updateDryRun, AllowDowngrade: updateAllowDowngrade, AcceptMoved: updateAcceptMoved, AcceptFindings: updateAcceptFindings}
}

func updateAt(path, kind string, names []string) error {
	ctx := cmdContext()
	env := updateEnv(path)
	p, err := pinupdate.Prepare(ctx, env, kind, names)
	if err != nil {
		return fail(err)
	}
	report := pinupdate.NewReport(updateDryRun)
	if updateMajor {
		return majorUpdate(ctx, path, env, p, report)
	}
	return finishUpdate(report, runPrepared(ctx, env, p, report))
}

// runPrepared plans the update of p and writes the moves unless something is
// refused or this is a dry run. It returns nil, an exitFindings ExitError for a
// refusal, or the failure; printing the report is left to the caller.
func runPrepared(ctx context.Context, env pinupdate.Env, p *pinupdate.Prepared, report *updateReport) error {
	err := p.Run(ctx, env, updateOptions(), report)
	if report.LockPath != "" {
		logger.Success("Wrote lock file", "path", report.LockPath)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pinupdate.ErrRefused):
		return exitStatus(exitFindings)
	}
	return fail(err)
}
