package commands

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const rootTestConfig = "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\ngitignore = false\n"

func resetDiscoveryFlags(t *testing.T) {
	t.Helper()
	cfgFile, configDir = "", ""
	t.Cleanup(func() { cfgFile, configDir = "", "" })
}

func TestLoadConfigOrNoConfig_WalksUpFromASubdirectory(t *testing.T) {
	layouts := []struct {
		name string
		path string
		body string
	}{
		{"ai-rulez dir", filepath.Join(".ai-rulez", "config.toml"), rootTestConfig},
		{"agents dir", filepath.Join(".agents", "ai-rulez.toml"), rootTestConfig},
		{"config convention", filepath.Join(".config", "ai-rulez", "config.toml"), rootTestConfig},
		{"root file", "ai-rulez.toml", rootTestConfig},
	}
	for _, tt := range layouts {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetDiscoveryFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, tt.path), tt.body)
			sub := filepath.Join(root, "a", "b")
			require.NoError(t, os.MkdirAll(sub, 0o755))
			t.Chdir(sub)

			// Act
			cfg, err := loadConfigOrNoConfig(context.Background())

			// Assert
			require.NoError(t, err)
			want, _ := filepath.EvalSymlinks(root)
			got, _ := filepath.EvalSymlinks(cfg.BaseDir)
			assert.Equal(t, want, got)
		})
	}
}

func TestLoadConfigOrNoConfig_NearestRootWins(t *testing.T) {
	resetDiscoveryFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootTestConfig)
	nested := filepath.Join(root, "svc")
	writeFile(t, filepath.Join(nested, "ai-rulez.toml"), "version = \"5.0\"\nname = \"svc\"\npresets = [\"claude\"]\n")
	deep := filepath.Join(nested, "pkg")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	t.Chdir(deep)

	cfg, err := loadConfigOrNoConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "svc", cfg.Name)
}

func TestLoadConfigOrNoConfig_StopsAtTheGitRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	resetDiscoveryFlags(t)
	outer := t.TempDir()
	writeFile(t, filepath.Join(outer, ".ai-rulez", "config.toml"), rootTestConfig)
	repo := filepath.Join(outer, "repo")
	sub := filepath.Join(repo, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, gitutil.CommandNoContext("", "init", "-q", repo).Run())
	t.Chdir(sub)

	_, err := loadConfigOrNoConfig(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, config.ErrNoConfig))
	assert.Contains(t, err.Error(), "searched")
	assert.Contains(t, err.Error(), "up to")
}

func TestLoadConfigOrNoConfig_ExplicitConfigWinsOverWalkUp(t *testing.T) {
	resetDiscoveryFlags(t)
	cwdRoot := t.TempDir()
	writeFile(t, filepath.Join(cwdRoot, "ai-rulez.toml"), "version = \"5.0\"\nname = \"cwd\"\npresets = [\"claude\"]\n")
	other := t.TempDir()
	writeFile(t, filepath.Join(other, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"other\"\npresets = [\"claude\"]\n")
	sub := filepath.Join(cwdRoot, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)
	cfgFile = filepath.Join(other, ".ai-rulez", "config.toml")

	cfg, err := loadConfigOrNoConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "other", cfg.Name)
}

func TestRunGenerate_FromASubdirectoryWritesRelativeToTheConfigRoot(t *testing.T) {
	resetDiscoveryFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootTestConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\n\nUse tabs.\n")
	sub := filepath.Join(root, "pkg", "deep")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	err := runGenerate(GenerateCmd, nil)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "CLAUDE.md"))
	assert.NoFileExists(t, filepath.Join(sub, "CLAUDE.md"))
}

func TestLoadConfigOrNoConfig_NoConfigNamesWhereItLooked(t *testing.T) {
	resetDiscoveryFlags(t)
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := loadConfigOrNoConfig(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, config.ErrNoConfig))
	assert.Contains(t, err.Error(), "searched "+dir)
}

func TestOpenOperator_WalksUpFromASubdirectory(t *testing.T) {
	layouts := []struct {
		name string
		path string
	}{
		{"ai-rulez dir", filepath.Join(".ai-rulez", "config.toml")},
		{"config convention", filepath.Join(".config", "ai-rulez", "config.toml")},
		{"root file", "ai-rulez.toml"},
	}
	for _, tt := range layouts {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetDiscoveryFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, tt.path), rootTestConfig)
			sub := filepath.Join(root, "a", "b")
			require.NoError(t, os.MkdirAll(sub, 0o755))
			t.Chdir(sub)

			// Act
			op, err := openOperator()

			// Assert
			require.NoError(t, err)
			want, _ := filepath.EvalSymlinks(root)
			got, _ := filepath.EvalSymlinks(op.BaseDir())
			assert.Equal(t, want, got)
		})
	}
}

func TestOpenOperator_NoConfigKeepsTheNotFoundError(t *testing.T) {
	// Arrange
	resetDiscoveryFlags(t)
	t.Chdir(t.TempDir())

	// Act
	_, err := openOperator()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory not found")
}
