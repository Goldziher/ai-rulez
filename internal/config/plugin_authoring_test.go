package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadConfigTOML_PluginAuthoring(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.toml")
	content := `
version = "4.0"
name = "basemind"
description = "Code-map MCP server."

[plugin]
name = "basemind"
display_name = "Basemind"
description = "Full AI context layer over MCP."
version = "0.19.2"
homepage = "https://github.com/Goldziher/basemind"
repository = "https://github.com/Goldziher/basemind"
license = "MIT"
keywords = ["mcp", "rag"]
runtimes = ["claude", "cursor", "codex"]

[plugin.author]
name = "Na'aman Hirschfeld"
email = "nhirschfeld@gmail.com"

[[plugin.mcp]]
name = "basemind"
command = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
args = ["serve"]

[[plugin.hooks]]
event = "SessionStart"
matcher = "startup|resume"

[[plugin.hooks.hooks]]
type = "command"
command = "${PLUGIN_ROOT}/hooks/run-hook.cmd session-start"
async = false

[plugin.statusline]
script = ".claude-plugin/statusline.sh"
command = "bm-statusline"

[plugin.kimi]
skill_instructions = "Prefer basemind tools over grep."
session_start_skill = "basemind"

[marketplace]
name = "basemind"
description = "basemind marketplace"

[marketplace.owner]
name = "Na'aman Hirschfeld"
email = "nhirschfeld@gmail.com"
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	cfg, err := loadConfigTOML(configFile)
	require.NoError(t, err)

	require.NotNil(t, cfg.Plugin)
	assert.Equal(t, "basemind", cfg.Plugin.Name)
	assert.Equal(t, "Basemind", cfg.Plugin.DisplayName)
	assert.Equal(t, "0.19.2", cfg.Plugin.Version)
	assert.Equal(t, []string{"mcp", "rag"}, cfg.Plugin.Keywords)
	assert.Equal(t, []string{"claude", "cursor", "codex"}, cfg.Plugin.Runtimes)

	require.NotNil(t, cfg.Plugin.Author)
	assert.Equal(t, "nhirschfeld@gmail.com", cfg.Plugin.Author.Email)

	require.Len(t, cfg.Plugin.MCP, 1)
	assert.Equal(t, "${PLUGIN_ROOT}/scripts/mcp-launch.sh", cfg.Plugin.MCP[0].Command)
	assert.Equal(t, []string{"serve"}, cfg.Plugin.MCP[0].Args)

	require.Len(t, cfg.Plugin.Hooks, 1)
	assert.Equal(t, "SessionStart", cfg.Plugin.Hooks[0].Event)
	require.Len(t, cfg.Plugin.Hooks[0].Hooks, 1)
	assert.Equal(t, "command", cfg.Plugin.Hooks[0].Hooks[0].Type)

	require.NotNil(t, cfg.Plugin.Statusline)
	assert.Equal(t, "bm-statusline", cfg.Plugin.Statusline.Command)

	require.NotNil(t, cfg.Plugin.Kimi)
	assert.Equal(t, "basemind", cfg.Plugin.Kimi.SessionStartSkill)

	require.NotNil(t, cfg.Marketplace)
	assert.Equal(t, "basemind", cfg.Marketplace.Name)
	require.NotNil(t, cfg.Marketplace.Owner)
	assert.Equal(t, "Na'aman Hirschfeld", cfg.Marketplace.Owner.Name)
}

func TestResolvedRuntimes(t *testing.T) {
	t.Run("empty returns all runtimes", func(t *testing.T) {
		p := &PluginAuthoring{}
		assert.Equal(t, AllPluginRuntimes, p.ResolvedRuntimes())
	})
	t.Run("explicit list is returned verbatim", func(t *testing.T) {
		p := &PluginAuthoring{Runtimes: []string{"claude", "codex"}}
		assert.Equal(t, []string{"claude", "codex"}, p.ResolvedRuntimes())
	})
	t.Run("nil receiver returns all runtimes", func(t *testing.T) {
		var p *PluginAuthoring
		assert.Equal(t, AllPluginRuntimes, p.ResolvedRuntimes())
	})
}

func TestValidatePluginAuthoring(t *testing.T) {
	base := func() *PluginAuthoring {
		return &PluginAuthoring{
			Name:        "basemind",
			Version:     "0.19.2",
			Description: "Full AI context layer.",
			Runtimes:    []string{"claude"},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*PluginAuthoring)
		wantErr string
	}{
		{name: "valid minimal", mutate: func(*PluginAuthoring) {}},
		{name: "missing name", mutate: func(p *PluginAuthoring) { p.Name = "" }, wantErr: "requires field 'name'"},
		{name: "missing version", mutate: func(p *PluginAuthoring) { p.Version = "" }, wantErr: "requires field 'version'"},
		{name: "bad version", mutate: func(p *PluginAuthoring) { p.Version = "not-a-version" }, wantErr: "invalid version"},
		{name: "good prerelease version", mutate: func(p *PluginAuthoring) { p.Version = "1.2.3-beta.1" }},
		{name: "short version", mutate: func(p *PluginAuthoring) { p.Version = "1.2" }, wantErr: "invalid version"},
		{name: "missing description", mutate: func(p *PluginAuthoring) { p.Description = "" }, wantErr: "requires field 'description'"},
		{name: "safe content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "plugin" }},
		{name: "absolute content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "/tmp/plugin" }, wantErr: "unsafe content root"},
		{name: "Windows absolute content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = `C:\tmp\plugin` }, wantErr: "unsafe content root"},
		{name: "Windows drive-relative content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = `C:plugin` }, wantErr: "unsafe content root"},
		{name: "escaping content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "../plugin" }, wantErr: "unsafe content root"},
		{name: "Windows escaping Hermes source", mutate: func(p *PluginAuthoring) {
			p.Hermes = &HermesExtras{Source: `..\outside.py`}
		}, wantErr: "unsafe Hermes source"},
		{name: "unknown runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"claude", "bogus"} }, wantErr: "unknown runtime"},
		{name: "duplicate runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"claude", "claude"} }, wantErr: "duplicate runtime"},
		{name: "Hermes runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"hermes"} }},
		{name: "mcp missing name", mutate: func(p *PluginAuthoring) { p.MCP = []PluginMCPLaunch{{Command: "x"}} }, wantErr: "MCP entry"},
		{name: "stdio mcp missing command", mutate: func(p *PluginAuthoring) { p.MCP = []PluginMCPLaunch{{Name: "s"}} }, wantErr: "no command"},
		{name: "http mcp missing url", mutate: func(p *PluginAuthoring) {
			p.MCP = []PluginMCPLaunch{{Name: "r", Transport: "http"}}
		}, wantErr: "no url"},
		{name: "http mcp with url ok", mutate: func(p *PluginAuthoring) {
			p.MCP = []PluginMCPLaunch{{Name: "r", Transport: "http", URL: "https://x"}}
		}},
		{name: "statusline missing script", mutate: func(p *PluginAuthoring) { p.Statusline = &Statusline{Command: "bm"} }, wantErr: "statusline requires 'script'"},
		{name: "hook missing event", mutate: func(p *PluginAuthoring) { p.Hooks = []HookGroup{{Matcher: "x"}} }, wantErr: "missing 'event'"},
		{name: "hook action with neither command nor script", mutate: func(p *PluginAuthoring) {
			p.Hooks = []HookGroup{{Event: "SessionStart", Hooks: []HookAction{{Type: "command"}}}}
		}, wantErr: "requires either 'command' or 'script'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(p)
			cfg := &Config{Plugin: p}
			err := cfg.validatePluginAuthoring()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidatePluginAuthoring_CodexRequiresCanonicalMetadata(t *testing.T) {
	plugin := &PluginAuthoring{
		Name:        "basemind",
		Version:     "1.0.0",
		Description: "Code map.",
		Runtimes:    []string{"codex"},
	}
	err := (&Config{Plugin: plugin}).validatePluginAuthoring()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "author.name")

	plugin.Author = &Author{Name: "Goldziher"}
	plugin.Interface = &PluginInterface{
		DisplayName:      "basemind",
		ShortDescription: "Code map",
		LongDescription:  "Navigate source code with a structural index.",
		DeveloperName:    "Goldziher",
		Category:         "Developer Tools",
		Capabilities:     []string{"Read"},
		DefaultPrompt:    []string{"Map this repository."},
	}
	require.NoError(t, (&Config{Plugin: plugin}).validatePluginAuthoring())
}

// hookHandlerTOML declares one hook action using every supported handler field,
// shared by the TOML decode test and the JSON-schema agreement test below so the
// two can never drift.
const hookHandlerTOML = `
version = "4.0"
name = "basemind"
description = "Code-map MCP server."

[plugin]
name = "basemind"
description = "Full AI context layer over MCP."
version = "1.0.0"

[[plugin.hooks]]
event = "SessionStart"
matcher = "startup|resume|clear|compact|fork"

[[plugin.hooks.hooks]]
type = "command"
script = ".ai-rulez/hooks/bootstrap.sh"
args = ["generate", "--quiet"]
timeout = 120
async = false
if = "!test -f CLAUDE.md"
status_message = "Bootstrapping generated AI config"
`

func TestLoadConfigTOML_HookHandlerFields(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configFile, []byte(hookHandlerTOML), 0o644))

	cfg, err := loadConfigTOML(configFile)
	require.NoError(t, err)

	require.NotNil(t, cfg.Plugin)
	require.Len(t, cfg.Plugin.Hooks, 1)
	group := cfg.Plugin.Hooks[0]
	assert.Equal(t, "SessionStart", group.Event)
	assert.Equal(t, "startup|resume|clear|compact|fork", group.Matcher)
	require.Len(t, group.Hooks, 1)
	assert.Equal(t, HookAction{
		Type:          HookTypeCommand,
		Script:        ".ai-rulez/hooks/bootstrap.sh",
		Args:          []string{"generate", "--quiet"},
		Timeout:       120,
		Async:         false,
		If:            "!test -f CLAUDE.md",
		StatusMessage: "Bootstrapping generated AI config",
	}, group.Hooks[0])
}

// TestHookHandlerFields_SchemaAcceptsEveryField guards the additionalProperties:
// false hook objects in schema/ai-rules.schema.json: a field added to HookAction
// without a matching schema property would be rejected for schema users.
func TestHookHandlerFields_SchemaAcceptsEveryField(t *testing.T) {
	yamlConfig := `
version: "4.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  hooks:
    - event: SessionStart
      matcher: startup|resume|clear|compact|fork
      hooks:
        - type: command
          script: .ai-rulez/hooks/bootstrap.sh
          args: [generate, --quiet]
          timeout: 120
          async: false
          if: "!test -f CLAUDE.md"
          status_message: Bootstrapping generated AI config
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))

	var decoded Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlConfig), &decoded))
	require.NotNil(t, decoded.Plugin)
	require.Len(t, decoded.Plugin.Hooks, 1)
	require.Len(t, decoded.Plugin.Hooks[0].Hooks, 1)
	action := decoded.Plugin.Hooks[0].Hooks[0]
	assert.Equal(t, ".ai-rulez/hooks/bootstrap.sh", action.Script)
	assert.Equal(t, []string{"generate", "--quiet"}, action.Args)
	assert.Equal(t, 120, action.Timeout)
	assert.Equal(t, "!test -f CLAUDE.md", action.If)
	assert.Equal(t, "Bootstrapping generated AI config", action.StatusMessage)
}

// hookProjectWithScript creates a project dir containing an executable bootstrap
// script and returns the project dir plus the script's project-relative path.
func hookProjectWithScript(t *testing.T) (projectDir, scriptPath string) {
	t.Helper()
	projectDir = t.TempDir()
	hooksDir := filepath.Join(projectDir, ".ai-rulez", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bootstrap.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	return projectDir, ".ai-rulez/hooks/bootstrap.sh"
}

func TestValidatePluginAuthoring_HookScriptDeclarations(t *testing.T) {
	projectDir, scriptPath := hookProjectWithScript(t)

	tests := []struct {
		name    string
		action  HookAction
		wantErr string
	}{
		{name: "script alone is accepted", action: HookAction{Script: scriptPath}},
		{name: "command alone is accepted", action: HookAction{Command: "echo hi"}},
		{
			name:    "command and script are mutually exclusive",
			action:  HookAction{Command: "echo hi", Script: scriptPath},
			wantErr: "sets both 'command' and 'script'",
		},
		{
			name:    "script missing on disk",
			action:  HookAction{Script: ".ai-rulez/hooks/absent.sh"},
			wantErr: "hook script not found",
		},
		{
			name:    "script escaping the project is rejected",
			action:  HookAction{Script: "../outside.sh"},
			wantErr: "unsafe hook script",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				BaseDir: projectDir,
				Plugin: &PluginAuthoring{
					Name:        "basemind",
					Version:     "1.0.0",
					Description: "Full AI context layer.",
					Runtimes:    []string{PluginRuntimeClaude},
					Hooks: []HookGroup{{
						Event: "SessionStart",
						Hooks: []HookAction{tc.action},
					}},
				},
			}
			err := cfg.validatePluginAuthoring()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestKnownHookEvents_CoversDocumentedEventSet(t *testing.T) {
	assert.Len(t, KnownHookEvents, 33, "Claude Code documents 33 hook events")
	for _, event := range []string{"SessionStart", "Setup", "WorktreeCreate", "WorktreeRemove", "SessionEnd"} {
		assert.Contains(t, KnownHookEvents, event)
	}
	for _, event := range HookEventsWithoutMatcher {
		assert.Contains(t, KnownHookEvents, event, "matcher-less event %q must be a known event", event)
	}
}

func TestHookDeclarationWarnings(t *testing.T) {
	t.Run("known event with a supported matcher warns nothing", func(t *testing.T) {
		warnings := hookDeclarationWarnings([]HookGroup{{
			Event:   "SessionStart",
			Matcher: "startup|resume",
			Hooks:   []HookAction{{Command: "echo hi"}},
		}})
		assert.Empty(t, warnings)
	})

	t.Run("unknown event warns", func(t *testing.T) {
		warnings := hookDeclarationWarnings([]HookGroup{{Event: "SesionStart"}})
		require.Len(t, warnings, 1)
		assert.Equal(t, warnUnknownHookEvent, warnings[0].Message)
		assert.Equal(t, "SesionStart", warnings[0].Event)
		assert.Equal(t, "plugin.hooks.event", warnings[0].Field)
	})

	t.Run("matcher on a matcher-less event warns", func(t *testing.T) {
		warnings := hookDeclarationWarnings([]HookGroup{{Event: "Stop", Matcher: "*"}})
		require.Len(t, warnings, 1)
		assert.Equal(t, warnIgnoredHookMatcher, warnings[0].Message)
		assert.Equal(t, "Stop", warnings[0].Event)
		assert.Equal(t, "plugin.hooks.matcher", warnings[0].Field)
	})

	t.Run("matcher-less event without a matcher warns nothing", func(t *testing.T) {
		assert.Empty(t, hookDeclarationWarnings([]HookGroup{{Event: "WorktreeCreate"}}))
	})

	t.Run("if on an event that does not evaluate it warns", func(t *testing.T) {
		warnings := hookDeclarationWarnings([]HookGroup{{
			Event: "SessionStart",
			Hooks: []HookAction{{Script: "bootstrap.sh", If: "Bash(git *)"}},
		}})
		require.Len(t, warnings, 1)
		assert.Equal(t, warnInertHookIf, warnings[0].Message)
		assert.Equal(t, "SessionStart", warnings[0].Event)
		assert.Equal(t, "plugin.hooks.hooks.if", warnings[0].Field)
	})

	t.Run("if on a tool-use event warns nothing", func(t *testing.T) {
		for _, event := range HookEventsEvaluatingIf {
			assert.Empty(t, hookDeclarationWarnings([]HookGroup{{
				Event: event,
				Hooks: []HookAction{{Command: "echo hi", If: "Edit(*.ts)"}},
			}}), "%s evaluates 'if' and must not warn", event)
		}
	})

	t.Run("if on an unknown event warns only about the event", func(t *testing.T) {
		warnings := hookDeclarationWarnings([]HookGroup{{
			Event: "PreToolUsage",
			Hooks: []HookAction{{Command: "echo hi", If: "Bash(*)"}},
		}})
		require.Len(t, warnings, 1, "an unknown event must not also be judged on its 'if' support")
		assert.Equal(t, warnUnknownHookEvent, warnings[0].Message)
	})

	t.Run("warnings are not errors", func(t *testing.T) {
		cfg := &Config{Plugin: &PluginAuthoring{
			Name:        "basemind",
			Version:     "1.0.0",
			Description: "Full AI context layer.",
			Runtimes:    []string{PluginRuntimeClaude},
			Hooks: []HookGroup{{
				Event:   "FutureClaudeEvent",
				Matcher: "whatever",
				Hooks:   []HookAction{{Command: "echo hi"}},
			}},
		}}
		require.NoError(t, cfg.validatePluginAuthoring())
	})
}

func TestValidatePluginAuthoring_NilIsValid(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, cfg.validatePluginAuthoring())
	require.NoError(t, cfg.validateMarketplaceAuthoring())
}

func TestValidateMarketplaceAuthoring(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{}}
		err := cfg.validateMarketplaceAuthoring()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires field 'name'")
	})
	t.Run("duplicate member", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{
			Name:    "mp",
			Members: []string{"plugins/a", "plugins/a"},
		}}
		err := cfg.validateMarketplaceAuthoring()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate member")
	})
	t.Run("member path traversal rejected", func(t *testing.T) {
		for _, bad := range []string{"../outside", `plugins\..\..\etc`, "/abs/path", `C:\abs\path`, `C:relative`} {
			cfg := &Config{Marketplace: &MarketplaceAuthoring{Name: "mp", Members: []string{bad}}}
			err := cfg.validateMarketplaceAuthoring()
			require.Error(t, err, "expected %q to be rejected", bad)
			assert.Contains(t, err.Error(), "invalid member path")
		}
	})
	t.Run("valid", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{
			Name:    "mp",
			Members: []string{"plugins/a", "plugins/b"},
		}}
		require.NoError(t, cfg.validateMarketplaceAuthoring())
	})
}

// TestValidateHookActionRejectsSymlinkEscape covers the validate-time half of
// the passthrough symlink defense: isUnsafeProjectPath is lexical, so a script
// that reaches outside the project through a link has to be caught by resolving
// it. The generator refuses to bundle such a file; validate should say so first.
func TestValidateHookActionRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	t.Run("should_reject_a_hook_script_symlinked_out_of_the_project", func(t *testing.T) {
		t.Parallel()

		outside := t.TempDir()
		secret := filepath.Join(outside, "id_rsa")
		require.NoError(t, os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600))

		baseDir := t.TempDir()
		require.NoError(t, os.Symlink(secret, filepath.Join(baseDir, "bootstrap.sh")))

		cfg := &Config{BaseDir: baseDir}
		err := cfg.validateHookAction("basemind", "SessionStart", 0, &HookAction{Script: "bootstrap.sh"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside the project")
	})

	t.Run("should_accept_a_hook_script_inside_the_project", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(baseDir, "bootstrap.sh"), []byte("#!/bin/sh\n"), 0o755))

		cfg := &Config{BaseDir: baseDir}

		require.NoError(t, cfg.validateHookAction("basemind", "SessionStart", 0, &HookAction{Script: "bootstrap.sh"}))
	})
}
