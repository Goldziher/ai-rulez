package toolnames

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateMatcher(t *testing.T) {
	tests := []struct {
		name     string
		harness  string
		matcher  string
		want     string
		unmapped string
	}{
		// single tool
		{"cursor single", "cursor", "Bash", "^Shell$", ""},
		{"gemini single", "gemini", "Bash", "^run_shell_command$", ""},
		{"copilot single is not wrapped (vendor anchors)", "copilot", "Bash", "bash", ""},
		{"copilot-cli single", "copilot-cli", "Write", "create", ""},
		{"factory single", "factory", "Bash", "^Execute$", ""},
		{"devin single", "devin", "Bash", "^exec$", ""},
		{"augment single", "augment", "Bash", "^launch-process$", ""},
		{"antigravity single", "antigravity", "Bash", "^run_command$", ""},
		// alternation, duplicates collapse
		{"cursor alternation collapses Write", "cursor", "Edit|Write", "^Write$", ""},
		{"gemini alternation", "gemini", "Edit|Write", "^replace$|^write_file$", ""},
		{"gemini read spans two tools", "gemini", "Read", "^read_file$|^read_many_files$", ""},
		{"copilot alternation", "copilot", "Edit|Write", "edit|create", ""},
		{"factory alternation", "factory", "Edit|Write|Bash", "^Edit$|^Create$|^Execute$", ""},
		// anchored
		{"devin anchored", "devin", "^Bash$", "^exec$", ""},
		{"copilot anchored keeps anchors", "copilot", "^Bash$", "^bash$", ""},
		{"factory anchored alternation", "factory", "^Edit$|^Bash$", "^Edit$|^Execute$", ""},
		// match all
		{"cursor wildcard", "cursor", ".*", "*", ""},
		{"gemini wildcard", "gemini", ".*", ".*", ""},
		{"devin wildcard has no documented spelling", "devin", "*", "", "*"},
		// remaining harnesses
		{"kiro shell category", "kiro", "Bash", "shell", ""},
		{"kiro read spans several tools so is unmapped", "kiro", "Read", "", "Read"},
		{"kiro alternation is unmapped", "kiro", "Bash|Read", "", "Bash|Read"},
		{"goose single", "goose", "Bash", "^shell$", ""},
		{"goose mcp", "goose", "mcp__github__.*", "^github__.*$", ""},
		{"goose wildcard", "goose", "*", ".*", ""},
		{"crush alternation", "crush", "Read|Task", "^view$|^agent$", ""},
		{"crush mcp", "crush", "mcp__github__.*", "^mcp_github_.*$", ""},
		{"crush websearch undocumented", "crush", "WebSearch", "", "WebSearch"},
		{"cortex notebook", "cortex", "NotebookEdit", "^notebook_edit_cell$", ""},
		{"cortex mcp passthrough", "cortex", "mcp__github__.*", "^mcp__github__.*$", ""},
		{"poolside bare name is exact", "poolside", "Bash", "shell", ""},
		{"poolside pipe list is exact", "poolside", "Edit|Write", "edit|write", ""},
		{"poolside anchored anchors every token", "poolside", "^Bash$|Edit", "^shell$|^edit$", ""},
		{"poolside mcp undocumented", "poolside", "mcp__a__b", "", "mcp__a__b"},
		{"vibe bash", "vibe", "Bash", "bash", ""},
		{"vibe glob has no alternation", "vibe", "Bash|Grep", "", "Bash|Grep"},
		{"vibe wildcard", "vibe", ".*", "*", ""},
		// mcp
		{"factory mcp passthrough", "factory", "mcp__github__.*", "^mcp__github__.*$", ""},
		{"devin mcp tool", "devin", "mcp__github__create_issue", "^mcp__github__create_issue$", ""},
		{"gemini mcp rewrites separator", "gemini", "mcp__github__.*", "^mcp_github_.*$", ""},
		{"gemini mcp mixed with builtin", "gemini", "Bash|mcp__github__.*", "^run_shell_command$|^mcp_github_.*$", ""},
		{"gemini mcp any server", "gemini", "mcp__.*__search", "^mcp_.*_search$", ""},
		{"cursor mcp is undocumented", "cursor", "mcp__github__.*", "", "mcp__github__.*"},
		{"copilot mcp is undocumented", "copilot", "mcp__github__.*", "", "mcp__github__.*"},
		{"mcp with a complex regex is unmapped", "factory", "mcp__(a|b)__.*", "", "mcp__(a"},
		// unmapped tokens skip the whole matcher
		{"unmapped tool", "devin", "Bash|WebSearch", "", "WebSearch"},
		{"unmapped tool alone", "cursor", "NotebookEdit", "", "NotebookEdit"},
		{"unknown name", "gemini", "Frobnicate", "", "Frobnicate"},
		{"regex outside the grammar", "gemini", "Ba.*", "", "Ba.*"},
		{"empty token", "gemini", "Bash|", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			vocab, ok := For(tt.harness)
			require.True(t, ok, "no vocabulary for %s", tt.harness)

			// Act
			got, unmapped, ok := vocab.TranslateMatcher(tt.matcher)

			// Assert
			if tt.unmapped != "" || tt.want == "" {
				assert.False(t, ok)
				assert.Equal(t, tt.unmapped, unmapped)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNoAlternationVocabularyRejectsSeveralTools(t *testing.T) {
	// Arrange
	vocab := &Vocabulary{Harness: "x", NoAlternation: true, Tools: map[string][]string{Bash: {"sh"}, Read: {"cat"}}}

	// Act
	single, _, singleOK := vocab.TranslateMatcher("Bash")
	_, _, multiOK := vocab.TranslateMatcher("Bash|Read")

	// Assert
	assert.True(t, singleOK)
	assert.Equal(t, "sh", single)
	assert.False(t, multiOK)
}

func TestInverseListsClaudeNamesInCanonicalOrder(t *testing.T) {
	// Arrange
	vocab, _ := For("opencode")

	// Act
	inverse := vocab.Inverse()

	// Assert
	assert.Equal(t, []string{"Edit", "MultiEdit", "Write"}, inverse["apply_patch"])
	assert.Equal(t, []string{"Edit", "MultiEdit"}, inverse["edit"])
	assert.Equal(t, []string{"Task", "Agent"}, inverse["task"])
}

func TestEveryVocabularyCitesItsSource(t *testing.T) {
	for _, harness := range Harnesses() {
		vocab, _ := For(harness)
		assert.NotEmpty(t, vocab.Source, harness)
		assert.NotEmpty(t, vocab.Tools, harness)
		for claude := range vocab.Tools {
			assert.Contains(t, ClaudeTools, claude, "%s maps unknown Claude tool %s", harness, claude)
		}
	}
}
