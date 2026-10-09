package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSBOMFormatIsTextOrJSON(t *testing.T) {
	// Arrange
	sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})

	// Act
	code, _, _ := runSBOMWith(sbomFlags{docType: "cyclonedx", format: "cyclonedx"})

	// Assert: cyclonedx names a document type now, not an output format
	assert.Equal(t, 1, code)
}

func TestSBOMJSONReportsTheCheck(t *testing.T) {
	// Arrange
	root := sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})
	committed := filepath.Join(root, "sbom.cdx.json")
	check := sbomFlags{docType: "cyclonedx", format: formatJSON, output: committed, check: true}

	// Act and assert: missing, written, in sync, drifted
	code, out, _ := runSBOMWith(check)
	assert.Equal(t, exitDrift, code)
	doc := decodeSBOMReport(t, out)
	assert.Equal(t, "drift", doc.Status)
	require.Len(t, doc.Findings, 1)
	assert.Equal(t, "AR753", doc.Findings[0].Code)

	code, out, _ = runSBOMWith(sbomFlags{docType: "cyclonedx", format: formatJSON, output: committed})
	require.Equal(t, 0, code)
	assert.Equal(t, "ok", decodeSBOMReport(t, out).Status)

	code, out, _ = runSBOMWith(check)
	require.Equal(t, 0, code)
	assert.Equal(t, "ok", decodeSBOMReport(t, out).Status)

	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "r2.md"), []byte("# R2\n"), 0o600))
	code, out, _ = runSBOMWith(check)
	assert.Equal(t, exitDrift, code)
	doc = decodeSBOMReport(t, out)
	assert.Equal(t, "drift", doc.Status)
	assert.NotEmpty(t, doc.Differences)
}

func TestSBOMJSONReportsAFailedGate(t *testing.T) {
	// Arrange
	sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})

	// Act
	code, out, errOut := runSBOMWith(sbomFlags{docType: "cyclonedx", format: formatJSON, requireLock: true})

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Empty(t, errOut)
	doc := decodeSBOMReport(t, out)
	assert.Equal(t, "findings", doc.Status)
	require.Len(t, doc.Findings, 1)
	assert.Equal(t, "AR752", doc.Findings[0].Code)
}

func TestSBOMJSONWithoutOutputPrintsTheDocument(t *testing.T) {
	// Arrange
	sbomProject(t, sbomBaseConfig, map[string]string{"rules/r.md": "# R\n"})

	// Act
	code, out, errOut := runSBOMWith(sbomFlags{docType: "spdx-json", format: formatJSON})

	// Assert
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, `"spdxVersion": "SPDX-2.3"`)
}

func decodeSBOMReport(t *testing.T, out string) sbomReport {
	t.Helper()
	var doc sbomReport
	require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
	assert.Equal(t, 1, doc.SchemaVersion)
	return doc
}
