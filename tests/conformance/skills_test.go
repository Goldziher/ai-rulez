package conformance

import (
	"bytes"
	"regexp"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Agent Skills (https://agentskills.io/specification) has no JSON Schema; its
// SKILL.md rules are written out here from the specification.
var skillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// checkSkill asserts that content is a valid SKILL.md for the skill directory dir.
func checkSkill(t *testing.T, dir string, content []byte) {
	t.Helper()
	rest, ok := bytes.CutPrefix(content, []byte("---\n"))
	require.True(t, ok, "%s: SKILL.md must start with YAML frontmatter", dir)
	raw, _, ok := bytes.Cut(rest, []byte("\n---"))
	require.True(t, ok, "%s: the frontmatter is not closed", dir)

	var fm map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &fm), dir)

	name, _ := fm["name"].(string)
	assert.Equal(t, dir, name, "name must match the parent directory")
	assert.LessOrEqual(t, utf8.RuneCountInString(name), 64)
	assert.Regexp(t, skillName, name, "lowercase letters, digits and single hyphens only")

	description, _ := fm["description"].(string)
	assert.NotEmpty(t, description)
	assert.LessOrEqual(t, utf8.RuneCountInString(description), 1024)

	if v, ok := fm["compatibility"]; ok {
		s, isString := v.(string)
		assert.True(t, isString)
		assert.NotEmpty(t, s)
		assert.LessOrEqual(t, utf8.RuneCountInString(s), 500)
	}
	if v, ok := fm["license"]; ok {
		_, isString := v.(string)
		assert.True(t, isString)
	}
	if v, ok := fm["allowed-tools"]; ok {
		_, isString := v.(string)
		assert.True(t, isString, "allowed-tools is a space-separated string")
	}
	if v, ok := fm["metadata"]; ok {
		m, isMap := v.(map[string]any)
		require.True(t, isMap, "metadata is a map")
		for k, val := range m {
			_, isString := val.(string)
			assert.True(t, isString, "metadata.%s must be a string", k)
		}
	}
}

func TestGeneratedSkillsFollowTheAgentSkillsSpecification(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{
		".ai-rulez/config.toml":            "version = \"5.0\"\nname = \"conf\"\npresets = [\"claude\", \"codex\"]\n",
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Ship it.\nlicense: MIT\nmetadata:\n  owner: acme\n---\nDeploy.\n",
		".ai-rulez/skills/review/SKILL.md": "---\nname: review\ndescription: Review a diff.\n---\nReview.\n",
	})

	// Act
	run(t, dir, "generate", "--yes")

	// Assert
	for _, root := range []string{".claude/skills", ".agents/skills"} {
		for _, name := range []string{"deploy", "review"} {
			checkSkill(t, name, read(t, dir, root, name, "SKILL.md"))
		}
	}
}

func TestSkillRulesRejectInvalidSkills(t *testing.T) {
	assert.False(t, skillName.MatchString("Bad"))
	assert.False(t, skillName.MatchString("a--b"))
	assert.False(t, skillName.MatchString("-a"))
	assert.True(t, skillName.MatchString("code-review2"))
}
