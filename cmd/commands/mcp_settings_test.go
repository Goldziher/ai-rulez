package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// settingsFlags is a flag set with the flags the [mcp] keys stand for, parsed
// from args.
func settingsFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("mcp", pflag.ContinueOnError)
	fs.Bool(flagMCPReadOnly, false, "")
	fs.Bool(flagServeSkills, false, "")
	fs.Bool(flagServeDaemon, false, "")
	fs.Bool(flagServeNoWatch, false, "")
	fs.StringSlice("allow", nil, "")
	fs.StringSlice("deny", nil, "")
	fs.StringSlice(flagMCPTools, nil, "")
	require.NoError(t, fs.Parse(args))
	return fs
}

func boolPtrOf(v bool) *bool { return &v }

func TestResolveMCPSettings_FlagsWinOverTheConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		project mcp.ProjectSettings
		want    mcpSettings
	}{
		{name: "nothing set", want: mcpSettings{}},
		{
			name:    "the configuration applies when no flag is given",
			project: mcp.ProjectSettings{MCP: config.MCPConfig{ReadOnly: boolPtrOf(true), Daemon: boolPtrOf(true), NoWatch: boolPtrOf(true), Allow: []string{"a"}, Deny: []string{"b"}, Tools: []string{"find_skill"}}},
			want:    mcpSettings{readOnly: true, daemon: true, noWatch: true, allow: []string{"a"}, deny: []string{"b"}, tools: []string{"find_skill"}, ignoredSkillsKeys: []string{"allow", "deny", "no_watch", "daemon"}},
		},
		{
			name:    "a flag set to false wins over a true key",
			args:    []string{"--read-only=false", "--daemon=false", "--no-watch=false"},
			project: mcp.ProjectSettings{MCP: config.MCPConfig{ReadOnly: boolPtrOf(true), Daemon: boolPtrOf(true), NoWatch: boolPtrOf(true)}},
			want:    mcpSettings{},
		},
		{
			name:    "a flag set to true wins over a false key",
			args:    []string{"--read-only"},
			project: mcp.ProjectSettings{MCP: config.MCPConfig{ReadOnly: boolPtrOf(false)}},
			want:    mcpSettings{readOnly: true},
		},
		{
			name:    "lists given on the command line replace the configured ones",
			args:    []string{"--allow=x-*", "--deny=y", "--tools=find_skill,load_skill"},
			project: mcp.ProjectSettings{MCP: config.MCPConfig{Allow: []string{"a"}, Deny: []string{"b"}, Tools: []string{"load_context"}}},
			want:    mcpSettings{allow: []string{"x-*"}, deny: []string{"y"}, tools: []string{"find_skill", "load_skill"}, ignoredSkillsKeys: []string{"allow", "deny"}},
		},
		{
			name:    "an empty flag list clears the configured one",
			args:    []string{"--tools="},
			project: mcp.ProjectSettings{MCP: config.MCPConfig{Tools: []string{"load_context"}}},
			want:    mcpSettings{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMCPSettings(settingsFlags(t, tt.args...), tt.project)

			assert.Equal(t, tt.want, got)
		})
	}
}

// The policy only tightens: neither the key nor the flag can switch off a
// read-only server the policy requires.
func TestResolveMCPSettings_PolicyReadOnlyCannotBeLoosened(t *testing.T) {
	tests := []struct {
		name string
		args []string
		key  *bool
	}{
		{name: "no key, no flag"},
		{name: "key false", key: boolPtrOf(false)},
		{name: "flag false", args: []string{"--read-only=false"}},
		{name: "key and flag false", args: []string{"--read-only=false"}, key: boolPtrOf(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := mcp.ProjectSettings{MCP: config.MCPConfig{ReadOnly: tt.key}, PolicyReadOnly: true}

			got := resolveMCPSettings(settingsFlags(t, tt.args...), project)

			assert.True(t, got.readOnly)
		})
	}
}

func TestMCPSettings_ValidateTools(t *testing.T) {
	tests := []struct {
		name    string
		tools   []string
		wantErr string
	}{
		{name: "none"},
		{name: "valid", tools: []string{"find_skill", "load_skill", "find_context", "load_context"}},
		{name: "unknown names the tool and the valid ones", tools: []string{"find_skil"}, wantErr: "find_skil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mcpSettings{tools: tt.tools}.validate()

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "load_skill")
		})
	}
}

func TestMCPCmd_ToolsFlag(t *testing.T) {
	flag := MCPCmd.Flags().Lookup(flagMCPTools)
	require.NotNil(t, flag, "mcp --tools restricts the registered tools")
	assert.Equal(t, "stringSlice", flag.Value.Type(), "repeatable and comma separated")
	assert.NotContains(t, dynamicServeFlagNames, flagMCPTools, "--tools also applies without --serve-skills")
}

func writeSettingsFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestLoadMCPSettings_ReadsTheProjectUnderTheFlags(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
	root := t.TempDir()
	writeSettingsFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[mcp]\nread_only = true\ntools = [\"find_skill\", \"load_skill\"]\n")
	t.Chdir(root)

	// Act
	fromConfig, err := loadMCPSettings(cmdContext(), settingsFlags(t))
	overridden, errFlag := loadMCPSettings(cmdContext(), settingsFlags(t, "--read-only=false", "--tools=load_context"))

	// Assert
	require.NoError(t, err)
	assert.True(t, fromConfig.readOnly)
	assert.Equal(t, []string{"find_skill", "load_skill"}, fromConfig.tools)
	require.NoError(t, errFlag)
	assert.False(t, overridden.readOnly, "a flag wins over the key")
	assert.Equal(t, []string{"load_context"}, overridden.tools)
}

func TestLoadMCPSettings_UnknownToolIsAnErrorThatNamesTheValidOnes(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
	root := t.TempDir()
	writeSettingsFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[mcp]\ntools = [\"find_skil\"]\n")
	t.Chdir(root)

	// Act
	_, err := loadMCPSettings(cmdContext(), settingsFlags(t))

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "find_skil")
	assert.Contains(t, err.Error(), "find_skill")
}

func TestLoadMCPSettings_WithoutAProjectTheFlagsDecide(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
	t.Chdir(t.TempDir())

	// Act
	got, err := loadMCPSettings(cmdContext(), settingsFlags(t, "--read-only", "--tools=find_skill"))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, mcpSettings{readOnly: true, tools: []string{"find_skill"}}, got)
}

// An organization policy that requires read-only is not something the project's
// [mcp] table or the command line can loosen.
func TestLoadMCPSettings_PolicyRequiredReadOnlyCannotBeLoosened(t *testing.T) {
	tests := []struct {
		name string
		key  string
		args []string
	}{
		{name: "key false", key: "read_only = false\n"},
		{name: "flag false", args: []string{"--read-only=false"}},
		{name: "both", key: "read_only = false\n", args: []string{"--read-only=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testutil.IsolateHome(t)
			root := t.TempDir()
			writeSettingsFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
				"version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[mcp]\n"+tt.key)
			policyFile := filepath.Join(t.TempDir(), "policy.toml")
			writeSettingsFile(t, policyFile, "policy_version = 1\n[mcp]\nread_only = true\n")
			t.Chdir(root)
			previous := activePolicy
			activePolicy = policy.NewEnforcer(func() policy.DiscoverOptions { return policy.DiscoverOptions{Flag: policyFile} })
			t.Cleanup(func() { activePolicy = previous })

			// Act
			got, err := loadMCPSettings(cmdContext(), settingsFlags(t, tt.args...))

			// Assert
			require.NoError(t, err)
			assert.True(t, got.readOnly)
		})
	}
}

// TestLoadMCPSettings_AnotherCheckoutsIncludeCacheKeepsTheTools guards #524: the
// include cache is shared by every checkout of the repository on one machine, so
// a lock-pinned include must be read at the commit its own lock records instead
// of failing the load and leaving the server with every tool registered.
func TestLoadMCPSettings_AnotherCheckoutsIncludeCacheKeepsTheTools(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
	cliLockPolicy.Mode, cliLockPolicy.Refresh, cliLockPolicy.Offline = includes.LockAuto, nil, false
	t.Cleanup(func() {
		cliLockPolicy.Mode, cliLockPolicy.Refresh, cliLockPolicy.Offline = includes.LockAuto, nil, false
	})
	remote := t.TempDir()
	crossGit(t, remote, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(remote, ".ai-rulez", "rules", "shared.md"), "# Shared\n\nversion one\n")
	crossGit(t, remote, "add", "-A")
	crossGit(t, remote, "commit", "-qm", "one")
	root := lockProject(t, `
[mcp]
tools = ["find_skill", "load_skill"]

[[includes]]
name = "shared"
source = "file://`+filepath.ToSlash(remote)+`"
`)
	require.Equal(t, 0, writeLockAt("", "", nil), "lock the include at its first commit")
	pinned, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	require.Len(t, pinned.Include, 1)
	first := pinned.Include[0].Commit

	// Another checkout on this machine pins a newer commit and leaves the shared
	// cache at it.
	writeFile(t, filepath.Join(remote, ".ai-rulez", "rules", "shared.md"), "# Shared\n\nversion two\n")
	crossGit(t, remote, "commit", "-qam", "two")
	require.Equal(t, 0, writeLockAt("", "", nil), "the other checkout locks the newer commit")
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), pinned), "this checkout still pins its own commit")

	// Act
	settings, err := loadMCPSettings(cmdContext(), settingsFlags(t))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"find_skill", "load_skill"}, settings.tools,
		"the project's allow-list is read at the commit its lock pins")
	assert.Equal(t, first, pinned.Include[0].Commit)
}

// TestLoadMCPSettings_AProjectThatCannotBeReadStopsTheServer: serving every tool
// after a failed read silently ignores the project's allow-list, so the run fails
// instead.
func TestLoadMCPSettings_AProjectThatCannotBeReadStopsTheServer(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
	root := t.TempDir()
	writeSettingsFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\nname = \"p\"\n")
	t.Chdir(root)

	// Act
	_, err := loadMCPSettings(cmdContext(), settingsFlags(t))

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[mcp] settings")
}

func TestResolveMCPSettings_ServeSkills(t *testing.T) {
	tests := []struct {
		name string
		args []string
		keys config.MCPConfig
		want bool
	}{
		{name: "off by default"},
		{name: "the key turns it on", keys: config.MCPConfig{ServeSkills: boolPtrOf(true)}, want: true},
		{name: "the flag turns it on", args: []string{"--serve-skills"}, want: true},
		{name: "the flag false wins over a true key", args: []string{"--serve-skills=false"}, keys: config.MCPConfig{ServeSkills: boolPtrOf(true)}},
		{name: "the flag true wins over a false key", args: []string{"--serve-skills"}, keys: config.MCPConfig{ServeSkills: boolPtrOf(false)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMCPSettings(settingsFlags(t, tt.args...), mcp.ProjectSettings{MCP: tt.keys})

			assert.Equal(t, tt.want, got.serveSkills)
		})
	}
}

func TestResolveMCPSettings_IgnoredSkillsKeys(t *testing.T) {
	tests := []struct {
		name string
		args []string
		keys config.MCPConfig
		want []string
	}{
		{name: "nothing set"},
		{
			name: "every skills-only key is named in a fixed order",
			keys: config.MCPConfig{
				DiscoverSkills: "repo", Allow: []string{"a"}, Deny: []string{"b"}, Exclude: []string{"c"},
				NoWatch: boolPtrOf(true), Daemon: boolPtrOf(true),
			},
			want: []string{"discover_skills", "allow", "deny", "exclude", "no_watch", "daemon"},
		},
		{name: "discover_skills off is not a setting", keys: config.MCPConfig{DiscoverSkills: "off"}},
		{name: "false booleans are not settings", keys: config.MCPConfig{NoWatch: boolPtrOf(false), Daemon: boolPtrOf(false)}},
		{name: "nothing is ignored when skills are served by the key", keys: config.MCPConfig{ServeSkills: boolPtrOf(true), DiscoverSkills: "all"}},
		{name: "nothing is ignored when skills are served by the flag", args: []string{"--serve-skills"}, keys: config.MCPConfig{DiscoverSkills: "all"}},
		{name: "the flag turning serving off ignores the keys", args: []string{"--serve-skills=false"}, keys: config.MCPConfig{ServeSkills: boolPtrOf(true), Allow: []string{"a"}}, want: []string{"allow"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMCPSettings(settingsFlags(t, tt.args...), mcp.ProjectSettings{MCP: tt.keys})

			assert.Equal(t, tt.want, got.ignoredSkillsKeys)
		})
	}
}

func TestIgnoredSkillsKeysWarning(t *testing.T) {
	assert.Empty(t, ignoredSkillsKeysWarning(nil))
	got := ignoredSkillsKeysWarning([]string{"discover_skills", "allow"})
	assert.Contains(t, got, "discover_skills, allow")
	assert.Contains(t, got, "--serve-skills")
	assert.Contains(t, got, "[mcp] serve_skills")
}

// A read-only skills server registers only the skill-serving tools unless the
// project asks for the authoring set or names its own list (#482).
func TestMCPSettings_ReadOnlySkillsServerDefaultsToTheSkillTools(t *testing.T) {
	skillTools := mcp.SkillServingTools()
	tests := []struct {
		name string
		args []string
		keys config.MCPConfig
		want bool
	}{
		{name: "read-only skills", keys: config.MCPConfig{ServeSkills: boolPtrOf(true), ReadOnly: boolPtrOf(true)}, want: true},
		{name: "flags", args: []string{"--serve-skills", "--read-only"}, want: true},
		{name: "authoring opts out", keys: config.MCPConfig{ServeSkills: boolPtrOf(true), ReadOnly: boolPtrOf(true), Authoring: boolPtrOf(true)}},
		{name: "skills without read-only keep the full set", keys: config.MCPConfig{ServeSkills: boolPtrOf(true)}},
		{name: "read-only without skills keeps the full set", keys: config.MCPConfig{ReadOnly: boolPtrOf(true)}},
		{name: "an explicit list wins", keys: config.MCPConfig{ServeSkills: boolPtrOf(true), ReadOnly: boolPtrOf(true), Tools: []string{"find_skill"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := resolveMCPSettings(settingsFlags(t, tt.args...), mcp.ProjectSettings{MCP: tt.keys})

			if tt.want {
				assert.ElementsMatch(t, skillTools, settings.toolAllowList())
			} else {
				assert.NotElementsMatch(t, skillTools, settings.toolAllowList())
			}
		})
	}
}

func TestMCPSettings_DaemonIdleTimeout(t *testing.T) {
	good := resolveMCPSettings(settingsFlags(t), mcp.ProjectSettings{MCP: config.MCPConfig{DaemonIdleTimeout: "90s"}})
	require.NoError(t, good.validate())
	assert.Equal(t, 90*time.Second, good.daemonIdleTimeout)
	setup := &mcp.ServeSetup{}
	good.applyTo(setup)
	assert.Equal(t, 90*time.Second, setup.DaemonIdleTimeout)

	for _, bad := range []string{"soon", "0s", "-5m"} {
		got := resolveMCPSettings(settingsFlags(t), mcp.ProjectSettings{MCP: config.MCPConfig{DaemonIdleTimeout: bad}})
		assert.Error(t, got.validate(), bad)
	}
}

func TestMCPSettings_ServeUserInvokedSkills(t *testing.T) {
	tests := []struct {
		name string
		keys config.MCPConfig
		want bool
	}{
		{name: "off by default"},
		{name: "explicitly off", keys: config.MCPConfig{ServeUserInvokedSkills: boolPtrOf(false)}},
		{name: "on", keys: config.MCPConfig{ServeUserInvokedSkills: boolPtrOf(true)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			settings := resolveMCPSettings(settingsFlags(t), mcp.ProjectSettings{MCP: tt.keys})
			setup := &mcp.ServeSetup{}

			// Act
			settings.applyTo(setup)

			// Assert
			assert.Equal(t, tt.want, setup.ServeUserInvoked)
		})
	}
}
