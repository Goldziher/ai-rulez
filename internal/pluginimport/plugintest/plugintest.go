// Package plugintest builds the fixtures the marketplace and plugin tests need:
// real local git repositories made at test time (init, commit, tag), plugin and
// marketplace trees for every supported layout, and a minimal project. No test
// using it touches the network.
package plugintest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// Repo is a git repository in a temporary directory, served through file://.
type Repo struct {
	tb  testing.TB
	Dir string
}

// NewRepo makes a repository on branch main holding files in one commit.
func NewRepo(tb testing.TB, files map[string]string) *Repo {
	tb.Helper()
	dir := tb.TempDir()
	testutil.Git(tb, dir, "init", "-b", "main")
	r := &Repo{tb: tb, Dir: dir}
	r.Commit(files, "initial")
	return r
}

// URL is the file:// URL of the repository.
func (r *Repo) URL() string { return "file://" + filepath.ToSlash(r.Dir) }

// Commit writes files, stages everything and commits; it returns the commit.
func (r *Repo) Commit(files map[string]string, msg string) string {
	r.tb.Helper()
	testutil.WriteTree(r.tb, r.Dir, files)
	testutil.Git(r.tb, r.Dir, "add", "-A")
	testutil.Git(r.tb, r.Dir, "commit", "--allow-empty", "-m", msg)
	return r.Head()
}

// Remove deletes paths and commits.
func (r *Repo) Remove(msg string, paths ...string) string {
	r.tb.Helper()
	for _, p := range paths {
		require.NoError(r.tb, os.RemoveAll(filepath.Join(r.Dir, filepath.FromSlash(p))))
	}
	testutil.Git(r.tb, r.Dir, "add", "-A")
	testutil.Git(r.tb, r.Dir, "commit", "--allow-empty", "-m", msg)
	return r.Head()
}

// Head is the current commit.
func (r *Repo) Head() string {
	r.tb.Helper()
	return testutil.Git(r.tb, r.Dir, "rev-parse", "HEAD")
}

// Tag tags the current commit (lightweight), replacing an existing tag.
func (r *Repo) Tag(name string) {
	r.tb.Helper()
	testutil.Git(r.tb, r.Dir, "tag", "-f", name)
}

// TagAnnotated tags the current commit with an annotated tag.
func (r *Repo) TagAnnotated(name string) {
	r.tb.Helper()
	testutil.Git(r.tb, r.Dir, "tag", "-f", "-a", "-m", name, name)
}

// Symlink adds a symlink at rel pointing to target and commits it.
func (r *Repo) Symlink(rel, target string) string {
	r.tb.Helper()
	full := filepath.Join(r.Dir, filepath.FromSlash(rel))
	require.NoError(r.tb, os.MkdirAll(filepath.Dir(full), 0o755))
	testutil.SymlinkOrSkip(r.tb, target, full)
	testutil.Git(r.tb, r.Dir, "add", "-A")
	testutil.Git(r.tb, r.Dir, "commit", "-m", "symlink "+rel)
	return r.Head()
}

// Project makes a project directory with a minimal .ai-rulez/config.toml
// (presets claude and codex plus extra TOML) and a hand-written CLAUDE.md.
func Project(tb testing.TB, extraTOML string) string {
	tb.Helper()
	dir := tb.TempDir()
	cfg := "version = \"5.0\"\nname = \"fixture\"\npresets = [\"claude\", \"codex\"]\n" + extraTOML
	testutil.WriteTree(tb, dir, map[string]string{
		".ai-rulez/config.toml":       cfg,
		".ai-rulez/rules/base.md":     "---\npriority: high\n---\n\nKeep it simple.\n",
		".ai-rulez/rules/.gitkeep":    "",
		".ai-rulez/context/.gitkeep":  "",
		".ai-rulez/agents/.gitkeep":   "",
		".ai-rulez/commands/.gitkeep": "",
		".ai-rulez/skills/.gitkeep":   "",
	})
	return dir
}

// Merge combines fixture file maps; a later map wins.
func Merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// Prefix puts every path of files below dir.
func Prefix(dir string, files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for k, v := range files {
		out[strings.TrimSuffix(dir, "/")+"/"+k] = v
	}
	return out
}

// Paths lists the keys of files, sorted.
func Paths(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Skill is the files of one skill directory.
func Skill(name, body string) map[string]string {
	return map[string]string{
		"skills/" + name + "/SKILL.md":            "---\nname: " + name + "\ndescription: " + name + " skill\n---\n\n" + body + "\n",
		"skills/" + name + "/references/guide.md": "# Guide for " + name + "\n",
	}
}

// ClaudePlugin is a Claude Code plugin with the default directories: a skill, an
// agent, a command, a hooks file and an MCP file.
func ClaudePlugin(name string) map[string]string {
	return Merge(Skill("review-"+name, "Review carefully."), map[string]string{
		".claude-plugin/plugin.json": `{"name": "` + name + `", "version": "1.0.0", "description": "The ` + name + ` plugin"}`,
		"agents/helper.md":           "---\nname: helper\ndescription: Helps\ntools: Read, Grep\n---\n\nYou help.\n",
		"commands/deploy.md":         "---\ndescription: Deploy it\n---\n\nDeploy $ARGUMENTS.\n",
		"hooks/hooks.json": `{"hooks": {"PostToolUse": [{"matcher": "Write|Edit", "hooks": [
			{"type": "command", "command": "echo formatted", "timeout": 10}]}]}}`,
		".mcp.json": `{"mcpServers": {"` + name + `-docs": {"command": "npx", "args": ["-y", "@example/docs-server"]}}}`,
	})
}

// CodexPlugin is a Codex plugin: a manifest, a skill and an MCP file.
func CodexPlugin(name string) map[string]string {
	return Merge(Skill("codex-"+name, "Codex skill."), map[string]string{
		".codex-plugin/plugin.json": `{"name": "` + name + `", "version": "0.2.0", "description": "Codex ` + name + `", "skills": "./skills/", "mcpServers": "./.mcp.json"}`,
		".mcp.json":                 `{"mcpServers": {"` + name + `-api": {"url": "https://example.com/mcp", "type": "http"}}}`,
	})
}

// GeminiExtension is a Gemini extension: a manifest with an inline MCP server and
// a context file, a TOML command and a skill.
func GeminiExtension(name string) map[string]string {
	return Merge(Skill("gem-"+name, "Gemini skill."), map[string]string{
		"gemini-extension.json": `{"name": "` + name + `", "version": "2.0.0", "contextFileName": "GEMINI.md",
			"mcpServers": {"` + name + `-srv": {"command": "node", "args": ["server.js"]}}}`,
		"GEMINI.md":                "# " + name + " context\n\nPrefer small functions.\n",
		"commands/review.toml":     "description = \"Review a change\"\nprompt = \"Review {{args}} carefully.\"\n",
		"commands/git/commit.toml": "prompt = \"Write a commit message.\"\n",
	})
}

// CursorPlugin is a Cursor plugin: a manifest, a rule, an agent and a command.
func CursorPlugin(name string) map[string]string {
	return map[string]string{
		".cursor-plugin/plugin.json": `{"name": "` + name + `", "version": "1.1.0", "description": "Cursor ` + name + `"}`,
		"rules/style.mdc":            "---\ndescription: Style\nalwaysApply: true\n---\n\nUse tabs.\n",
		"agents/planner.md":          "---\nname: planner\ndescription: Plans\n---\n\nPlan first.\n",
		"commands/ship.md":           "Ship it.\n",
	}
}

// AgentPlugin is an Agent Plugins package: a root plugin.json, a skill, an MCP
// file and a Claude Code namespace holding an agent.
func AgentPlugin(name string) map[string]string {
	return Merge(Skill("ap-"+name, "Agent plugin skill."), map[string]string{
		"plugin.json": `{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": "` + name + `", "version": "1.0.0", "description": "AP ` + name + `"}`,
		"mcp.json":    `{"mcpServers": {"` + name + `-ap": {"command": "uvx", "args": ["server"]}}}`,
		"com.anthropic.claude-code/agents/ap-agent.md": "---\nname: ap-agent\ndescription: AP agent\n---\n\nAgent body.\n",
	})
}

// ClaudeMarketplace is a Claude Code marketplace index listing the given plugin
// entries (raw JSON objects).
func ClaudeMarketplace(name string, entries ...string) map[string]string {
	return map[string]string{
		".claude-plugin/marketplace.json": `{"name": "` + name + `", "owner": {"name": "Acme"}, "plugins": [` + strings.Join(entries, ",") + `]}`,
	}
}
