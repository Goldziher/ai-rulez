package generator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestGenerator_OutputsCarryingMCPSecretsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	tests := []struct {
		name     string
		existing os.FileMode // 0 means the file does not exist before generation
	}{
		{"fresh files", 0},
		{"existing world-readable files are tightened", 0o644},
	}
	secretFiles := []string{".mcp.json", ".claude/settings.json", ".gemini/settings.json", "opencode.json"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := writeHeadersProject(t, `["claude", "gemini", "opencode"]`, true,
				`{ Authorization = "Bearer ${API_TOKEN}", X-Team = "core" }`)
			if tt.existing != 0 {
				for _, f := range secretFiles {
					p := filepath.Join(dir, filepath.FromSlash(f))
					require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
					require.NoError(t, os.WriteFile(p, []byte("{}\n"), tt.existing))
					require.NoError(t, os.Chmod(p, tt.existing))
				}
			}
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			cfg.MCPEnvOverrides = map[string]string{"API_TOKEN": "supersecret-token"}

			// Act
			require.NoError(t, NewGenerator(cfg).Generate("default"))

			// Assert
			for _, f := range secretFiles {
				info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f)))
				require.NoError(t, err, f)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), f)
			}
		})
	}
}

func TestGenerator_UnchangedSecretOutputIsTightenedWithoutRewrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	// Arrange: generate once, loosen the mode, generate again (content unchanged)
	dir := writeHeadersProject(t, `["claude"]`, true, `{ Authorization = "Bearer ${API_TOKEN}" }`)
	generate := func() {
		cfg, err := config.LoadConfig(context.Background(), dir)
		require.NoError(t, err)
		cfg.MCPEnvOverrides = map[string]string{"API_TOKEN": "supersecret-token"}
		require.NoError(t, NewGenerator(cfg).Generate("default"))
	}
	generate()
	path := filepath.Join(dir, ".mcp.json")
	require.NoError(t, os.Chmod(path, 0o644))

	// Act
	generate()

	// Assert
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestGenerator_FilesWithoutSecretsKeepDefaultMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	// Arrange: the header value is not a placeholder and not a sensitive name
	dir := writeHeadersProject(t, `["claude"]`, true, `{ X-Team = "core" }`)
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)

	// Act
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// Assert
	info, err := os.Stat(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0o600), info.Mode().Perm())
}
