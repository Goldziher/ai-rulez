package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetContentFlags restores the package-level flag state the add, remove and
// list commands read, so tests do not leak into each other.
func resetContentFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		addDomain, addPriority, addTargets, addContent, addDesc, addLocal = "", "medium", "", "", "", false
		removeDomain, removeForce, removeLocal = "", false, false
		listDomain, listJSON, listLocal = "", false, false
	})
}

func TestLocalContentCommands_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		add    func()
		remove func()
		list   func()
		rel    string // below .ai-rulez/local
		kind   string
	}{
		{"rule", "", func() { runAddRule(addRuleCmd, []string{"item"}) },
			func() { runRemoveRule(removeRuleCmd, []string{"item"}) },
			func() { runListRules(listRulesCmd, nil) }, "rules/item.md", "rules"},
		{"context", "", func() { runAddContext(addContextCmd, []string{"item"}) },
			func() { runRemoveContext(removeContextCmd, []string{"item"}) },
			func() { runListContext(listContextCmd, nil) }, "context/item.md", "context"},
		{"skill", "", func() { runAddSkill(addSkillCmd, []string{"item"}) },
			func() { runRemoveSkill(removeSkillCmd, []string{"item"}) },
			func() { runListSkills(listSkillsCmd, nil) }, "skills/item/SKILL.md", "skills"},
		{"agent", "", func() { runAddAgent(addAgentCmd, []string{"item"}) },
			func() { _ = removeAgentCmd.RunE(removeAgentCmd, []string{"item"}) },
			func() { _ = listAgentsCmd.RunE(listAgentsCmd, nil) }, "agents/item.md", "agents"},
		{"command", "", func() { runAddCommand(addCommandCmd, []string{"item"}) },
			func() { _ = removeCommandCmd.RunE(removeCommandCmd, []string{"item"}) },
			func() { _ = listCommandsCmd.RunE(listCommandsCmd, nil) }, "commands/item.md", "commands"},
		{"rule in a domain", "team", func() { runAddRule(addRuleCmd, []string{"item"}) },
			func() { runRemoveRule(removeRuleCmd, []string{"item"}) },
			func() { runListRules(listRulesCmd, nil) }, "domains/team/rules/item.md", "rules"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetContentFlags(t)
			dir := localProject(t)
			localFile := filepath.Join(dir, "local", filepath.FromSlash(tt.rel))
			addLocal, removeLocal, listLocal = true, true, true
			addDomain, removeDomain, listDomain = tt.domain, tt.domain, tt.domain
			removeForce, listJSON = true, true

			// Act: add
			tt.add()

			// Assert: the file is below local/, the shared tree is untouched, list sees it
			assert.FileExists(t, localFile)
			assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(tt.rel)))
			out := captureStdout(t, tt.list)
			assert.Contains(t, out, `"name": "item"`)

			// The shared view does not list it.
			listLocal = false
			listDomain = ""
			shared := captureStdout(t, tt.list)
			assert.NotContains(t, shared, `"name": "item"`)
			listLocal, listDomain = true, tt.domain

			// Act: remove
			tt.remove()

			// Assert
			assert.NoFileExists(t, localFile)
		})
	}
}

func TestLocalContentCommands_LocalWithDomainWritesLocalDomain(t *testing.T) {
	// Arrange
	resetContentFlags(t)
	dir := localProject(t)
	addLocal, addDomain, addContent = true, "scratch", "Body text.\n"

	// Act
	runAddAgent(addAgentCmd, []string{"helper"})

	// Assert: no "cannot combine" error; the domain is created below local/.
	data, err := os.ReadFile(filepath.Join(dir, "local", "domains", "scratch", "agents", "helper.md"))
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(data), "\n\nBody text.\n"), "the body is kept: %q", data)
	assert.Contains(t, string(data), "type: Reference", "a local item is an OKF concept too")
	assert.NoFileExists(t, filepath.Join(dir, "local", "domains", "scratch", "agents", "index.md"), "the local tree has no indexes")
}
