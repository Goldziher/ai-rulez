package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/progress"
)

// badOverlay loads fine but violates the local overlay schema (unknown header style).
const badOverlay = "[header]\nstyle = \"weird\"\n"

func TestValidateConfigFile_OverlaySchema(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		wantErr bool
	}{
		{"valid overlay", "description = \"mine\"\n", false},
		{"schema-invalid overlay names the overlay file", badOverlay, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), ".ai-rulez")
			writeFile(t, filepath.Join(dir, "config.toml"), validRootConfig)
			overlayPath := filepath.Join(dir, "config.local.toml")
			writeFile(t, overlayPath, tt.overlay)

			// Act
			err := validateConfigFile(filepath.Join(dir, "config.toml"))

			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), overlayPath)
		})
	}
}

func TestValidateLocalOverlay_NoOverlayIsOK(t *testing.T) {
	assert.NoError(t, validateLocalOverlay(&config.Config{}))
}

func TestRunRecursiveValidate_BadOverlayFails(t *testing.T) {
	// Arrange
	root := twoRoots(t, validRootConfig)
	writeFile(t, filepath.Join(root, "b", ".ai-rulez", "config.local.toml"), badOverlay)
	t.Cleanup(func() { progress.SetQuiet(false) })

	// Act
	code := runRecursiveValidate()

	// Assert
	assert.Equal(t, 1, code)
}

func TestLocalOverlaySummary_CountsKeysAndHidesValues(t *testing.T) {
	// Arrange
	dir := filepath.Join(t.TempDir(), ".ai-rulez")
	writeFile(t, filepath.Join(dir, "config.toml"), localCmdShared)
	writeFile(t, filepath.Join(dir, "config.local.toml"),
		"name = \"mine\"\ndescription = \"SECRETISH\"\n\n[[mcp_servers]]\nname = \"gh\"\nremove = true\n")
	cfg, err := config.LoadConfig(t.Context(), filepath.Dir(dir))
	require.NoError(t, err)

	// Act
	line := localOverlaySummary(cfg)

	// Assert
	assert.Contains(t, line, "1 overridden, 1 added, 1 removed")
	assert.NotContains(t, line, "SECRETISH")
}
