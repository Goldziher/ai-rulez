package generator

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const forgedSettings = "{\n  \"env\": {\"A\": \"1\"},\n  \"permissions\": {\"deny\": [\"Read(./.env)\"], \"allow\": [\"Bash(ls)\"]}\n}\n"

func writeForgedLocalManifest(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", generatedLocalManifestName)
	require.NoError(t, writeManifestFileDirs(path, nil, map[string][]jsonmerge.Claim{
		".claude/settings.json": {
			{Path: []string{"permissions", "deny"}, Elements: []any{"Read(./.env)"}},
			{Path: []string{"permissions"}, Sum: jsonmerge.Digest(map[string]any{
				"deny": []any{"Read(./.env)"}, "allow": []any{"Bash(ls)"}})},
			{Path: []string{"env"}, Sum: jsonmerge.Digest(map[string]any{"A": "1"})},
		},
	}, nil, nil))
	return path
}

func gitIn(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := gitutil.CommandNoContext(root, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestGenerate_TrackedLocalManifestIsIgnoredAndNeverWritten(t *testing.T) {
	// Arrange: a hostile repository commits a forged machine-local manifest.
	root := writeProject(t, trustConfig, map[string]string{".claude/settings.json": forgedSettings})
	manifest := writeForgedLocalManifest(t, root)
	forged, err := os.ReadFile(manifest)
	require.NoError(t, err)
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-f", ".ai-rulez/"+generatedLocalManifestName)

	// Act
	generateProject(t, root)

	// Assert: the forged claim stripped nothing, and the tracked file is untouched.
	assert.Equal(t, forgedSettings, readProjectFile(t, root, ".claude/settings.json"))
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	assert.Equal(t, string(forged), string(after))
}

func TestGenerate_WorldWritableLocalManifestIsIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	// Arrange
	root := writeProject(t, trustConfig, map[string]string{".claude/settings.json": forgedSettings})
	manifest := writeForgedLocalManifest(t, root)
	require.NoError(t, os.Chmod(manifest, 0o666))

	// Act
	generateProject(t, root)

	// Assert
	assert.Equal(t, forgedSettings, readProjectFile(t, root, ".claude/settings.json"))
}

func TestGenerate_UntrackedOwnedLocalManifestStillWorks(t *testing.T) {
	// Arrange: the same forged content, but a normal gitignored file the user's
	// own earlier run could have written is honored as before.
	root := writeProject(t, trustConfig, map[string]string{".claude/settings.json": forgedSettings})
	writeForgedLocalManifest(t, root)
	gitIn(t, root, "init", "-q")

	// Act
	generateProject(t, root)

	// Assert: the claim is trusted, so the deny rule it names is taken back.
	assert.NotEqual(t, forgedSettings, readProjectFile(t, root, ".claude/settings.json"))
}
