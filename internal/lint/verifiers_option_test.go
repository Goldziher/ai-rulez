package lint

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithVerifiersReportsTheSuppliedFindings(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFiles(t, root, deliveryFixture(""))
	gitAdd(t, root)
	cfg := loadCfg(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	file := filepath.Join(root, "db", "0042.sql")

	// Act
	rep, err := Run(cfg, tree, WithVerifiers([]VerifierFinding{
		{Code: CodeVerifierFailed, Severity: SeverityError, File: file, Line: 3, Message: "migrations-have-down: missing -- down"},
		{Code: CodeVerifierLLM, Message: "llm verifier skipped"},
	}))

	// Assert
	require.NoError(t, err)
	var failed, skipped *Finding
	for i := range rep.Findings {
		switch rep.Findings[i].Code {
		case CodeVerifierFailed:
			failed = &rep.Findings[i]
		case CodeVerifierLLM:
			skipped = &rep.Findings[i]
		}
	}
	require.NotNil(t, failed)
	assert.Equal(t, SeverityError, failed.Severity, "the verifier's own severity wins over the rule default")
	assert.Equal(t, 3, failed.Line)
	assert.Contains(t, failed.File, "0042.sql")
	require.NotNil(t, skipped)
	assert.Equal(t, SeverityInfo, skipped.Severity)
}

func TestWithoutVerifiersNoAR9HFindingAppears(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, deliveryFixture(""))
	gitAdd(t, root)
	cfg := loadCfg(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)

	rep, err := Run(cfg, tree)

	require.NoError(t, err)
	for _, code := range []string{CodeVerifierFailed, CodeVerifierInvalid, CodeVerifierCommand, CodeVerifierLLM, CodeVerifierDeadScope, CodeVerifierNoExample} {
		assert.Zero(t, countCode(rep.Findings, code), code)
	}
}
