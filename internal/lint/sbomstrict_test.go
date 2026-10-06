package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithSBOMReportsTheSuppliedFindingsAtTheirRegisteredSeverity(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	cfg := loadCfg(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)

	// Act
	rep, err := Run(cfg, tree, WithSBOM([]SBOMFinding{
		{Code: CodeSBOMUnpinned, Message: "mcp-server fs: floats"},
		{Code: CodeSBOMLockStale, Path: "ai-rulez.lock", Message: "the lock does not match the sources"},
		{Code: CodeSBOMDrift, Path: "sbom.cdx.json", Message: "differs from the SBOM generated now"},
	}))

	// Assert
	require.NoError(t, err)
	for code, wantSeverity := range map[string]Severity{CodeSBOMUnpinned: SeverityInfo, CodeSBOMLockStale: SeverityError, CodeSBOMDrift: SeverityError} {
		var found *Finding
		for i := range rep.Findings {
			if rep.Findings[i].Code == code {
				found = &rep.Findings[i]
			}
		}
		require.NotNil(t, found, code)
		assert.Equal(t, wantSeverity, found.Severity, code)
	}
	assert.Zero(t, countCode(rep.Findings, CodeSBOMUnknownCoords))
}

func TestSBOMFindingsAreSelectedWithTheConfigAnalyzer(t *testing.T) {
	// Arrange: only the lock analyzer runs, so the config analyzer's SBOM findings are not reported
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	cfg := loadCfg(t, root)
	tree, err := LoadTree(root)
	require.NoError(t, err)

	// Act
	rep, err := RunWith(cfg, tree, Options{Analyzers: []string{AnalyzerLock}}, WithSBOM([]SBOMFinding{{Code: CodeSBOMUnpinned, Message: "x"}}))

	// Assert
	require.NoError(t, err)
	assert.Zero(t, countCode(rep.Findings, CodeSBOMUnpinned))
}
