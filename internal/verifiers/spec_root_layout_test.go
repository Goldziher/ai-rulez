package verifiers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func rootLayoutVerifierConfig(t *testing.T, extra string, files map[string]string) *config.Config {
	t.Helper()
	root := t.TempDir()
	body := "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n" + extra
	files["ai-rulez.toml"] = body
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	require.True(t, cfg.Contentless())
	return cfg
}

func TestLoadSpecs_RootLayoutIgnoresAVerifiersDirectoryOfTheProject(t *testing.T) {
	// Arrange: a verifiers/ folder at the project root belongs to the project, not to ai-rulez.
	cfg := rootLayoutVerifierConfig(t, "", map[string]string{
		"verifiers/mine.toml": "[[verifiers]]\nid = \"v\"\nrule = \"ghost\"\n[verifiers.require.file_exists]\npath = \"a\"\n",
	})

	// Act
	specs, problems := LoadSpecs(cfg)

	// Assert
	assert.Empty(t, specs)
	assert.Empty(t, problems)
}

func TestLoadSpecs_RootLayoutReadsInlineSpecs(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  string // substring of the problem; "" means the spec loads
	}{
		{
			"a spec with no target",
			"\n[[verifiers]]\nname = \"readme\"\nskill = \"\"\nrule = \"\"\n",
			"exactly one of rule",
		},
		{
			"a spec naming a rule the root layout cannot have",
			"\n[[verifiers]]\nname = \"v\"\nrule = \"database\"\nwhen_changed = [\"*.sql\"]\n[verifiers.require.regex]\nregex = \"x\"\n",
			"does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := rootLayoutVerifierConfig(t, tt.extra, map[string]string{})

			// Act
			specs, problems := LoadSpecs(cfg)

			// Assert
			assert.Empty(t, specs)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, tt.want)
		})
	}
}

func TestRun_RootLayoutRunsPlainVerifiers(t *testing.T) {
	// Arrange
	cfg := rootLayoutVerifierConfig(t, "\n[[verifiers]]\nname = \"has-readme\"\ntype = \"file_exists\"\npath = \"README.md\"\n",
		map[string]string{"README.md": "# Solo\n"})

	// Act
	rep := Run(context.Background(), cfg, Options{})

	// Assert
	require.NoError(t, rep.Err)
	require.Len(t, rep.Results, 1)
	assert.Equal(t, "has-readme", rep.Results[0].Name)
	assert.Equal(t, StatusPass, rep.Results[0].Status)
	assert.False(t, rep.Failed(true))
}

func TestRun_RootLayoutReportsAFailingPlainVerifier(t *testing.T) {
	// Arrange
	cfg := rootLayoutVerifierConfig(t, "\n[[verifiers]]\nname = \"has-readme\"\ntype = \"file_exists\"\npath = \"README.md\"\n", map[string]string{})

	// Act
	rep := Run(context.Background(), cfg, Options{})

	// Assert
	require.NoError(t, rep.Err)
	require.Len(t, rep.Results, 1)
	assert.Equal(t, StatusFail, rep.Results[0].Status)
	assert.True(t, rep.Failed(false))
}
