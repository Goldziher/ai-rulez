package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func hardeningSkill(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["SKILL.md"] = "---\nname: s\ndescription: d\n---\n# s\n"
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return dir
}

func TestInstalledSkillBundle_NeverBundlesCredentialsOrHiddenTrees(t *testing.T) {
	tests := []struct {
		name     string
		skipped  string
		wantWarn bool
	}{
		{"rsa key", "id_rsa", true},
		{"ed25519 key", "id_ed25519", true},
		{"nested pem", "docs/server.pem", true},
		{"key file", "keys/app.key", true},
		{"pkcs12", "cert.p12", true},
		{"pfx", "cert.pfx", true},
		{"jks keystore", "store.keystore", true},
		{"credentials.json", "credentials.json", true},
		{"terraform state", "infra/terraform.tfstate", true},
		{"env file", ".env", false},
		{"env variant", ".env.production", false},
		{"nested hidden dir", "docs/.git/config", false},
		{"nested hidden dir file", ".github/workflows/ci.yml", false},
		{"node_modules", "tools/node_modules/pkg/index.js", false},
		{"nested evals", "docs/evals/case.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := hardeningSkill(t, map[string]string{"keep.md": "# keep\n", tt.skipped: "secret"})
			log := &recordingLogger{}

			// Act
			resources, err := LoadSkillResourcesContext(logger.WithContext(context.Background(), log), dir)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, []string{"keep.md"}, resourcePaths(resources))
			if tt.wantWarn {
				assert.Contains(t, strings.Join(log.warns, "\n"), filepath.Base(tt.skipped))
			}
		})
	}
}

func TestInstalledSkillBundle_CredentialsStayOutEvenWithCustomResources(t *testing.T) {
	dir := hardeningSkill(t, map[string]string{"keep.md": "# keep\n", "id_rsa": "k", "assets/server.pem": "k"})
	ctx := WithSkillResources(context.Background(), []string{"*", "assets"})

	resources, err := LoadSkillResourcesContext(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, []string{"keep.md"}, resourcePaths(resources))
}

func TestInstalledSkillBundle_SymlinkOutsideAndFIFONeverBundled(t *testing.T) {
	// Arrange
	dir := hardeningSkill(t, map[string]string{"keep.md": "# keep\n"})
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, "link.md"))
	testutil.SymlinkOrSkip(t, filepath.Dir(outside), filepath.Join(dir, "linkdir"))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644))

	// Act
	resources, err := LoadSkillResourcesContext(context.Background(), dir)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"keep.md"}, resourcePaths(resources))
}

func TestInstalledSkillBundle_OversizeFileIsSkippedWithAWarning(t *testing.T) {
	// Arrange
	dir := hardeningSkill(t, map[string]string{
		"keep.md":         "# keep\n",
		"big.bin":         strings.Repeat("x", maxContentFileBytes+1),
		"assets/huge.bin": strings.Repeat("x", maxContentFileBytes+1),
	})
	log := &recordingLogger{}

	// Act
	resources, err := LoadSkillResourcesContext(logger.WithContext(context.Background(), log), dir)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"keep.md"}, resourcePaths(resources))
	warned := strings.Join(log.warns, "\n")
	assert.Contains(t, warned, "big.bin")
	assert.Contains(t, warned, "huge.bin")
}

func TestInstalledSkillBundle_TotalCapFailsNamingTheLimit(t *testing.T) {
	tests := []struct {
		name  string
		files func() map[string]string
		want  string
	}{
		{"too many files", func() map[string]string {
			files := map[string]string{}
			for i := range maxSkillBundleFiles + 1 {
				files[fmt.Sprintf("docs/f%04d.md", i)] = "x"
			}
			return files
		}, fmt.Sprintf("%d files", maxSkillBundleFiles)},
		{"too many bytes", func() map[string]string {
			chunk := strings.Repeat("x", maxContentFileBytes-1)
			return map[string]string{"a.bin": chunk, "b.bin": chunk, "c.bin": chunk}
		}, fmt.Sprintf("%d MiB", maxSkillBundleBytes>>20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := hardeningSkill(t, tt.files())

			_, err := LoadSkillResourcesContext(context.Background(), dir)

			require.Error(t, err)
			var limit *SkillBundleLimitError
			require.True(t, errors.As(err, &limit), err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "resources")
		})
	}
}

func TestInstalledSkillBundle_NarrowingResourcesStaysUnderTheCap(t *testing.T) {
	files := map[string]string{"keep/a.md": "x"}
	for i := range maxSkillBundleFiles + 1 {
		files[fmt.Sprintf("docs/f%04d.md", i)] = "x"
	}
	dir := hardeningSkill(t, files)
	ctx := WithSkillResources(context.Background(), []string{"*", "!docs", "!evals", "!agents", "!.*"})

	resources, err := LoadSkillResourcesContext(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, []string{"keep/a.md"}, resourcePaths(resources))
}
