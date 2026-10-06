package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestValidateStrictVerifiers_ReportsAR9HFindings(t *testing.T) {
	// Arrange
	extra := "[[verifiers]]\nname = \"readme\"\ntype = \"file_exists\"\npath = \"MISSING.md\"\n" +
		"[[verifiers]]\nname = \"cmd\"\nrule = \"r\"\nwhen_changed = [\"*.txt\"]\n[verifiers.require.command]\nargv = [\"sh\", \"-c\", \"exit 1\"]\n" +
		"[[verifiers]]\nname = \"model\"\nrule = \"r\"\nwhen_changed = [\"*.txt\"]\n[verifiers.require.llm]\nchecklist = [\"x\"]\n"
	root, cfg := strictProject(t, extra, map[string]string{".ai-rulez/rules/r.md": "# R\n\nA rule.\n", "a.txt": "x\n"})
	_ = root
	t.Cleanup(func() { validateVerifiers = false })

	validateVerifiers = false
	without := lintProject(t, cfg)
	validateVerifiers = true
	with := lintProject(t, cfg)

	// Assert
	codes := func(r *lint.Report) map[string]lint.Severity {
		out := map[string]lint.Severity{}
		for _, f := range r.Findings {
			if strings.HasPrefix(f.Code, "AR9H") {
				out[f.Code] = f.Severity
			}
		}
		return out
	}
	assert.Empty(t, codes(without), "verifiers are opt-in for validate")
	got := codes(with)
	assert.Equal(t, lint.SeverityError, got["AR9H1"], "the flat verifier fails at its own severity")
	assert.Equal(t, lint.SeverityInfo, got["AR9H3"], "validate never runs a command: it says so at info")
	assert.Equal(t, lint.SeverityInfo, got["AR9H4"], "validate never calls a model")
}

func TestCheckStrictFlags_VerifiersNeedsStrict(t *testing.T) {
	t.Cleanup(func() { validateStrict, validateVerifiers = false, false })
	validateStrict, validateVerifiers = false, true
	require.Error(t, checkStrictFlags())
	validateStrict = true
	require.NoError(t, checkStrictFlags())
}
