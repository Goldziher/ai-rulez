package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const floatingMCP = "\n[[mcp_servers]]\nname = \"floaty\"\ncommand = \"npx\"\nargs = [\"-y\", \"some-mcp-server@latest\"]\n"

func sbomCodes(findings []lint.SBOMFinding) map[string]string {
	out := map[string]string{}
	for _, f := range findings {
		out[f.Code] += f.Message + "\n"
	}
	return out
}

func TestSBOMFindingsForReportsFloatingPackagesAndUnknownCoordinates(t *testing.T) {
	// Arrange: one MCP server floats on latest, another is a binary with no package
	lockProject(t, floatingMCP+"\n[[mcp_servers]]\nname = \"local\"\ncommand = \"/usr/local/bin/my-server\"\n")
	cfg := mustLoadConfig(t)

	// Act
	got := sbomCodes(sbomFindingsFor(cfg))

	// Assert
	assert.Contains(t, got["AR750"], "floaty")
	assert.Contains(t, got["AR751"], "local")
	assert.NotContains(t, got, "AR752", "no lock, no require-lock: an unlocked project is not an AR752 finding")
	assert.NotContains(t, got, "AR753", "no committed SBOM")
}

func TestSBOMFindingsForReportsAStaleLock(t *testing.T) {
	// Arrange: pin, then change a rule
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "style.md"), []byte("# Style\nUse spaces.\n"), 0o644))
	cfg := mustLoadConfig(t)

	// Act
	got := sbomCodes(sbomFindingsFor(cfg))

	// Assert
	assert.Contains(t, got["AR752"], "lock")
}

func TestSBOMFindingsForComparesACommittedSBOMWithAFreshOne(t *testing.T) {
	// Arrange: commit the SBOM, then change the configuration
	root := lockProject(t, "")
	var out, errOut bytes.Buffer
	require.Equal(t, 0, runSBOM(&out, &errOut, sbomFlags{format: "cyclonedx", output: filepath.Join(root, "sbom.cdx.json")}, false))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "style.md"), []byte("# Style\nUse spaces.\n"), 0o644))
	cfg := mustLoadConfig(t)

	// Act
	stale := sbomCodes(sbomFindingsFor(cfg))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "rules", "style.md"), []byte("# Style\nUse tabs.\n"), 0o644))
	fresh := sbomCodes(sbomFindingsFor(mustLoadConfig(t)))

	// Assert
	assert.Contains(t, stale["AR753"], "sbom.cdx.json")
	assert.NotContains(t, fresh, "AR753", "an SBOM that matches the configuration is not drift")
}

func TestSBOMFindingsForIgnoresADocumentAnotherToolMade(t *testing.T) {
	// Arrange
	root := lockProject(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "sbom.cdx.json"), []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[]}`), 0o644))

	// Act
	got := sbomCodes(sbomFindingsFor(mustLoadConfig(t)))

	// Assert
	assert.NotContains(t, got, "AR753")
}
