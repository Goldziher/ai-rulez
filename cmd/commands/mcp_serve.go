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
	flagServeDaemon        = "daemon"
	flagServeDiscover      = "discover-skills"
	flagServeExclude       = "exclude"
	// flagServeDaemonServer marks the process the proxy spawned as the daemon
	// itself. It is hidden: a user runs --daemon, and the proxy adds this flag
	// on the child only.
	flagServeDaemonServer = "daemon-server"
)

// dynamicServeFlagNames are the flags that only make sense with --serve-skills.
var dynamicServeFlagNames = []string{
	flagServeSource, flagServeRole, flagServeFrozen, flagServeOffline, flagServeIncludeStatic,
	flagServeBudget, flagServeMaxClone, flagServeUsageLog, flagServeUsageSink, flagServeNoWatch, flagServePoll,
	flagServeDaemon, flagServeDiscover, flagServeExclude,
}

func registerDynamicServeFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringArray(flagServeSource, nil, "Serve the skills of a source, repeatable: [git+]<url>[@<tag|commit>][#<subdir>] or a local directory (requires --serve-skills)")
	f.String(flagServeRole, "", "Serve only the skills of this role (see 'ai-rulez roles list'), with the delivery the role sets; it is also the default role of find_skill (or AI_RULEZ_ROLE; requires --serve-skills)")
	f.Bool(flagServeFrozen, false, "Never use the network and require ai-rulez.lock to cover every remote include, installed skill and skill source (requires --serve-skills)")
	f.Bool(flagServeOffline, false, "Never use the network; use cached content (requires --serve-skills)")
	f.Bool(flagServeIncludeStatic, false, "Also serve skills whose delivery is static (requires --serve-skills)")
	f.Int(flagServeBudget, 0, "Bytes of skill content a session may read (load_skill, get_skill, read_skill_file, resources/read); 0 is the default (256 KiB), -1 removes the cap (requires --serve-skills)")
	f.Int64(flagServeMaxClone, 0, "Largest git skill source clone in bytes for sources that set no max_clone_bytes; overrides AI_RULEZ_MAX_CLONE_BYTES; 0 uses the environment variable, then 256 MiB (requires --serve-skills)")
	f.String(flagServeUsageLog, "", "Append one identifier-only JSON line per load_skill to this file (requires --serve-skills)")
	f.String(flagServeUsageSink, "", "Shell command that receives each load_skill usage line on stdin (requires --serve-skills)")
	f.Bool(flagServeNoWatch, false, "Do not reload skills when their files change (requires --serve-skills)")
	f.Duration(flagServePoll, 0, "How often to check skill files for changes; default 2s (requires --serve-skills)")
	f.Bool(flagServeDaemon, false, "Share one long-lived skills server per repository: proxy stdin/stdout to a per-repo daemon, starting it on first use, and fall back to serving in-process when none is reachable (requires --serve-skills)")
	f.String(flagServeDiscover, "", "Also serve the skills already in the repository: repo (skill directories of the working tree: skills/, .claude/skills at any depth and the configured presets' skill directories, honoring .gitignore) or all (repo plus installed and included skills); either also indexes nested AGENTS.md, CLAUDE.md, GEMINI.md, rules and agents by scope for find_context, load_context and the path and scope arguments of find_skill; a bare --discover-skills means repo; default is [mcp] discover_skills, else off (requires --serve-skills)")
	f.Lookup(flagServeDiscover).NoOptDefVal = "repo"
	f.StringArray(flagServeExclude, nil, "Glob, relative to the project root, of paths repository discovery never reads (skill directories and the context index: CLAUDE.md, AGENTS.md, rules, agents); repeatable, and added to [mcp] exclude; ** crosses directories, a pattern without a slash matches at any depth (requires --serve-skills and --discover-skills or [mcp] discover_skills)")
	f.Bool(flagServeDaemonServer, false, "Run as the shared daemon itself instead of proxying to it; set by the spawned child, not from the command line")
	_ = f.MarkHidden(flagServeDaemonServer) //nolint:errcheck // the flag was just registered
}

// buildDynamicSkillServer loads the project (and any --source), renders the
// skills that are served, scans them, and wraps them in the read-only server.
// It starts the live-reload watcher on ctx. Nothing is written to disk except
// the optional usage log and the source cache.
func buildDynamicSkillServer(ctx context.Context, cmd *cobra.Command, settings mcpSettings) (*mcp.Server, error) {
	setup, err := serveSetupFor(cmd, settings)
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

// serveSetupFor is the setup of this run: the flags of `mcp --serve-skills`,
// with the settings the flags and the project's [mcp] table resolved to.
func serveSetupFor(cmd *cobra.Command, settings mcpSettings) (*mcp.ServeSetup, error) {
	setup, err := serveSetupFromFlags(cmd)
	if err != nil {
		return nil, err
	}
	settings.applyTo(setup)
	return setup, nil
}

// serveSetupFromFlags reads the flags of `mcp --serve-skills`.
func serveSetupFromFlags(cmd *cobra.Command) (*mcp.ServeSetup, error) {
	flags := cmd.Flags()
	setup := &mcp.ServeSetup{Version: Version, WorkDir: workingDir()}
	// The merged server registers the authoring surface too, so `mcp
	// --serve-skills` is the one MCP entry a project needs. The engines and the
	// directory confinement come from the command line exactly as for `mcp`;
	// serveSetupFor narrows them with the read-only and tool settings.
	setup.Authoring = append(engineOptions(), authoringOptions(cmd)...)
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
	read(flagServeDaemon, func() (e error) { setup.Daemon, e = flags.GetBool(flagServeDaemon); return })
	read(flagServeDiscover, func() (e error) { setup.DiscoverSkills, e = flags.GetString(flagServeDiscover); return })
	read(flagServeExclude, func() (e error) { setup.Exclude, e = flags.GetStringArray(flagServeExclude); return })
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped above
	}
	if setup.Daemon {
		// The identity is the command line itself, so two MCP entries that differ
		// in any flag (a profile, a root, a usage sink) never share a daemon, and
		// the spawned child, which gets the same arguments, agrees with its proxy.
		setup.DaemonIdentity = daemonIdentity()
	}
	return setup, nil
}
