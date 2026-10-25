package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pathGatedSkill(name, paths string) string {
	return "---\nname: " + name + "\ndescription: Kafka consumer conventions for " + name + "\npaths:\n" + paths + "---\n\n# " + name + "\n"
}

func pathGatedProject(t *testing.T) string {
	t.Helper()
	root := project(t, discoverConfig, nil)
	for rel, body := range map[string]string{
		".claude/skills/kafka-conventions/SKILL.md":  pathGatedSkill("kafka-conventions", "  - \"backend/services/**/kafka*.py\"\n"),
		".claude/skills/py-conventions/SKILL.md":     pathGatedSkill("py-conventions", "  - \"**/*.py\"\n"),
		".claude/skills/web-conventions/SKILL.md":    pathGatedSkill("web-conventions", "  - \"products/web/**\"\n"),
		".claude/skills/plain-conventions/SKILL.md":  "---\nname: plain-conventions\ndescription: Kafka consumer conventions without paths\n---\n\n# plain\n",
		"products/web/.claude/skills/local/SKILL.md": pathGatedSkill("local", "  - \"src/**\"\n"),
	} {
		writeFile(t, root, rel, body)
	}
	writeFile(t, root, "products/web/src/x.py", "x = 1\n")
	return root
}

func TestFindSkill_PathGatedSkillsFollowTheQueryPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "a file the glob names", path: "backend/services/orders/kafka_consumer.py", want: []string{"kafka-conventions", "py-conventions", "plain-conventions"}},
		{name: "a file in another product", path: "products/web/src/x.py", want: []string{"py-conventions", "web-conventions", "plain-conventions", "local"}},
		{name: "a product directory drops other products' gated skills", path: "products/web", want: []string{"py-conventions", "web-conventions", "plain-conventions", "local"}},
		{name: "a directory the glob reaches into", path: "backend/services", want: []string{"kafka-conventions", "py-conventions", "plain-conventions"}},
		{name: "a directory no gated skill reaches", path: "docs", want: []string{"py-conventions", "plain-conventions"}},
		{name: "a file only unrestricted skills match", path: "docs/readme.md", want: []string{"plain-conventions"}},
		{name: "a nested skill's globs are relative to its scope", path: "products/web/other", want: []string{"py-conventions", "web-conventions", "plain-conventions"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newServerFor(t, &ServeSetup{WorkDir: pathGatedProject(t), DiscoverSkills: "repo"})
			session := connect(t, srv)

			// Act
			out, isErr := toolJSON(t, session, "find_skill", map[string]any{"task": "kafka consumer conventions", "path": tt.path, "limit": 20})

			// Assert
			require.False(t, isErr, "%v", out)
			assert.ElementsMatch(t, tt.want, resultNames(t, out))
		})
	}
}

func TestFindSkill_PathGatedSkillsAreUnchangedWithoutAPathFilter(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "no filter", args: map[string]any{}},
		{name: "a scope filter", args: map[string]any{"scope": ".", "recursive": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newServerFor(t, &ServeSetup{WorkDir: pathGatedProject(t), DiscoverSkills: "repo"})
			session := connect(t, srv)
			args := map[string]any{"task": "kafka consumer conventions", "limit": 20}
			for k, v := range tt.args {
				args[k] = v
			}

			// Act
			out, isErr := toolJSON(t, session, "find_skill", args)

			// Assert
			require.False(t, isErr, "%v", out)
			assert.Subset(t, resultNames(t, out), []string{"kafka-conventions", "py-conventions", "web-conventions", "plain-conventions"})
		})
	}
}

func TestLoadSkill_LoadsAPathGatedSkillByName(t *testing.T) {
	// Arrange
	srv := newServerFor(t, &ServeSetup{WorkDir: pathGatedProject(t), DiscoverSkills: "repo"})
	session := connect(t, srv)

	// Act
	out, isErr := toolJSON(t, session, "load_skill", map[string]any{"name": "kafka-conventions"})

	// Assert
	require.False(t, isErr, "%v", out)
}
