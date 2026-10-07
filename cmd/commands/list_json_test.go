package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An empty list in JSON mode is the JSON array [], not nothing: a script piping
// `--format json` into jq must not fail on a project with nothing to list.
func TestListCommands_EmptyListPrintsAnEmptyJSONArray(t *testing.T) {
	tests := []struct {
		name string
		json *bool
		run  func()
	}{
		{"list rules", &listJSON, func() { runListRules(listRulesCmd, nil) }},
		{"list context", &listJSON, func() { runListContext(listContextCmd, nil) }},
		{"list skills", &listJSON, func() { runListSkills(listSkillsCmd, nil) }},
		{"list agents", &listJSON, func() { listAgentsCmd.Run(listAgentsCmd, nil) }},
		{"list commands", &listJSON, func() { listCommandsCmd.Run(listCommandsCmd, nil) }},
		{"list checks", &listJSON, func() { listChecksCmd.Run(listChecksCmd, nil) }},
		{"skill list", &skillJSON, func() { runSkillList(skillListCmd, nil) }},
		{"include list", &includeJSON, func() { runIncludeList(includeListCmd, nil) }},
		{"profile list", &profileJSON, func() { runProfileList(profileListCmd, nil) }},
		{"domain list", &domainJSON, func() { runDomainList(domainListCmd, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a project with a config and nothing else
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", "")
			resetContentFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
				"version = \"4.0\"\nname = \"empty\"\npresets = [\"claude\"]\n")
			chdir(t, root)
			prev := *tt.json
			*tt.json = true
			t.Cleanup(func() { *tt.json = prev })

			// Act
			out := captureStdout(t, tt.run)

			// Assert
			var got []any
			require.NoError(t, json.Unmarshal([]byte(out), &got), "stdout = %q", out)
			assert.Empty(t, got)
		})
	}
}
