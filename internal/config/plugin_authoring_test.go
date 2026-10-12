package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestLoadConfigTOML_PluginAuthoring(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.toml")
	content := `
version = "5.0"
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
include = ["scripts/**", "assets/*.png"]
package_manager = "pnpm@9.15.0"
opencode_dependency = "^2.1.0"

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

	cfg, err := loadConfigTOML(osView(filepath.Dir(configFile)), configFile)
	require.NoError(t, err)

	require.NotNil(t, cfg.Plugin)
	assert.Equal(t, "basemind", cfg.Plugin.Name)
	assert.Equal(t, "Basemind", cfg.Plugin.DisplayName)
	assert.Equal(t, "0.19.2", cfg.Plugin.Version)
	assert.Equal(t, []string{"mcp", "rag"}, cfg.Plugin.Keywords)
	assert.Equal(t, []string{"claude", "cursor", "codex"}, cfg.Plugin.Runtimes)
	assert.Equal(t, []string{"scripts/**", "assets/*.png"}, cfg.Plugin.Include)
	assert.Equal(t, "pnpm@9.15.0", cfg.Plugin.PackageManager)
	assert.Equal(t, "^2.1.0", cfg.Plugin.OpencodeDependency)

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
	t.Run("agent-plugins is opt-in only", func(t *testing.T) {
		assert.NotContains(t, AllPluginRuntimes, PluginRuntimeAgentPlugins)
		assert.Contains(t, KnownPluginRuntimes, PluginRuntimeAgentPlugins)
	})
	t.Run("Pi is opt-in only", func(t *testing.T) {
		assert.NotContains(t, AllPluginRuntimes, "pi")
		assert.Contains(t, KnownPluginRuntimes, "pi")
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
		{name: "skills dir owned", mutate: func(p *PluginAuthoring) { p.SkillsDirOwned = true }},
		{name: "skills dir owned with a generated Pi dir", mutate: func(p *PluginAuthoring) {
			p.SkillsDirOwned, p.Pi = true, &PiExtras{SkillsDir: "skills"}
		}},
		{name: "skills dir owned over the config directory", mutate: func(p *PluginAuthoring) {
			p.SkillsDirOwned, p.Pi = true, &PiExtras{SkillsDir: ".ai-rulez/skills"}
		}, wantErr: "holds the authored sources"},
		{name: "skills dir owned over the content root", mutate: func(p *PluginAuthoring) {
			p.SkillsDirOwned, p.ContentRoot, p.Pi = true, "plugin", &PiExtras{SkillsDir: "plugin/skills"}
		}, wantErr: "holds the authored sources"},
		{name: "owned dir over sources is fine when not owned", mutate: func(p *PluginAuthoring) {
			p.Pi = &PiExtras{SkillsDir: ".ai-rulez/skills"}
		}},
		{name: "Pi runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"pi"} }},
		{name: "agent-plugins runtime valid name", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"agent-plugins"} }},
		{name: "agent-plugins invalid name", mutate: func(p *PluginAuthoring) {
			p.Name, p.Runtimes = "Bad_Name", []string{"agent-plugins"}
		}, wantErr: "not valid for the agent-plugins runtime"},
		{name: "agent-plugins consecutive hyphen", mutate: func(p *PluginAuthoring) {
			p.Name, p.Runtimes = "bad--name", []string{"agent-plugins"}
		}, wantErr: "not valid for the agent-plugins runtime"},
		{name: "spec 1.0.0", mutate: func(p *PluginAuthoring) { p.Spec = "1.0.0" }},
		{name: "spec 1.1.0", mutate: func(p *PluginAuthoring) { p.Spec = "1.1.0" }},
		{name: "unknown spec", mutate: func(p *PluginAuthoring) { p.Spec = "2.0.0" }, wantErr: "unsupported Agent Plugins spec"},
		{name: "dotted and underscored name ok", mutate: func(p *PluginAuthoring) { p.Name = "my_tool.v2-x" }},
		{name: "uppercase name", mutate: func(p *PluginAuthoring) { p.Name = "MyTool" }, wantErr: "not a valid plugin name"},
		{name: "scoped name with slash", mutate: func(p *PluginAuthoring) { p.Name = "@scope/tool" }, wantErr: "not a valid plugin name"},
		{name: "path separator", mutate: func(p *PluginAuthoring) { p.Name = "a/b" }, wantErr: "not a valid plugin name"},
		{name: "backslash", mutate: func(p *PluginAuthoring) { p.Name = `a\b` }, wantErr: "not a valid plugin name"},
		{name: "parent segment", mutate: func(p *PluginAuthoring) { p.Name = "a..b" }, wantErr: "not a valid plugin name"},
		{name: "leading dot", mutate: func(p *PluginAuthoring) { p.Name = ".hidden" }, wantErr: "not a valid plugin name"},
		{name: "comment terminator", mutate: func(p *PluginAuthoring) { p.Name = "x*/y" }, wantErr: "not a valid plugin name"},
		{name: "quote", mutate: func(p *PluginAuthoring) { p.Name = `x"y` }, wantErr: "not a valid plugin name"},
		{name: "every runtime checks the name", mutate: func(p *PluginAuthoring) {
			p.Name, p.Runtimes = "Bad Name", []string{"opencode"}
		}, wantErr: "not a valid plugin name"},
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
		{name: "include glob accepted", mutate: func(p *PluginAuthoring) { p.Include = []string{"scripts/**"} }},
		{name: "include multiple globs accepted", mutate: func(p *PluginAuthoring) {
			p.Include = []string{"scripts/**", "assets/*.png"}
		}},
		{name: "include escaping the content root", mutate: func(p *PluginAuthoring) {
			p.Include = []string{"../scripts/**"}
		}, wantErr: "escapes the content root"},
		{name: "include absolute path", mutate: func(p *PluginAuthoring) {
			p.Include = []string{"/scripts/**"}
		}, wantErr: "escapes the content root"},
		{name: "include Windows absolute path", mutate: func(p *PluginAuthoring) {
			p.Include = []string{`C:\scripts\**`}
		}, wantErr: "escapes the content root"},
		{name: "include empty pattern", mutate: func(p *PluginAuthoring) {
			p.Include = []string{"  "}
		}, wantErr: "empty pattern"},
		{name: "include malformed glob", mutate: func(p *PluginAuthoring) {
			p.Include = []string{"scripts/[abc"}
		}, wantErr: "malformed glob"},
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

// TestPluginVersionFrom_SchemaAcceptsTheField guards the plugin.version_from
// property in schema/ai-rules.schema.json: a config that declares a version
// source instead of a literal version must not be flagged as an unknown key.
func TestPluginVersionFrom_SchemaAcceptsTheField(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version_from: package.json
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))
}

// TestPluginProvenanceHeaders_SchemaAcceptsTheField guards the
// plugin.provenance_headers property in schema/ai-rules.schema.json.
func TestPluginProvenanceHeaders_SchemaAcceptsTheField(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  provenance_headers: false
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))

	var decoded Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlConfig), &decoded))
	require.NotNil(t, decoded.Plugin)
	require.NotNil(t, decoded.Plugin.ProvenanceHeaders)
	assert.False(t, *decoded.Plugin.ProvenanceHeaders)
}

// writeVersionSource writes a version manifest under dir, creating parents.
func writeVersionSource(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestPluginAuthoring_ResolvedVersion(t *testing.T) {
	dir := t.TempDir()
	writeVersionSource(t, dir, "package.json", `{"name":"p","version":"2.3.4"}`)
	writeVersionSource(t, dir, "frontend/package.json", `{"version":"9.9.9"}`)
	writeVersionSource(t, dir, "pyproject.toml", "[project]\nname = \"p\"\nversion = \"2.3.4\"\n")
	writeVersionSource(t, dir, "Cargo.toml", "[package]\nname = \"p\"\nversion = \"2.3.4\"\n")
	writeVersionSource(t, dir, "nokey/package.json", `{"name":"p"}`)

	tests := []struct {
		name        string
		version     string
		versionFrom string
		want        string
		wantErr     string
	}{
		{name: "literal version", version: "1.0.0", want: "1.0.0"},
		{name: "package.json version", versionFrom: "package.json", want: "2.3.4"},
		{name: "nested package.json", versionFrom: "frontend/package.json", want: "9.9.9"},
		{name: "pyproject project.version", versionFrom: "pyproject.toml", want: "2.3.4"},
		{name: "Cargo package.version", versionFrom: "Cargo.toml", want: "2.3.4"},
		{name: "missing file", versionFrom: "nested/package.json", wantErr: "not found"},
		{name: "unsupported file", versionFrom: "setup.cfg", wantErr: "unsupported file"},
		{name: "missing key", versionFrom: "nokey/package.json", wantErr: "has no version"},
		{name: "escaping path", versionFrom: "../package.json", wantErr: "not a safe project path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			p := &PluginAuthoring{Name: "p", Version: tc.version, VersionFrom: tc.versionFrom}

			// Act
			got, err := p.ResolvedVersion(dir)

			// Assert
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var p *PluginAuthoring
		got, err := p.ResolvedVersion(dir)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestValidatePluginAuthoring_VersionFrom(t *testing.T) {
	dir := t.TempDir()
	writeVersionSource(t, dir, "package.json", `{"name":"p","version":"1.2.3"}`)
	writeVersionSource(t, dir, "short/package.json", `{"version":"1.2"}`)
	writeVersionSource(t, dir, "nokey/package.json", `{"name":"p"}`)

	base := func() *PluginAuthoring {
		return &PluginAuthoring{Name: "p", Description: "d", Runtimes: []string{PluginRuntimeClaude}}
	}
	tests := []struct {
		name    string
		mutate  func(*PluginAuthoring)
		wantErr string
	}{
		{name: "version_from resolves", mutate: func(p *PluginAuthoring) { p.VersionFrom = "package.json" }},
		{name: "both version and version_from", mutate: func(p *PluginAuthoring) {
			p.Version, p.VersionFrom = "1.0.0", "package.json"
		}, wantErr: "both 'version' and 'version_from'"},
		{name: "neither version nor version_from", mutate: func(*PluginAuthoring) {}, wantErr: "requires field 'version'"},
		{name: "version_from missing file", mutate: func(p *PluginAuthoring) {
			p.VersionFrom = "nested/package.json"
		}, wantErr: "not found"},
		{name: "version_from missing key", mutate: func(p *PluginAuthoring) {
			p.VersionFrom = "nokey/package.json"
		}, wantErr: "has no version"},
		{name: "version_from not semver", mutate: func(p *PluginAuthoring) {
			p.VersionFrom = "short/package.json"
		}, wantErr: "invalid version"},
		{name: "version_from unsupported file", mutate: func(p *PluginAuthoring) {
			p.VersionFrom = "setup.cfg"
		}, wantErr: "unsupported file"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			p := base()
			tc.mutate(p)
			cfg := &Config{BaseDir: dir, Plugin: p}

			// Act
			err := cfg.validatePluginAuthoring()

			// Assert
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestPluginAuthoring_WritesProvenanceHeaders(t *testing.T) {
	off, on := false, true
	tests := []struct {
		name string
		ptr  *bool
		want bool
	}{
		{name: "unset defaults true", ptr: nil, want: true},
		{name: "explicit false", ptr: &off, want: false},
		{name: "explicit true", ptr: &on, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &PluginAuthoring{ProvenanceHeaders: tc.ptr}
			assert.Equal(t, tc.want, p.WritesProvenanceHeaders())
		})
	}

	var p *PluginAuthoring
	assert.True(t, p.WritesProvenanceHeaders())
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
version = "5.0"
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

	cfg, err := loadConfigTOML(osView(filepath.Dir(configFile)), configFile)
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
version: "5.0"
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

// TestPluginInclude_SchemaAcceptsTheField guards the plugin.include property in
// schema/ai-rules.schema.json: a config that uses it must not be flagged as an
// unknown key.
func TestPluginSkillsDirOwned_SchemaAcceptsTheField(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  skills_dir_owned: true
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))
}

func TestPluginInclude_SchemaAcceptsTheField(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  runtimes: [agent-plugins]
  include: ["scripts/**"]
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))
}

// TestPluginPackageFields_SchemaAcceptsTheFields guards the plugin.package_manager
// and plugin.opencode_dependency properties in schema/ai-rules.schema.json: a
// config that uses them must not be flagged as an unknown key.
func TestPluginPackageFields_SchemaAcceptsTheFields(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  runtimes: [opencode]
  package_manager: pnpm@9.15.0
  opencode_dependency: "^2.1.0"
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))

	var decoded Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlConfig), &decoded))
	require.NotNil(t, decoded.Plugin)
	assert.Equal(t, "pnpm@9.15.0", decoded.Plugin.PackageManager)
	assert.Equal(t, "^2.1.0", decoded.Plugin.OpencodeDependency)
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
			name:    "script with shell metacharacters is rejected",
			action:  HookAction{Script: ".ai-rulez/hooks/a b;id.sh"},
			wantErr: "may only contain",
		},
		{
			name:    "script with a command substitution is rejected",
			action:  HookAction{Script: ".ai-rulez/hooks/$(id).sh"},
			wantErr: "may only contain",
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
		testutil.SymlinkOrSkip(t, secret, filepath.Join(baseDir, "bootstrap.sh"))

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

// A [[hooks]] group that carries its command on the group itself instead of in a
// [[hooks.hooks]] action decodes to a group with no actions, so no harness writes
// anything for it. That is the config a user is most likely to write by mistake
// and it must not pass silently.
func TestEmptyHookGroupWarnings(t *testing.T) {
	tests := []struct {
		name   string
		groups []HookGroup
		want   []string
	}{
		{"group with an action", []HookGroup{{Event: "PreToolUse", Hooks: []HookAction{{Command: "x"}}}}, nil},
		{"group without actions", []HookGroup{{Event: "PreToolUse", Matcher: "Bash"}}, []string{"PreToolUse"}},
		{"only the empty one is reported", []HookGroup{
			{Event: "Stop", Hooks: []HookAction{{Command: "x"}}}, {Event: "SessionStart"},
		}, []string{"SessionStart"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			warnings := emptyHookGroupWarnings(tt.groups)

			// Assert
			var events []string
			for _, w := range warnings {
				events = append(events, w.Event)
				assert.Equal(t, "hooks.hooks", w.Field)
				assert.Contains(t, w.Message, "[[hooks.hooks]]")
			}
			assert.Equal(t, tt.want, events)
		})
	}
}

// TestLoadConfigTOML_PiAndSharedResources guards the [plugin.pi] sub-table and
// the [plugin] shared_resources list: both decode and resolve their defaults.
func TestLoadConfigTOML_PiAndSharedResources(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	content := `
version = "5.0"
name = "basemind"
description = "Code-map MCP server."

[plugin]
name = "basemind"
description = "Full AI context layer over MCP."
version = "1.0.0"
shared_resources = ["references", "templates"]

[plugin.pi]
skills_dir = "skills"
package_json = "merge"
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	cfg, err := loadConfigTOML(osView(filepath.Dir(configFile)), configFile)
	require.NoError(t, err)

	require.NotNil(t, cfg.Plugin)
	assert.Equal(t, []string{"references", "templates"}, cfg.Plugin.SharedResources)
	require.NotNil(t, cfg.Plugin.Pi)
	assert.Equal(t, "skills", cfg.Plugin.Pi.SkillsDir)
	assert.Equal(t, "skills", cfg.Plugin.Pi.ResolvedSkillsDir())
	assert.Equal(t, PiPackageJSONMerge, cfg.Plugin.Pi.ResolvedPackageJSON())
	assert.True(t, cfg.Plugin.Pi.MergesPackageJSON())
}

// TestPiExtrasDefaults covers the resolved defaults of [plugin.pi] and the
// nil-receiver behavior the generator relies on.
func TestPiExtrasDefaults(t *testing.T) {
	tests := []struct {
		name        string
		extras      *PiExtras
		wantSkills  string
		wantMode    string
		wantMerging bool
	}{
		{name: "nil receiver", extras: nil, wantSkills: DefaultPiSkillsDir, wantMode: PiPackageJSONGenerate},
		{name: "empty", extras: &PiExtras{}, wantSkills: DefaultPiSkillsDir, wantMode: PiPackageJSONGenerate},
		{name: "explicit skills dir", extras: &PiExtras{SkillsDir: "skills"}, wantSkills: "skills", wantMode: PiPackageJSONGenerate},
		{name: "explicit merge", extras: &PiExtras{PackageJSON: PiPackageJSONMerge}, wantSkills: DefaultPiSkillsDir, wantMode: PiPackageJSONMerge, wantMerging: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantSkills, tc.extras.ResolvedSkillsDir())
			assert.Equal(t, tc.wantMode, tc.extras.ResolvedPackageJSON())
			assert.Equal(t, tc.wantMerging, tc.extras.MergesPackageJSON())
		})
	}
}

func TestPiExtrasValidate(t *testing.T) {
	tests := []struct {
		name    string
		extras  *PiExtras
		wantErr string
	}{
		{name: "nil", extras: nil},
		{name: "unset", extras: &PiExtras{}},
		{name: "generate", extras: &PiExtras{PackageJSON: PiPackageJSONGenerate}},
		{name: "merge", extras: &PiExtras{PackageJSON: PiPackageJSONMerge}},
		{name: "relative skills dir", extras: &PiExtras{SkillsDir: "skills"}},
		{name: "nested skills dir", extras: &PiExtras{SkillsDir: "vendor/skills"}},
		{name: "unknown package_json", extras: &PiExtras{PackageJSON: "replace"}, wantErr: "unknown value"},
		{name: "unscoped package_name", extras: &PiExtras{PackageName: "ghost-library-skills"}},
		{name: "scoped package_name", extras: &PiExtras{PackageName: "@acme/pi-tool"}},
		{name: "package_name with spaces", extras: &PiExtras{PackageName: "my pkg"}, wantErr: "plugin.pi.package_name"},
		{name: "package_name uppercase", extras: &PiExtras{PackageName: "Pkg"}, wantErr: "plugin.pi.package_name"},
		{name: "package_name empty scope", extras: &PiExtras{PackageName: "@/pkg"}, wantErr: "plugin.pi.package_name"},
		{name: "skills_entry each", extras: &PiExtras{SkillsEntry: PiSkillsEntryEach}},
		{name: "skills_entry directory", extras: &PiExtras{SkillsEntry: PiSkillsEntryDirectory}},
		{name: "unknown skills_entry", extras: &PiExtras{SkillsEntry: "all"}, wantErr: "plugin.pi.skills_entry"},
		{name: "escaping skills dir", extras: &PiExtras{SkillsDir: "../skills"}, wantErr: "not a safe project path"},
		{name: "absolute skills dir", extras: &PiExtras{SkillsDir: "/skills"}, wantErr: "not a safe project path"},
		{name: "windows absolute skills dir", extras: &PiExtras{SkillsDir: `C:\skills`}, wantErr: "not a safe project path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.extras.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestResolveSharedResources(t *testing.T) {
	project := t.TempDir()
	contentRoot := filepath.Join(project, "plugin")
	writeSharedDir(t, filepath.Join(contentRoot, "references"), map[string]string{"r.md": "ref"})
	writeSharedDir(t, filepath.Join(contentRoot, "other", "references"), map[string]string{"r.md": "ref"})

	fileEntry := filepath.Join(contentRoot, "notes.md")
	require.NoError(t, os.WriteFile(fileEntry, []byte("x"), 0o644))

	base := func(entries []string, contentRootValue string) (*PluginAuthoring, *Config) {
		return &PluginAuthoring{SharedResources: entries, ContentRoot: contentRootValue},
			&Config{BaseDir: project, ConfigDir: filepath.Join(project, ".ai-rulez")}
	}

	tests := []struct {
		name    string
		entries []string
		root    string
		want    []SharedResource
		wantErr string
	}{
		{name: "empty", entries: nil},
		{
			name: "resolves against the content root", entries: []string{"references"}, root: "plugin",
			want: []SharedResource{{Name: "references", Dir: filepath.Join(contentRoot, "references")}},
		},
		{
			name: "trailing slash and nested entry", entries: []string{"references/"}, root: "plugin",
			want: []SharedResource{{Name: "references", Dir: filepath.Join(contentRoot, "references")}},
		},
		{name: "missing directory", entries: []string{"absent"}, root: "plugin", wantErr: "not found"},
		{name: "file not directory", entries: []string{"notes.md"}, root: "plugin", wantErr: "not a directory"},
		{name: "escaping entry", entries: []string{"../references"}, root: "plugin", wantErr: "not a safe directory"},
		{name: "absolute entry", entries: []string{"/references"}, root: "plugin", wantErr: "not a safe directory"},
		{name: "duplicate base name", entries: []string{"references", "other/references"}, root: "plugin", wantErr: "duplicate directory name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, cfg := base(tc.entries, tc.root)
			got, err := p.ResolveSharedResources(cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var p *PluginAuthoring
		got, err := p.ResolveSharedResources(&Config{BaseDir: project})
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

// writeSharedDir writes files under dir, creating parents.
func writeSharedDir(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

// TestPluginPiAndSharedResources_SchemaAcceptsTheFields guards the plugin.pi and
// plugin.shared_resources properties in schema/ai-rules.schema.json.
func TestPluginPiAndSharedResources_SchemaAcceptsTheFields(t *testing.T) {
	yamlConfig := `
version: "5.0"
name: basemind
description: Code-map MCP server.
plugin:
  name: basemind
  description: Full AI context layer over MCP.
  version: 1.0.0
  runtimes: [pi]
  shared_resources: [references, templates]
  pi:
    skills_dir: skills
    package_json: merge
`
	require.NoError(t, schema.ValidateWithSchema([]byte(yamlConfig)))

	var decoded Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlConfig), &decoded))
	require.NotNil(t, decoded.Plugin)
	assert.Equal(t, []string{"references", "templates"}, decoded.Plugin.SharedResources)
	require.NotNil(t, decoded.Plugin.Pi)
	assert.Equal(t, "skills", decoded.Plugin.Pi.SkillsDir)
	assert.Equal(t, PiPackageJSONMerge, decoded.Plugin.Pi.PackageJSON)
}
