package commands

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// Flags of the dynamic skill loading mode of `mcp --serve-skills`.
const (
	flagServeSource        = "source"
	flagServeRole          = flagRole
	flagServeFrozen        = "frozen"
	flagServeOffline       = "offline"
	flagServeIncludeStatic = "include-static"
	flagServeBudget        = "budget-bytes"
	flagServeMaxClone      = "max-clone-bytes"
	flagServeUsageLog      = "usage-log"
	flagServeUsageSink     = "usage-sink"
	flagServeNoWatch       = "no-watch"
	flagServePoll          = "reload-interval"
)

// dynamicServeFlagNames are the flags that only make sense with --serve-skills.
var dynamicServeFlagNames = []string{
	flagServeSource, flagServeRole, flagServeFrozen, flagServeOffline, flagServeIncludeStatic,
	flagServeBudget, flagServeMaxClone, flagServeUsageLog, flagServeUsageSink, flagServeNoWatch, flagServePoll,
}

func registerDynamicServeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringArray(flagServeSource, nil, "Serve the skills of a source, repeatable: [git+]<url>[@<tag|commit>][#<subdir>] or a local directory (requires --serve-skills)")
	f.String(flagServeRole, "", "Serve only the skills of this role (see 'ai-rulez roles list'), with the delivery the role sets; it is also the default role of find_skill (requires --serve-skills)")
	f.Bool(flagServeFrozen, false, "Never use the network and require ai-rulez.lock to cover every remote include, installed skill and skill source (requires --serve-skills)")
	f.Bool(flagServeOffline, false, "Never use the network; use cached content (requires --serve-skills)")
	f.Bool(flagServeIncludeStatic, false, "Also serve skills whose delivery is static (requires --serve-skills)")
	f.Int(flagServeBudget, 0, "Bytes of skill content a session may read (load_skill, get_skill, read_skill_file, resources/read); 0 is the default (256 KiB), -1 removes the cap (requires --serve-skills)")
	f.Int64(flagServeMaxClone, 0, "Largest git skill source clone in bytes for sources that set no max_clone_bytes; overrides AI_RULEZ_MAX_CLONE_BYTES; 0 uses the environment variable, then 256 MiB (requires --serve-skills)")
	f.String(flagServeUsageLog, "", "Append one identifier-only JSON line per load_skill to this file (requires --serve-skills)")
	f.String(flagServeUsageSink, "", "Shell command that receives each load_skill usage line on stdin (requires --serve-skills)")
	f.Bool(flagServeNoWatch, false, "Do not reload skills when their files change (requires --serve-skills)")
	f.Duration(flagServePoll, 0, "How often to check skill files for changes; default 2s (requires --serve-skills)")
}

// buildDynamicSkillServer loads the project (and any --source), renders the
// skills that are served, scans them, and wraps them in the read-only server.
// It starts the live-reload watcher on ctx. Nothing is written to disk except
// the optional usage log and the source cache.
func buildDynamicSkillServer(ctx context.Context, cmd *cobra.Command) (*mcp.Server, error) {
	setup, err := serveSetupFromFlags(cmd)
	if err != nil {
		return nil, err
	}
	srv, err := setup.NewServer(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // already wrapped
	}
	go srv.Watch(ctx)
	return srv, nil
}

// serveSetupFromFlags reads the flags of `mcp --serve-skills`.
func serveSetupFromFlags(cmd *cobra.Command) (*mcp.ServeSetup, error) {
	flags := cmd.Flags()
	setup := &mcp.ServeSetup{Version: Version, WorkDir: workingDir()}
	var err error
	read := func(name string, f func() error) {
		if err == nil {
			err = oops.Wrapf(f(), "read --%s", name)
		}
	}
	read("profile", func() (e error) { setup.Profile, e = flags.GetString("profile"); return })
	read("targets", func() (e error) { setup.Preset, e = flags.GetString("targets"); return })
	read(flagServeDomain, func() (e error) { setup.Filter.Domains, e = flags.GetStringSlice(flagServeDomain); return })
	read("allow", func() (e error) { setup.Filter.Allow, e = flags.GetStringSlice("allow"); return })
	read("deny", func() (e error) { setup.Filter.Deny, e = flags.GetStringSlice("deny"); return })
	read(flagServeSource, func() (e error) { setup.Sources, e = flags.GetStringArray(flagServeSource); return })
	read(flagServeRole, func() (e error) { setup.Role, e = flags.GetString(flagServeRole); return })
	read(flagServeFrozen, func() (e error) { setup.Frozen, e = flags.GetBool(flagServeFrozen); return })
	read(flagServeOffline, func() (e error) { setup.Offline, e = flags.GetBool(flagServeOffline); return })
	read(flagServeIncludeStatic, func() (e error) { setup.IncludeStatic, e = flags.GetBool(flagServeIncludeStatic); return })
	read(flagServeBudget, func() (e error) { setup.BudgetBytes, e = flags.GetInt(flagServeBudget); return })
	read(flagServeMaxClone, func() (e error) { setup.MaxCloneBytes, e = flags.GetInt64(flagServeMaxClone); return })
	read(flagServeUsageLog, func() (e error) { setup.UsageLog, e = flags.GetString(flagServeUsageLog); return })
	read(flagServeUsageSink, func() (e error) { setup.UsageSink, e = flags.GetString(flagServeUsageSink); return })
	read(flagServeNoWatch, func() (e error) { setup.NoWatch, e = flags.GetBool(flagServeNoWatch); return })
	read(flagServePoll, func() (e error) { setup.PollInterval, e = flags.GetDuration(flagServePoll); return })
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped above
	}
	return setup, nil
}
