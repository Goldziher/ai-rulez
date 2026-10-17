package mcp

// End-to-end check of `ai-rulez mcp --serve-skills --discover-skills` (issue 432):
// the skills a repository already keeps are served beside the ai-rulez ones.

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
