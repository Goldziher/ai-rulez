package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	internal "github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// pluginProject is a project and a marketplace repository served from disk.
type pluginProject struct {
	t      *testing.T
	dir    string
	home   string
	market *plugintest.Repo
}

func newPluginProject(t *testing.T) *pluginProject {
	t.Helper()
	p := &pluginProject{t: t, dir: testutil.CreateTempDir(t), home: testutil.CreateTempDir(t)}
	p.market = plugintest.NewRepo(t, p.marketFiles("review the change"))
	p.market.Tag("v1.0.0")
	internal.WriteTree(t, p.dir, map[string]string{
		".ai-rulez/config.toml":   "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\", \"codex\"]\ngitignore = false\n",
		".ai-rulez/rules/base.md": "# Base\n\nKeep it simple.\n",
	})
	return p
}

func (p *pluginProject) marketFiles(skillBody string) map[string]string {
	return plugintest.Merge(
		plugintest.ClaudeMarketplace("acme", `{"name":"tools","source":"./plugins/tools"}`),
		plugintest.Prefix("plugins/tools", plugintest.ClaudePlugin("tools")),
		map[string]string{"plugins/tools/skills/review-tools/SKILL.md": "---\nname: review-tools\ndescription: Review skill\n---\n\n" + skillBody + "\n"})
}

func (p *pluginProject) run(args ...string) *testutil.CLIResult {
	p.t.Helper()
	return testutil.RunCLIWithEnv(p.t, p.dir, map[string]string{"HOME": p.home, "USERPROFILE": p.home, "AI_RULEZ_ALLOW_FILE_URLS": "1", "AI_RULEZ_ACK_COMMANDS": "1"}, args...)
}

func (p *pluginProject) mustRun(args ...string) *testutil.CLIResult {
	p.t.Helper()
	res := p.run(args...)
	require.Equal(p.t, 0, res.ExitCode, "ai-rulez %s\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.Stdout, res.Stderr)
	return res
}

func (p *pluginProject) read(rel string) string {
	p.t.Helper()
	data, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(rel)))
	require.NoError(p.t, err, rel)
	return string(data)
}

func TestInstalledPlugins_EndToEnd(t *testing.T) {
	// Arrange
	p := newPluginProject(t)

	// Act: install through the commands
	p.mustRun("marketplace", "add", "acme", "--source", p.market.URL(), "--version", "^1.0")
	p.mustRun("plugin", "install", "tools", "--from", "acme", "--enable-hooks")
	list := p.mustRun("plugin", "list", "--format", "json")
	var doc struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(list.Stdout), &doc), "stdout: %q stderr: %q", list.Stdout, list.Stderr)

	// Assert
	require.Len(t, doc.Items, 1)
	assert.Equal(t, "tools", doc.Items[0]["name"])
	assert.Equal(t, true, doc.Items[0]["enable_hooks"])
	assert.Contains(t, p.mustRun("marketplace", "list").Stdout, "acme")
	config := p.read(".ai-rulez/config.toml")
	assert.Contains(t, config, "[[installed_marketplaces]]")
	assert.Contains(t, config, "[[installed_plugins]]")

	// Act: lock and generate
	p.mustRun("lock")
	lock := p.read(".ai-rulez/ai-rulez.lock")
	p.mustRun("generate")

	// Assert: pinned, and emitted for each preset
	assert.Contains(t, lock, "[[marketplace]]")
	assert.Contains(t, lock, "[[plugin]]")
	assert.Contains(t, lock, "tag = 'v1.0.0'")
	assert.Contains(t, lock, "source = 'marketplace:acme'")
	assert.Contains(t, p.read(".claude/skills/review-tools/SKILL.md"), "review the change")
	assert.Contains(t, p.read(".claude/agents/helper.md"), "You help.")
	assert.Contains(t, p.read(".claude/skills/deploy/SKILL.md"), "Deploy")
	assert.Contains(t, p.read(".mcp.json"), "tools-docs")
	assert.Contains(t, p.read(".claude/settings.json"), "echo formatted")
	assert.FileExists(t, filepath.Join(p.dir, ".agents", "skills", "review-tools", "SKILL.md"), "the codex preset gets the skill too")

	// Act / Assert: a second generate changes nothing, and the checks agree
	before := p.read(".claude/settings.json") + p.read(".claude/agents/helper.md")
	p.mustRun("generate")
	assert.Equal(t, before, p.read(".claude/settings.json")+p.read(".claude/agents/helper.md"))
	p.mustRun("generate", "--check")
	p.mustRun("lock", "--check")
	p.mustRun("generate", "--locked")
	p.mustRun("generate", "--frozen")
}

func TestInstalledPlugins_UpstreamChangeThenUpdateIsDrift(t *testing.T) {
	// Arrange
	p := newPluginProject(t)
	p.mustRun("marketplace", "add", "acme", "--source", p.market.URL(), "--version", "^1.0")
	p.mustRun("plugin", "install", "tools", "--from", "acme")
	p.mustRun("lock")
	p.mustRun("generate")
	p.mustRun("generate", "--check")

	// Act: the marketplace publishes a new compatible release and a major one
	p.market.Commit(p.marketFiles("review the change more strictly"), "1.1")
	p.market.Tag("v1.1.0")
	p.market.Commit(p.marketFiles("rewritten"), "2.0")
	p.market.Tag("v2.0.0")

	// Assert: the pin holds, so nothing drifts
	p.mustRun("generate", "--check")
	outdated := p.mustRun("lock", "--outdated", "--format", "json")
	assert.Contains(t, outdated.Stdout, "v1.1.0")

	// Act: update moves the pin; the outputs are now stale
	p.mustRun("update", "--kind", "marketplace")
	check := p.run("generate", "--check")

	// Assert
	assert.Equal(t, 2, check.ExitCode, "stdout: %s\nstderr: %s", check.Stdout, check.Stderr)
	assert.Contains(t, p.read(".ai-rulez/ai-rulez.lock"), "tag = 'v1.1.0'")

	// Act: regenerating settles it
	p.mustRun("generate")
	p.mustRun("generate", "--check")
	assert.Contains(t, p.read(".claude/skills/review-tools/SKILL.md"), "more strictly")
	assert.NotContains(t, p.read(".claude/skills/review-tools/SKILL.md"), "rewritten")
}

func TestInstalledPlugins_HooksNeedTheFlag(t *testing.T) {
	// Arrange
	p := newPluginProject(t)
	p.mustRun("marketplace", "add", "acme", "--source", p.market.URL())
	p.mustRun("plugin", "install", "tools", "--from", "acme")
	p.mustRun("lock")

	// Act
	p.mustRun("generate")

	// Assert
	settings := ""
	if data, err := os.ReadFile(filepath.Join(p.dir, ".claude", "settings.json")); err == nil {
		settings = string(data)
	}
	assert.NotContains(t, settings, "echo formatted")
}

func TestInstalledPlugins_CommandErrors(t *testing.T) {
	p := newPluginProject(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"marketplace without a source", []string{"marketplace", "add", "acme"}, "source"},
		{"plugin from an unknown marketplace", []string{"plugin", "install", "x", "--from", "nope"}, "not an installed_marketplaces"},
		{"plugin with neither marketplace nor source", []string{"plugin", "install", "x"}, "required"},
		{"credential in a source", []string{"marketplace", "add", "acme", "--source", "https://user:pw@example.com/x.git"}, "credential"},
		{"plain http", []string{"marketplace", "add", "acme", "--source", "http://example.com/x.git"}, "http://"},
		{"unknown kind", []string{"plugin", "install", "x", "--source", "a/b", "--include", "themes"}, "component kind"},
		{"remove a missing marketplace", []string{"marketplace", "remove", "nope", "--yes"}, "not found"},
		{"remove a missing plugin", []string{"plugin", "remove", "nope", "--yes"}, "not found"},
		{"update with an unknown kind", []string{"update", "--kind", "themes"}, "unknown --kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := p.run(tt.args...)
			assert.NotEqual(t, 0, res.ExitCode)
			assert.Contains(t, res.Stdout+res.Stderr, tt.want)
		})
	}
	t.Run("a marketplace in use cannot be removed", func(t *testing.T) {
		p.mustRun("marketplace", "add", "acme", "--source", p.market.URL())
		p.mustRun("plugin", "install", "tools", "--from", "acme")
		res := p.run("marketplace", "remove", "acme", "--yes")
		assert.NotEqual(t, 0, res.ExitCode)
		assert.Contains(t, res.Stdout+res.Stderr, "used by installed plugin")
		p.mustRun("plugin", "remove", "tools", "--yes")
		p.mustRun("marketplace", "remove", "acme", "--yes")
	})
}

func TestInstalledPlugins_ARefusedSourceNeverReachesTheOutputs(t *testing.T) {
	// Arrange: a marketplace entry that tries to leave the repository
	p := newPluginProject(t)
	evil := plugintest.NewRepo(t, plugintest.ClaudeMarketplace("evil", `{"name":"tools","source":"../../etc"}`))
	p.mustRun("marketplace", "add", "evil", "--source", evil.URL())
	p.mustRun("plugin", "install", "tools", "--from", "evil")

	// Act
	res := p.run("generate")

	// Assert
	assert.NotEqual(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout+res.Stderr, "leaves the directory")
	assert.NoFileExists(t, filepath.Join(p.dir, ".claude", "skills", "review-tools", "SKILL.md"))
	_ = pluginimport.IndexClaude
}
