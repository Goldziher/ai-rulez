package generator

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_SecretOnlyInIgnoredSensitiveFiles(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["vibe", "claude", "cursor"]`)

	// Act
	generateRepo(t, root)

	// Assert: every file holding the secret is gitignored by git itself and
	// owner-only.
	var holders []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".git" {
				return fs.SkipDir
			}
			return err
		}
		if data, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(data, []byte(leakToken)) {
			rel, _ := filepath.Rel(root, path)
			holders = append(holders, filepath.ToSlash(rel))
			info, _ := d.Info()
			if runtime.GOOS != "windows" { // Windows reports 0o666 for every file
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), rel)
			}
		}
		return nil
	}))
	require.NotEmpty(t, holders)
	for _, rel := range holders {
		assert.True(t, gitIgnores(t, root, rel), "%s holds the secret and must be gitignored", rel)
	}
}

func TestGenerate_CheckFileIsNotMistakenForMCPConfig(t *testing.T) {
	// Arrange: cursor merges .cursor/BUGBOT.md (a check) and writes mcp.json with
	// an env reference; the secret never reaches BUGBOT.md.
	root := newSecretMCPRepo(t, `["cursor"]`)

	// Act + Assert: generate must not refuse because of the check file.
	generateRepo(t, root)
	bugbot, err := os.ReadFile(filepath.Join(root, ".cursor", "BUGBOT.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(bugbot), leakToken)
}

func gitIgnores(t *testing.T, root, rel string) bool {
	t.Helper()
	return gitutil.CommandNoContext(root, "check-ignore", "-q", rel).Run() == nil
}
