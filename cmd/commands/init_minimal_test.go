package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestInitMinimal_WritesOnlyAiRulezToml(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantName string
	}{
		{"named project", []string{"init", "--minimal", "demo"}, "demo"},
		{"project named after the directory", []string{"init", "--minimal"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "AGENTS.md"), "# Mine\n")
			chdir(t, root)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			require.Equal(t, 0, res.code, res.stdout+res.stderr)
			body := readFileString(t, filepath.Join(root, "ai-rulez.toml"))
			assert.True(t, strings.HasPrefix(body, "#:schema "))
			if tt.wantName != "" {
				assert.Contains(t, body, `name = "`+tt.wantName+`"`)
			}
			assert.Contains(t, res.stdout+res.stderr, "Created ai-rulez.toml")
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
			assert.Equal(t, "# Mine\n", readFileString(t, filepath.Join(root, "AGENTS.md")))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			assert.Len(t, entries, 2, "only AGENTS.md and ai-rulez.toml")
			cfg, err := config.LoadConfig(t.Context(), root, config.WithoutRemote())
			require.NoError(t, err)
			assert.True(t, cfg.Contentless())
			assert.Equal(t, 0, runCLI(t, "validate").code)
		})
	}
}

func TestInitMinimal_JSONReportsTheCreatedFile(t *testing.T) {
	// Arrange
	root := t.TempDir()
	chdir(t, root)

	// Act
	res := runCLI(t, "init", "--minimal", "demo", "--format", "json")

	// Assert
	require.Equal(t, 0, res.code, res.stdout+res.stderr)
	assert.Contains(t, res.stdout, `"status": "created"`)
	assert.Contains(t, res.stdout, `"path": "ai-rulez.toml"`)
}

func TestInitMinimal_RefusesToOverwriteWithoutForce(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	chdir(t, root)

	// Act
	refused := runCLI(t, "init", "--minimal", "other")
	forced := runCLI(t, "init", "--minimal", "other", "--force")

	// Assert
	assert.Equal(t, 1, refused.code)
	assert.Contains(t, refused.stdout+refused.stderr, "ai-rulez.toml already exists")
	require.Equal(t, 0, forced.code, forced.stdout+forced.stderr)
	assert.Contains(t, readFileString(t, filepath.Join(root, "ai-rulez.toml")), `name = "other"`)
	backups, err := filepath.Glob(filepath.Join(root, "ai-rulez.toml.bak-*"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	assert.Equal(t, rootLayoutConfig, readFileString(t, backups[0]))
}

func TestInitMinimal_RejectsFlagsThatScaffoldAContentTree(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"source agents", []string{"init", "--minimal", "--source", "agents"}, "cannot be combined with --source agents"},
		{"unknown source", []string{"init", "--minimal", "--source", "nope"}, "source"},
		{"from", []string{"init", "--minimal", "--from", "auto"}, "cannot be combined with --from"},
		{"domains", []string{"init", "--minimal", "--domains", "a,b"}, "cannot be combined with --domains"},
		{"config dir", []string{"init", "--minimal", "--config-dir", ".config/ai-rulez"}, "cannot be combined with --config-dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			chdir(t, root)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.Equal(t, 1, res.code, res.stdout+res.stderr)
			assert.Contains(t, res.stdout+res.stderr, tt.want)
			assert.NoFileExists(t, filepath.Join(root, "ai-rulez.toml"))
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
		})
	}
}

func TestInitMinimalConflicts(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"empty project", nil, nil},
		{"only an ai-rulez.toml", map[string]string{"ai-rulez.toml": rootLayoutConfig}, nil},
		{
			"a .ai-rulez directory wins",
			map[string]string{".ai-rulez/config.toml": rootLayoutConfig},
			[]string{".ai-rulez/ already holds a configuration and takes precedence: the ai-rulez.toml this writes will be ignored"},
		},
		{
			"the .config convention wins",
			map[string]string{".config/ai-rulez/config.toml": rootLayoutConfig},
			[]string{".config/ai-rulez/ already holds a configuration and takes precedence: the ai-rulez.toml this writes will be ignored"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}

			// Act
			got := initMinimalConflicts(root)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInitMinimal_StillWritesTheFileBesideAConfigDirectory(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootLayoutConfig)
	chdir(t, root)

	// Act
	res := runCLI(t, "init", "--minimal", "demo")

	// Assert: the directory still wins when the project loads (the warning is initMinimalConflicts')
	require.Equal(t, 0, res.code, res.stdout+res.stderr)
	assert.FileExists(t, filepath.Join(root, "ai-rulez.toml"))
	cfg, err := config.LoadConfig(t.Context(), root, config.WithoutRemote())
	require.NoError(t, err)
	assert.False(t, cfg.Contentless())
}
