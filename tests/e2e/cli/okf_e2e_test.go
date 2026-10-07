package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	internaltestutil "github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOKFRoundTripE2E exports a project as an OKF bundle, checks it, lints it
// and imports it into a second project, through the built binary.
func TestOKFRoundTripE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	src := minimalProject(t, "")
	bundle := filepath.Join(t.TempDir(), "bundle")
	dst := t.TempDir()
	writeTree(t, dst, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"dst\"\npresets = [\"claude\"]\n"})

	// Act and Assert: export, then --check is clean, then drift exits 2.
	exp := env.run(src, "export", "okf", "-o", bundle)
	require.Equal(t, 0, exp.ExitCode, exp.Stderr)
	assert.FileExists(t, filepath.Join(bundle, "index.md"))
	assert.FileExists(t, filepath.Join(bundle, "skills", "deploy", "SKILL.md"))
	clean := env.run(src, "export", "okf", "-o", bundle, "--check")
	assert.Equal(t, 0, clean.ExitCode, clean.Stdout+clean.Stderr)

	// Lint the bundle.
	lint := env.run(src, "okf", "validate", bundle, "--format", "json")
	require.Equal(t, 0, lint.ExitCode, lint.Stderr)
	doc := requireJSONDoc(t, lint)
	assert.Equal(t, "0.2", doc["okf_spec"])
	assert.Equal(t, float64(2), doc["concepts"])
	assert.Equal(t, []any{}, doc["findings"])

	// Import: a dry run writes nothing, the real run writes both concepts, a rerun is a no-op.
	dry := env.run(dst, "import", "okf", bundle, "--dry-run", "--format", "json")
	require.Equal(t, 0, dry.ExitCode, dry.Stderr)
	assert.Equal(t, true, requireJSONDoc(t, dry)["dry_run"])
	assert.NoFileExists(t, filepath.Join(dst, ".ai-rulez", "rules", "local.md"))
	imp := env.run(dst, "import", "okf", bundle, "--format", "json")
	require.Equal(t, 0, imp.ExitCode, imp.Stderr)
	actions, ok := requireJSONDoc(t, imp)["actions"].([]any)
	require.True(t, ok, imp.Stdout)
	assert.Len(t, actions, 2)
	assert.FileExists(t, filepath.Join(dst, ".ai-rulez", "rules", "local.md"))
	assert.FileExists(t, filepath.Join(dst, ".ai-rulez", "skills", "deploy", "SKILL.md"))
	again := env.run(dst, "import", "okf", bundle)
	require.Equal(t, 0, again.ExitCode, again.Stderr)
	assert.Contains(t, again.Stdout, "2 unchanged")

	// Drift: a hand edit to the bundle is reported by --check with exit 2.
	f, err := os.OpenFile(filepath.Join(bundle, "index.md"), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("\nhand edit\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	drift := env.run(src, "export", "okf", "-o", bundle, "--check")
	assert.Equal(t, 2, drift.ExitCode, drift.Stdout+drift.Stderr)
}

func okfBundle(t *testing.T, concept string) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"index.md": "---\nokf_version: \"0.2\"\n---\n\n# Concepts\n\n* [A](a.md) - a concept\n",
		"a.md":     concept,
	})
	return dir
}

const okfGoodConcept = "---\ntype: Decision\ntitle: A\ndescription: a concept\n---\n\n# A\n\nBody.\n"

func TestOKFValidateExitCodesE2E(t *testing.T) {
	tests := []struct {
		name     string
		bundle   func(t *testing.T) string
		args     []string
		wantExit int
		wantCode string
	}{
		{name: "a conforming bundle passes", bundle: func(t *testing.T) string { return okfBundle(t, okfGoodConcept) }},
		{
			name:     "a concept without a type is an error",
			bundle:   func(t *testing.T) string { return okfBundle(t, "---\ntitle: A\n---\n\n# A\n") },
			wantExit: 2, wantCode: "AR9B1",
		},
		{
			name: "a broken link is a warning that passes the default gate",
			bundle: func(t *testing.T) string {
				return okfBundle(t, okfGoodConcept+"\nSee [missing](nope.md).\n")
			},
			wantCode: "AR9B2",
		},
		{
			name: "--fail-on warning turns the broken link into exit 2",
			bundle: func(t *testing.T) string {
				return okfBundle(t, okfGoodConcept+"\nSee [missing](nope.md).\n")
			},
			args:     []string{"--fail-on", "warning"},
			wantExit: 2, wantCode: "AR9B2",
		},
		{
			name: "a symlink in the bundle is refused",
			bundle: func(t *testing.T) string {
				dir := okfBundle(t, okfGoodConcept)
				outside := filepath.Join(t.TempDir(), "secret.md")
				require.NoError(t, os.WriteFile(outside, []byte("---\ntype: Secret\n---\nsecret\n"), 0o600))
				internaltestutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "b.md"))
				return dir
			},
			wantExit: 2, wantCode: "AR9B8",
		},
		{
			name:     "a path that is not a directory cannot be read",
			bundle:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			wantExit: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			dir := tt.bundle(t)

			// Act
			res := env.run(t.TempDir(), append([]string{"okf", "validate", dir, "--format", "json"}, tt.args...)...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			if tt.wantExit == 1 {
				return
			}
			doc := requireJSONDoc(t, res)
			if tt.wantCode != "" {
				assert.Contains(t, res.Stdout, tt.wantCode)
			} else {
				assert.Equal(t, []any{}, doc["findings"])
			}
		})
	}
}

// TestOKFValidateEmptyDirectoryE2E pins RV-CLI-7: an empty directory is not an
// OKF bundle and must not pass.
func TestOKFValidateEmptyDirectoryE2E(t *testing.T) {
	blockedOn(t, "RV-CLI-7")
	env := newIsoEnv(t)

	res := env.run(t.TempDir(), "okf", "validate", t.TempDir())

	assert.NotEqual(t, 0, res.ExitCode, res.Stdout)
}

// TestOKFImportRefusesASymlinkedConceptE2E: a bundle cannot smuggle a file from
// outside itself into .ai-rulez/ through a symlink.
func TestOKFImportRefusesASymlinkedConceptE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	bundle := okfBundle(t, okfGoodConcept)
	outside := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(outside, []byte("---\ntype: Decision\ntitle: Secret\n---\n\nTOPSECRET\n"), 0o600))
	internaltestutil.SymlinkOrSkip(t, outside, filepath.Join(bundle, "b.md"))
	dst := t.TempDir()
	writeTree(t, dst, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"dst\"\npresets = [\"claude\"]\n"})

	// Act
	res := env.run(dst, "import", "okf", bundle, "--format", "json")

	// Assert: nothing under .ai-rulez carries the outside file's text.
	var leaked []string
	require.NoError(t, filepath.Walk(filepath.Join(dst, ".ai-rulez"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // test walk
		if rerr == nil && strings.Contains(string(data), "TOPSECRET") {
			leaked = append(leaked, path)
		}
		return rerr
	}))
	assert.Empty(t, leaked, "exit %d, stdout: %s", res.ExitCode, res.Stdout)
}

// TestOKFImportJSONEmptyListsE2E pins MAN-6: import okf --format json prints
// null instead of [] for findings, security and skipped when there are none.
func TestOKFImportJSONEmptyListsE2E(t *testing.T) {
	blockedOn(t, "MAN-6")
	// Arrange
	env := newIsoEnv(t)
	bundle := okfBundle(t, okfGoodConcept)
	dst := t.TempDir()
	writeTree(t, dst, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"dst\"\npresets = [\"claude\"]\n"})

	// Act
	res := env.run(dst, "import", "okf", bundle, "--dry-run", "--format", "json")

	// Assert
	require.Equal(t, 0, res.ExitCode, res.Stderr)
	doc := requireJSONDoc(t, res)
	for _, key := range []string{"findings", "security", "skipped"} {
		assert.Equal(t, []any{}, doc[key], "%s is an empty list, not null", key)
	}
}
