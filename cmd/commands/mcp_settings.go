package commands

import (
	"context"
	"strings"

	"github.com/spf13/pflag"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

// flagMCPTools names the --tools flag, the allow-list of registered tools.
const flagMCPTools = "tools"

// flagServeSkills names the --serve-skills flag.
const flagServeSkills = "serve-skills"

// mcpSettings are the settings of `mcp` that both a flag and an [mcp] key can
// give. A flag that was given wins over the key; the organization policy's
// read-only requirement wins over both.
type mcpSettings struct {
	readOnly, daemon, noWatch bool
	// serveSkills is true when the run serves skills: the flag, else [mcp] serve_skills.
	serveSkills        bool
	allow, deny, tools []string
	// ignoredSkillsKeys names the [mcp] keys that only apply when skills are
	// served and are set while they are not, so the server ignores them.
	ignoredSkillsKeys []string
}

// resolveMCPSettings merges the command line over the project's [mcp] table.
func resolveMCPSettings(flags *pflag.FlagSet, project mcp.ProjectSettings) mcpSettings {
	keys := project.MCP
	boolSetting := func(name string, key *bool) bool {
		if flags.Changed(name) {
			v, _ := flags.GetBool(name) //nolint:errcheck // the flag is registered as a bool
			return v
		}
		return key != nil && *key
	}
	listSetting := func(name string, key []string) []string {
		if flags.Changed(name) {
			v, _ := flags.GetStringSlice(name) //nolint:errcheck // the flag is registered as a string slice
			key = v
		}
		if len(key) == 0 {
			return nil
		}
		return key
	}
	serve := boolSetting(flagServeSkills, keys.ServeSkills)
	settings := mcpSettings{
		serveSkills: serve,
		// A policy that requires read-only is not a default a flag can override.
		readOnly: boolSetting(flagMCPReadOnly, keys.ReadOnly) || project.PolicyReadOnly,
		daemon:   boolSetting(flagServeDaemon, keys.Daemon),
		noWatch:  boolSetting(flagServeNoWatch, keys.NoWatch),
		allow:    listSetting("allow", keys.Allow),
		deny:     listSetting("deny", keys.Deny),
		tools:    listSetting(flagMCPTools, keys.Tools),
	}
	if !serve {
		settings.ignoredSkillsKeys = skillsOnlyKeysSet(keys, settings)
	}
	return settings
}

// skillsOnlyKeysSet names the keys that only `mcp --serve-skills` reads and
// that are in effect, in a fixed order: the resolved values for the settings a
// flag can override, the [mcp] table for the rest.
func skillsOnlyKeysSet(keys config.MCPConfig, resolved mcpSettings) []string {
	var set []string
	add := func(name string, isSet bool) {
		if isSet {
			set = append(set, name)
		}
	}
	add("discover_skills", keys.DiscoverSkills != "" && keys.DiscoverSkills != config.DiscoverOff)
	add("allow", len(resolved.allow) > 0)
	add("deny", len(resolved.deny) > 0)
	add("exclude", len(keys.Exclude) > 0)
	add("no_watch", resolved.noWatch)
	add("daemon", resolved.daemon)
	return set
}

// ignoredSkillsKeysWarning is the startup warning for skills-only [mcp] keys
// that a server not serving skills ignores; empty when there are none.
func ignoredSkillsKeysWarning(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return "[mcp] " + strings.Join(keys, ", ") + " set but ignored: skills are not served; " +
		"pass --serve-skills or set [mcp] serve_skills = true"
}

// validate checks the tool allow-list against the tools the server has.
func (s mcpSettings) validate() error {
	return mcp.ValidateToolNames(s.tools) //nolint:wrapcheck // names the tool and the valid ones
}

// loadMCPSettings resolves the settings of this run: the [mcp] table of the
// project the server is started in, under the flags. A project that cannot be
// read is reported and the flags alone decide; the server's own load of the
// project reports the same problem where it matters.
func loadMCPSettings(ctx context.Context, flags *pflag.FlagSet) (mcpSettings, error) {
	project, err := mcp.LoadProjectSettings(ctx, workingDir())
	if err != nil {
		logger.Warn("Could not read the [mcp] settings of the project; using the command line only", "error", err.Error())
	}
	settings := resolveMCPSettings(flags, project)
	if err := settings.validate(); err != nil {
		return mcpSettings{}, err
	}
	return settings, nil
}

// options are the server options the settings give: the read-only tool set and
// the tool allow-list.
func (s mcpSettings) options() []mcp.Option {
	var opts []mcp.Option
	if s.readOnly {
		opts = append(opts, mcp.WithReadOnly())
	}
	if len(s.tools) > 0 {
		opts = append(opts, mcp.WithTools(s.tools...))
	}
	return opts
}

// applyTo puts the settings on a setup read from the flags: the skill filter,
// the watcher, the daemon and the authoring options. The values of the flags
// were already merged into the settings, so they replace what the setup read.
func (s mcpSettings) applyTo(setup *mcp.ServeSetup) {
	setup.Filter.Allow, setup.Filter.Deny = s.allow, s.deny
	setup.NoWatch = s.noWatch
	if s.daemon && !setup.Daemon {
		// The key turned the daemon on: its identity is the command line, as for the flag.
		setup.DaemonIdentity = daemonIdentity()
	}
	setup.Daemon = s.daemon
	setup.Authoring = append(setup.Authoring, s.options()...)
}
