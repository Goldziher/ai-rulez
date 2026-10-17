package mcp

// End-to-end check of `ai-rulez mcp --serve-skills --discover-skills` (issue 432):
// the skills a repository already keeps are served beside the ai-rulez ones.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const discoverServeConfig = "version = \"5.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\", \"codex\"]\n"

func discoverServeProject(t *testing.T, config string) string {
	t.Helper()
	files := map[string]string{"skills/own/SKILL.md": serveSkill("own", "The own skill of ai-rulez", "Own body.", "")}
	for _, rel := range []string{"skills/foo", ".claude/skills/bar", "pkg/.claude/skills/baz", ".agents/skills/qux"} {
		name := rel[len(rel)-3:]
		files["@"+rel+"/SKILL.md"] = serveSkill(name, "The "+name+" skill of the repository", name+" body.", "")
	}
	// The same name as the ai-rulez skill: shadowed.
	files["@skills/impostor/SKILL.md"] = serveSkill("own", "Pretends to be the own skill", "Impostor body.", "")
	return serveProject(t, config, files)
}

func TestServeSkills_DiscoverSkillsFlagServesRepositorySkills(t *testing.T) {
	// Arrange
	root := discoverServeProject(t, discoverServeConfig)
	p := startServe(t, root, "--discover-skills", "--no-watch")
	p.initialize()

	// Act / Assert
	for name, body := range map[string]string{"foo": "foo body.", "bar": "bar body.", "baz": "baz body.", "qux": "qux body.", "own": "Own body."} {
		assert.Contains(t, p.load(name), body, name)
	}
	assert.NotContains(t, p.load("own"), "Impostor body.", "the ai-rulez skill wins the name")
	assert.Contains(t, p.stderrText(), "shadowed")
}

func TestServeSkills_WithoutDiscoveryOnlyAiRulezSkillsAreServed(t *testing.T) {
	// Arrange
	root := discoverServeProject(t, discoverServeConfig)
	p := startServe(t, root, "--no-watch")
	p.initialize()

	// Act / Assert
	assert.Contains(t, p.load("own"), "Own body.")
	assert.Empty(t, p.load("foo"))
	assert.Empty(t, p.load("baz"))
}

func TestServeSkills_DiscoverSkillsFromConfiguration(t *testing.T) {
	// Arrange
	root := discoverServeProject(t, discoverServeConfig+"\n[mcp]\ndiscover_skills = \"repo\"\n")
	p := startServe(t, root, "--no-watch")
	p.initialize()

	// Act / Assert
	assert.Contains(t, p.load("qux"), "qux body.")
}

func TestServeSkills_DiscoverSkillsWorksWithoutAnyConfiguration(t *testing.T) {
	// Arrange
	root := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, gitutil.CommandNoContext("", "init", "-q", root).Run())
	skill := filepath.Join(root, ".claude", "skills", "bar", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
	require.NoError(t, os.WriteFile(skill, []byte(serveSkill("bar", "The bar skill of the repository", "bar body.", "")), 0o644))
	p := startServe(t, root, "--discover-skills=repo", "--no-watch")
	p.initialize()

	// Act / Assert
	assert.Contains(t, p.load("bar"), "bar body.")
}

// toolResult calls a tool and returns its JSON result, failing on a tool error.
func (p *serveProc) toolResult(name string, args map[string]any) map[string]any {
	p.t.Helper()
	resp := p.call("tools/call", map[string]any{"name": name, "arguments": args})
	result, ok := resp["result"].(map[string]any)
	require.True(p.t, ok, "%s: %v", name, resp)
	require.NotEqual(p.t, true, result["isError"], "%s: %v", name, result)
	content, _ := result["content"].([]any)
	require.NotEmpty(p.t, content)
	text, _ := content[0].(map[string]any)["text"].(string)
	var out map[string]any
	require.NoError(p.t, json.Unmarshal([]byte(text), &out), text)
	return out
}

func resultField(out map[string]any, field string) []string {
	var values []string
	results, _ := out["results"].([]any)
	for _, r := range results {
		values = append(values, r.(map[string]any)[field].(string))
	}
	return values
}

func monorepoServeProject(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"skills/own/SKILL.md":                   serveSkill("own", "The own skill of ai-rulez", "Own body.", ""),
		"@.claude/skills/lint/SKILL.md":         serveSkill("lint", "Lint the root with the root linter", "Root lint.", ""),
		"@pkg/api/.claude/skills/lint/SKILL.md": serveSkill("lint", "Lint the api package with the api linter", "Api lint.", ""),
		"@pkg/web/skills/ui/SKILL.md":           serveSkill("ui", "Build a web ui component", "Ui body.", ""),
		"@AGENTS.md":                            "# Root conventions\n\nBe terse.\n",
		"@pkg/api/AGENTS.md":                    "# API conventions\n\nWrap every error.\n",
		"@pkg/api/.claude/agents/reviewer.md":   "---\ndescription: Reviews changes to the api\n---\n\nReview carefully.\n",
	}
	return serveProject(t, discoverServeConfig, files)
}

func TestServeSkills_PathScopedFindSkillOverStdio(t *testing.T) {
	// Arrange
	p := startServe(t, monorepoServeProject(t), "--discover-skills", "--no-watch")
	p.initialize()

	// Act
	api := p.toolResult("find_skill", map[string]any{"task": "lint the code", "path": "pkg/api/handler.go"})
	web := p.toolResult("find_skill", map[string]any{"task": "lint the code", "path": "pkg/web/index.ts"})
	scoped := p.toolResult("find_skill", map[string]any{"task": "build ui", "scope": "pkg/web"})

	// Assert
	assert.Equal(t, []string{"pkg.api.claude.skills.lint"}, resultField(api, "name"), "the nearer lint shadows the root's")
	assert.Equal(t, []any{"pkg/api", "."}, api["scopes"])
	assert.Equal(t, []string{"lint"}, resultField(web, "name"))
	assert.Equal(t, []string{"ui"}, resultField(scoped, "name"))
}

func TestServeSkills_FindAndLoadContextOverStdio(t *testing.T) {
	// Arrange
	p := startServe(t, monorepoServeProject(t), "--discover-skills", "--no-watch")
	p.initialize()

	// Act
	found := p.toolResult("find_context", map[string]any{"path": "pkg/api/handler.go"})
	loaded := p.toolResult("load_context", map[string]any{"scope": "pkg/api", "name": "AGENTS.md"})
	read := p.call("resources/read", map[string]any{"uri": "context://pkg/api/reviewer"})

	// Assert
	assert.Equal(t, []string{"reviewer", "AGENTS.md", "AGENTS.md"}, resultField(found, "name"))
	assert.Equal(t, []string{"pkg/api", "pkg/api", "."}, resultField(found, "scope"))
	assert.Contains(t, loaded["content"], "Wrap every error.")
	contents := read["result"].(map[string]any)["contents"].([]any)
	assert.Contains(t, contents[0].(map[string]any)["text"], "Review carefully.")
}

func TestServeSkills_ContextToolsAreAbsentWithoutDiscovery(t *testing.T) {
	// Arrange
	p := startServe(t, monorepoServeProject(t), "--no-watch")
	p.initialize()

	// Act
	resp := p.call("tools/call", map[string]any{"name": "find_context", "arguments": map[string]any{}})

	// Assert
	result, _ := resp["result"].(map[string]any)
	assert.True(t, resp["error"] != nil || result["isError"] == true, "find_context is not a tool without discovery: %v", resp)
}

func TestServeSkills_ExcludeFlagAndConfigurationOverStdio(t *testing.T) {
	// Arrange: the flag excludes the api package, the configuration the web package.
	files := map[string]string{
		"skills/own/SKILL.md":                   serveSkill("own", "The own skill of ai-rulez", "Own body.", ""),
		"@pkg/api/.claude/skills/lint/SKILL.md": serveSkill("lint", "Lint the api package", "Api lint.", ""),
		"@pkg/web/skills/ui/SKILL.md":           serveSkill("ui", "Build a web ui component", "Ui body.", ""),
		"@pkg/web/AGENTS.md":                    "# Web conventions\n",
		"@pkg/api/AGENTS.md":                    "# API conventions\n",
		"@AGENTS.md":                            "# Root conventions\n",
	}
	root := serveProject(t, discoverServeConfig+"\n[mcp]\nexclude = [\"pkg/web/**\"]\n", files)
	p := startServe(t, root, "--discover-skills", "--no-watch", "--exclude", "pkg/api/**")
	p.initialize()

	// Act
	all := p.toolResult("find_context", map[string]any{"scope": ".", "recursive": true})

	// Assert
	assert.Equal(t, []string{"AGENTS.md"}, resultField(all, "name"))
	assert.Empty(t, p.load("lint"), "excluded by the flag")
	assert.Empty(t, p.load("ui"), "excluded by the configuration")
	assert.Contains(t, p.load("own"), "Own body.")
}
