package pluginresolve_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// marketFiles is the index file of a marketplace of the given format listing entries.
func marketFiles(format string, entries ...string) map[string]string {
	location := map[string]string{
		pluginimport.IndexClaude: ".claude-plugin/marketplace.json",
		pluginimport.IndexCodex:  ".agents/plugins/marketplace.json",
		pluginimport.IndexCursor: ".cursor-plugin/marketplace.json",
	}[format]
	doc := `{"name": "acme", "plugins": [`
	for i, e := range entries {
		if i > 0 {
			doc += ","
		}
		doc += e
	}
	return map[string]string{location: doc + `]}`}
}

// entry builds one marketplace entry of a source kind in the format's vocabulary.
func entry(format, kind, name string, repo *plugintest.Repo, sha string) string {
	switch kind {
	case "relative":
		if format == pluginimport.IndexCodex {
			return `{"name":"` + name + `","source":{"source":"local","path":"./plugins/` + name + `"}}`
		}
		return `{"name":"` + name + `","source":"./plugins/` + name + `"}`
	case "url+ref":
		return `{"name":"` + name + `","source":{"source":"url","url":"` + repo.URL() + `","ref":"main"}}`
	case "url+sha":
		return `{"name":"` + name + `","source":{"source":"url","url":"` + repo.URL() + `","sha":"` + sha + `"}}`
	case "subdir":
		return `{"name":"` + name + `","source":{"source":"git-subdir","url":"` + repo.URL() + `","path":"inner/` + name + `"}}`
	}
	panic("unknown kind " + kind)
}

func TestResolve_MarketplaceFormatsAndPluginSourceKinds(t *testing.T) {
	formats := []string{pluginimport.IndexClaude, pluginimport.IndexCodex, pluginimport.IndexCursor}
	kinds := []string{"relative", "url+ref", "url+sha", "subdir"}
	for _, format := range formats {
		for _, kind := range kinds {
			t.Run(format+"/"+kind, func(t *testing.T) {
				// Arrange
				isolate(t)
				plug := plugintest.NewRepo(t, plugintest.Merge(plugintest.ClaudePlugin("tools"), plugintest.Prefix("inner/tools", plugintest.ClaudePlugin("tools"))))
				files := plugintest.Merge(marketFiles(format, entry(format, kind, "tools", plug, plug.Head())),
					plugintest.Prefix("plugins/tools", plugintest.ClaudePlugin("tools")))
				market := plugintest.NewRepo(t, files)
				project := plugintest.Project(t, `
[[installed_marketplaces]]
name = "acme"
source = "`+market.URL()+`"

[[installed_plugins]]
name = "tools"
marketplace = "acme"
`)

				// Act
				cfg := mustLoad(t, project)

				// Assert
				assert.Equal(t, []string{"review-tools"}, skillNames(cfg))
				assert.Equal(t, []string{"helper"}, fileNames(cfg.Content.Agents))
				assert.Equal(t, []string{"deploy"}, fileNames(cfg.Content.Commands))
				assert.Contains(t, cfg.MCPServers, "tools-docs")
				assert.Empty(t, cfg.Hooks, "hooks are off by default")
			})
		}
	}
}

func TestResolve_MarketplaceFromLocalDirectoryAndIndexURL(t *testing.T) {
	t.Run("local directory", func(t *testing.T) {
		// Arrange
		isolate(t)
		project := plugintest.Project(t, `
[[installed_marketplaces]]
name = "acme"
source = "./vendor/market"

[[installed_plugins]]
name = "tools"
marketplace = "acme"
enable_hooks = true
`)
		testutil.WriteTree(t, project, plugintest.Prefix("vendor/market", plugintest.Merge(
			marketFiles(pluginimport.IndexClaude, `{"name":"tools","source":"./plugins/tools"}`),
			plugintest.Prefix("plugins/tools", plugintest.Merge(plugintest.ClaudePlugin("tools"), map[string]string{
				"hooks/hooks.json": `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/stop.sh"}]}]}}`,
				"stop.sh":          "#!/bin/sh\n",
			})))))

		// Act
		cfg := mustLoad(t, project)

		// Assert: a local plugin lives in the project, so its root variable is rewritten to that path
		assert.Equal(t, []string{"review-tools"}, skillNames(cfg))
		require.Len(t, cfg.Hooks, 1)
		assert.Equal(t, "./vendor/market/plugins/tools/stop.sh", cfg.Hooks[0].Hooks[0].Command)
	})
	t.Run("local directory outside the project is refused", func(t *testing.T) {
		// Arrange
		isolate(t)
		outside := t.TempDir()
		testutil.WriteTree(t, outside, marketFiles(pluginimport.IndexClaude))
		project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = "+testutil.TOMLString(outside)+"\n")

		// Act
		_, err := load(t, project)

		// Assert
		require.ErrorIs(t, err, config.ErrPluginUnresolved)
		assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
	})
	t.Run("relative source that escapes the marketplace is refused", func(t *testing.T) {
		// Arrange
		isolate(t)
		market := plugintest.NewRepo(t, marketFiles(pluginimport.IndexClaude, `{"name":"tools","source":"../outside"}`))
		project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"tools\"\nmarketplace = \"acme\"\n")

		// Act
		_, err := load(t, project)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "leaves the directory")
	})
	t.Run("symlinked plugin directory is refused", func(t *testing.T) {
		// Arrange
		isolate(t)
		outside := t.TempDir()
		testutil.WriteTree(t, outside, plugintest.ClaudePlugin("tools"))
		market := plugintest.NewRepo(t, marketFiles(pluginimport.IndexClaude, `{"name":"tools","source":"./plugins/tools"}`))
		market.Symlink("plugins/tools", outside)
		project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \"tools\"\nmarketplace = \"acme\"\n")

		// Act
		_, err := load(t, project)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink")
	})
	t.Run("missing and unusable entries explain themselves", func(t *testing.T) {
		// Arrange
		isolate(t)
		market := plugintest.NewRepo(t, marketFiles(pluginimport.IndexClaude, `{"name":"known","source":"./known"}`, `{"name":"npm","source":{"source":"npm","package":"x"}}`))
		for name, want := range map[string]string{"missing": "has no plugin", "npm": "not supported"} {
			project := plugintest.Project(t, "[[installed_marketplaces]]\nname = \"acme\"\nsource = \""+market.URL()+"\"\n\n[[installed_plugins]]\nname = \""+name+"\"\nmarketplace = \"acme\"\n")

			// Act
			_, err := load(t, project)

			// Assert
			require.Error(t, err, name)
			assert.Contains(t, err.Error(), want, name)
		}
	})
}

func TestResolve_DirectPluginSources(t *testing.T) {
	layouts := map[string]struct {
		files map[string]string
		skill string
	}{
		config.PluginFormatClaude:       {plugintest.ClaudePlugin("p"), "review-p"},
		config.PluginFormatCodex:        {plugintest.CodexPlugin("p"), "codex-p"},
		config.PluginFormatGemini:       {plugintest.GeminiExtension("p"), "gem-p"},
		config.PluginFormatAgentPlugins: {plugintest.AgentPlugin("p"), "ap-p"},
	}
	for format, l := range layouts {
		t.Run("repository root "+format, func(t *testing.T) {
			// Arrange
			isolate(t)
			repo := plugintest.NewRepo(t, l.files)
			project := plugintest.Project(t, "[[installed_plugins]]\nname = \"p\"\nsource = \""+repo.URL()+"\"\n")

			// Act
			cfg := mustLoad(t, project)

			// Assert
			assert.Equal(t, []string{l.skill}, skillNames(cfg))
		})
	}
	t.Run("cursor plugin brings rules", func(t *testing.T) {
		isolate(t)
		repo := plugintest.NewRepo(t, plugintest.CursorPlugin("c"))
		project := plugintest.Project(t, "[[installed_plugins]]\nname = \"c\"\nsource = \""+repo.URL()+"\"\n")
		cfg := mustLoad(t, project)
		assert.Contains(t, fileNames(cfg.Content.Rules), "style")
		assert.Equal(t, []string{"planner"}, fileNames(cfg.Content.Agents))
	})
	t.Run("subdirectory and tag", func(t *testing.T) {
		// Arrange
		isolate(t)
		repo := plugintest.NewRepo(t, plugintest.Prefix("packages/p", plugintest.ClaudePlugin("p")))
		repo.Tag("v1.0.0")
		repo.Commit(map[string]string{"packages/p/commands/extra.md": "extra\n"}, "later")
		project := plugintest.Project(t, "[[installed_plugins]]\nname = \"p\"\nsource = \""+repo.URL()+"\"\npath = \"packages/p\"\nref = \"v1.0.0\"\n")

		// Act
		cfg := mustLoad(t, project)

		// Assert: the tag, not the branch tip
		assert.Equal(t, []string{"deploy"}, fileNames(cfg.Content.Commands))
	})
	t.Run("local directory", func(t *testing.T) {
		isolate(t)
		project := plugintest.Project(t, "[[installed_plugins]]\nname = \"p\"\nsource = \"./tools/p\"\n")
		testutil.WriteTree(t, project, plugintest.Prefix("tools/p", plugintest.ClaudePlugin("p")))
		cfg := mustLoad(t, project)
		assert.Equal(t, []string{"review-p"}, skillNames(cfg))
	})
	t.Run("forced layout", func(t *testing.T) {
		isolate(t)
		repo := plugintest.NewRepo(t, plugintest.Merge(plugintest.ClaudePlugin("p"), plugintest.CodexPlugin("p")))
		project := plugintest.Project(t, "[[installed_plugins]]\nname = \"p\"\nsource = \""+repo.URL()+"\"\nformat = \"codex\"\n")
		cfg := mustLoad(t, project)
		assert.Contains(t, skillNames(cfg), "codex-p")
		assert.Empty(t, cfg.Content.Agents, "the Claude agents/ directory is not read as a Codex plugin")
		assert.NotContains(t, cfg.MCPServers, "p-docs", "the Claude .mcp.json is the Codex one here, but only its own server counts")
	})
	t.Run("a repository with no plugin fails the load", func(t *testing.T) {
		isolate(t)
		repo := plugintest.NewRepo(t, map[string]string{"README.md": "hi\n"})
		project := plugintest.Project(t, "[[installed_plugins]]\nname = \"p\"\nsource = \""+repo.URL()+"\"\n")
		_, err := load(t, project)
		require.ErrorIs(t, err, config.ErrPluginUnresolved)
	})
}

func TestResolve_KindsPrefixCollisionsAndScope(t *testing.T) {
	// Arrange
	isolate(t)
	repo := plugintest.NewRepo(t, plugintest.ClaudePlugin("p"))
	project := plugintest.Project(t, `
[profiles]
dev = ["backend"]

[[installed_plugins]]
name = "all"
source = "`+repo.URL()+`"
name_prefix = "a-"
enable_hooks = true

[[installed_plugins]]
name = "skills-only"
source = "`+repo.URL()+`"
include = ["skills"]
name_prefix = "s-"
profiles = ["dev"]

[[installed_plugins]]
name = "no-mcp"
source = "`+repo.URL()+`"
exclude = ["mcp", "hooks", "agents", "commands"]
name_prefix = "n-"
enable_mcp = false

[[installed_plugins]]
name = "clash"
source = "`+repo.URL()+`"
`)
	testutil.WriteTree(t, project, map[string]string{
		".ai-rulez/domains/backend/rules/r.md": "r\n",
		".ai-rulez/agents/helper.md":           "---\ndescription: mine\n---\n\nlocal helper\n",
	})

	// Act
	cfg := mustLoad(t, project)

	// Assert
	assert.ElementsMatch(t, []string{"a-review-p", "s-review-p", "n-review-p", "review-p"}, skillNames(cfg))
	for _, s := range cfg.Content.Skills {
		assert.Equal(t, s.Name, config.SkillID(s), "the output id carries the prefix like the name")
	}
	assert.ElementsMatch(t, []string{"helper", "a-helper"}, fileNames(cfg.Content.Agents), "the local agent is kept; the unprefixed plugin copy loses to it")
	for _, s := range cfg.Content.Skills {
		if s.Name == "s-review-p" {
			assert.Equal(t, []string{"dev"}, s.Profiles)
		}
	}
	assert.Len(t, cfg.Hooks, 1, "only the entry with enable_hooks imports hooks")
	assert.ElementsMatch(t, []string{"a-p-docs", "p-docs"}, mcpNames(cfg), "include = skills and enable_mcp = false import no servers")
}

func TestResolve_DomainPlacementAndProfileSelection(t *testing.T) {
	// Arrange
	isolate(t)
	repo := plugintest.NewRepo(t, plugintest.ClaudePlugin("p"))
	project := plugintest.Project(t, `
[profiles]
dev = ["backend"]
ops = []

[[installed_plugins]]
name = "p"
source = "`+repo.URL()+`"
domain = "backend"
profiles = ["dev"]
`)
	testutil.WriteTree(t, project, map[string]string{".ai-rulez/domains/backend/rules/r.md": "r\n"})
	cfg := mustLoad(t, project)

	// Act
	dev, err := cfg.GetContentForProfile("dev")
	require.NoError(t, err)
	ops, err := cfg.GetContentForProfile("ops")
	require.NoError(t, err)

	// Assert
	require.Contains(t, cfg.Content.Domains, "backend")
	assert.Equal(t, []string{"review-p"}, fileNames(cfg.Content.Domains["backend"].Skills))
	assert.Equal(t, []string{"helper"}, fileNames(dev.Domains["backend"].Agents))
	assert.Empty(t, ops.Domains["backend"], "the domain is not part of the ops profile")
	require.Contains(t, cfg.MCPServers, "p-docs")
	assert.Equal(t, "backend", cfg.MCPServers["p-docs"].Domain)
	assert.Equal(t, []string{"dev"}, cfg.MCPServers["p-docs"].Profiles)
}

func mcpNames(cfg *config.Config) []string {
	var out []string
	for name := range cfg.MCPServers {
		out = append(out, name)
	}
	return out
}

func TestResolve_EntryNamesAreStable(t *testing.T) {
	// The memo and lock keys are derived from the entry; its JSON form is stable.
	b, err := json.Marshal(config.InstalledPluginConfig{Name: "p", Source: "a/b"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"p","source":"a/b"}`, string(b))
}
