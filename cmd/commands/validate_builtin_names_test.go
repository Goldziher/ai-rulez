package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestStrictValidateChecksEveryHarnessThatReadsTheOutputDirectory(t *testing.T) {
	// Arrange: only cursor is enabled, and it writes skills to .agents/skills, which Gemini CLI also reads
	cfg := deliveryProject(t, `["cursor"]`, "", map[string]string{
		"skills/help/SKILL.md": "---\nname: help\ndescription: Use when testing the reach of the strict validate options.\n---\nbody\n",
	})
	tree, err := lint.LoadTree(filepath.Dir(cfg.ConfigDir))
	require.NoError(t, err)

	// Act
	rep, err := lint.Run(cfg, tree, contentLintOptions(t.Context(), cfg, nil)...)

	// Assert
	require.NoError(t, err)
	var gemini bool
	for _, f := range rep.Findings {
		if f.Code == lint.CodeBuiltinNameCollision && f.Severity == lint.SeverityWarning {
			gemini = gemini || assert.Contains(t, f.Message, "gemini built-in command /help") && assert.Contains(t, f.Message, ".agents/skills")
		}
	}
	assert.True(t, gemini, "an enabled preset need not be the one that collides (a reader is a warning): %+v", rep.Findings)
}
