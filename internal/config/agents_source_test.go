package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

const agentsTestConfig = "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n"

func writeAgentsFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestValidateSource(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"unset is the default", Config{}, ""},
		{"ai-rulez", Config{Source: SourceAIRulez}, ""},
		{"agents", Config{Source: SourceAgents}, ""},
		{"unknown", Config{Source: "claude"}, `unknown source "claude"`},
		{"agents layout implies agents", Config{AgentsLayout: true}, ""},
		{"agents layout with agents", Config{AgentsLayout: true, Source: SourceAgents}, ""},
		{"agents layout contradicts ai-rulez", Config{AgentsLayout: true, Source: SourceAIRulez}, "contradicts a configuration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.cfg.validateSource()
			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSourceRoundTripsThroughTOML(t *testing.T) {
	// Arrange
	cfg, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\n"), "config.toml")
	require.NoError(t, err)
	// Act
	out, err := MarshalTOML(cfg)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, SourceAgents, cfg.Source)
	assert.Contains(t, string(out), "source = 'agents'")

	plain, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"x\"\n"), "config.toml")
	require.NoError(t, err)
	written, err := MarshalTOML(plain)
	require.NoError(t, err)
	assert.NotContains(t, string(written), "source", "the default source is not written")
}

func TestAgentsConfigDirNames(t *testing.T) {
	tests := []struct {
		dir      string
		isAgents bool
		base     string
	}{
		{"/p/.agents", true, "/p"},
		{"/p/.agents/.ai-rulez", true, "/p"},
		{"/p/.ai-rulez", false, "/p"},
		{"/p/.config/ai-rulez", false, "/p"},
		{"/p/other/.ai-rulez", false, "/p/other"},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			assert.Equal(t, tt.isAgents, IsAgentsConfigDir(tt.dir))
			assert.Equal(t, filepath.FromSlash(tt.base), ProjectBaseDir(filepath.FromSlash(tt.dir)))
		})
	}
}

func TestDiscoveryPrefersAgentsConfig(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantDir string
	}{
		{"ai-rulez only", map[string]string{".ai-rulez/config.toml": agentsTestConfig}, ".ai-rulez"},
		{"agents only", map[string]string{".agents/ai-rulez.toml": agentsTestConfig}, ".agents"},
		{"both prefer agents", map[string]string{
			".ai-rulez/config.toml": agentsTestConfig, ".agents/ai-rulez.toml": agentsTestConfig,
		}, ".agents"},
		{"an .agents directory without the config is not a config", map[string]string{
			".agents/skills/a/SKILL.md": "x", ".ai-rulez/config.toml": agentsTestConfig,
		}, ".ai-rulez"},
		{"nothing", map[string]string{".agents/skills/a/SKILL.md": "x"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, content := range tt.files {
				writeAgentsFile(t, root, rel, content)
			}
			// Act
			dir := ResolveConfigDirName(root)
			found, findErr := FindConfigFile(root)
			// Assert
			assert.Equal(t, tt.wantDir, dir)
			if tt.wantDir == "" {
				assert.Error(t, findErr)
				return
			}
			require.NoError(t, findErr)
			name := "config.toml"
			if tt.wantDir == ".agents" {
				name = "ai-rulez.toml"
			}
			assert.Equal(t, filepath.Join(evalDir(t, root), tt.wantDir, name), evalFile(t, found))
		})
	}
}

func evalDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

func evalFile(t *testing.T, file string) string {
	t.Helper()
	return filepath.Join(evalDir(t, filepath.Dir(file)), filepath.Base(file))
}

func TestLoadAgentsLayoutPlacesStateBesideTheConfig(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".agents/ai-rulez.toml", agentsTestConfig)
	writeAgentsFile(t, root, ".agents/.ai-rulez/local/context/mine.md", "personal\n")
	reader := func(_ context.Context, _ *Config, tree *ContentTree) (*ContentTree, error) { return tree, nil }
	opts := []LoadOption{WithResolvers(Resolvers{Agents: reader})}

	entries := map[string]func() (*Config, error){
		"LoadConfig":        func() (*Config, error) { return LoadConfig(context.Background(), root, opts...) },
		"LoadConfigFromDir": func() (*Config, error) { return LoadConfigFromDir(context.Background(), root, ".agents", opts...) },
		"state directory": func() (*Config, error) {
			return LoadConfigFromDir(context.Background(), root, ".agents/.ai-rulez", opts...)
		},
		"config file path": func() (*Config, error) {
			return LoadConfigFromFile(context.Background(), filepath.Join(root, ".agents", "ai-rulez.toml"), opts...)
		},
		"agents folder path": func() (*Config, error) {
			return LoadConfigFromFile(context.Background(), filepath.Join(root, ".agents"), opts...)
		},
	}
	for name, load := range entries {
		t.Run(name, func(t *testing.T) {
			// Act
			cfg, err := load()
			// Assert
			require.NoError(t, err)
			assert.True(t, cfg.AgentsLayout)
			assert.True(t, cfg.UsesAgentsSource())
			assert.Equal(t, evalDir(t, root), evalDir(t, cfg.BaseDir))
			if cfg.Workspace != nil {
				_, relErr := workspace.Rel(cfg.Workspace, cfg.BaseDir)
				assert.NoError(t, relErr, "the workspace must contain the project directory")
			}
			assert.Equal(t, filepath.Join(evalDir(t, root), ".agents", ".ai-rulez"), filepath.Join(evalDir(t, filepath.Dir(cfg.ConfigDir)), filepath.Base(cfg.ConfigDir)))
			assert.Equal(t, ".agents/.ai-rulez", cfg.ConfigDirName)
			assert.Equal(t, AgentsConfigFilename, cfg.ConfigFile)
			assert.Equal(t, filepath.Join(evalDir(t, root), ".agents", "ai-rulez.toml"), evalFile(t, cfg.ConfigPath()))
			assert.Equal(t, filepath.Join(evalDir(t, root), ".agents"), filepath.Join(evalDir(t, filepath.Dir(cfg.SourceDir())), filepath.Base(cfg.SourceDir())))
			require.NotNil(t, cfg.LocalContent)
			require.Len(t, cfg.LocalContent.Context, 1, "machine-local content comes from the state directory")
		})
	}
}

func TestSourceAgentsWithAIRulezConfig(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\npresets = [\"claude\"]\n")
	writeAgentsFile(t, root, ".ai-rulez/rules/ignored.md", "# ignored\n")
	writeAgentsFile(t, root, ".agents/rules/style.md", "# Style\n")
	writeAgentsFile(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\ndescription: Review code\n---\nBody\n")
	reader := func(_ context.Context, _ *Config, tree *ContentTree) (*ContentTree, error) { return tree, nil }

	// Act
	cfg, err := LoadConfig(context.Background(), root, WithResolvers(Resolvers{Agents: reader}))

	// Assert
	require.NoError(t, err)
	assert.False(t, cfg.AgentsLayout)
	assert.True(t, cfg.UsesAgentsSource())
	assert.Equal(t, ".ai-rulez", cfg.ConfigDirName)
	require.Len(t, cfg.Content.Rules, 1, "the .ai-rulez content tree is not read")
	assert.Equal(t, "style", cfg.Content.Rules[0].Name)
	require.Len(t, cfg.Content.Skills, 1)
	assert.Equal(t, "review", cfg.Content.Skills[0].Name)
}

func TestSourceAgentsNeedsTheReader(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\n")

	// Act
	_, err := LoadConfig(context.Background(), root)

	// Assert
	assert.ErrorContains(t, err, "agents-source reader is unavailable")
}

func TestAgentsLayoutRejectsSourceAIRulez(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".agents/ai-rulez.toml", agentsTestConfig+"source = \"ai-rulez\"\n")
	reader := func(_ context.Context, _ *Config, tree *ContentTree) (*ContentTree, error) { return tree, nil }
	cfg, err := LoadConfig(context.Background(), root, WithResolvers(Resolvers{Agents: reader}))
	require.NoError(t, err)

	// Act
	err = cfg.Validate()

	// Assert
	assert.ErrorContains(t, err, "contradicts a configuration")
}

func TestSaveConfigWritesTheAgentsConfigFile(t *testing.T) {
	for _, dir := range []string{".agents", ".agents/.ai-rulez"} {
		t.Run(dir, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			cfg, err := decodeConfigTOML([]byte(agentsTestConfig), "ai-rulez.toml")
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".agents"), 0o755))

			// Act
			err = SaveConfig(cfg, filepath.Join(root, filepath.FromSlash(dir)))

			// Assert
			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(root, ".agents", "ai-rulez.toml"))
			assert.NoFileExists(t, filepath.Join(root, ".agents", "config.toml"))
			assert.NoFileExists(t, filepath.Join(root, ".agents", ".ai-rulez", "config.toml"))
		})
	}
}

func TestSwallowedSourceKey(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{"a source setting written after a table is swallowed", "version = \"5.0\"\nname = \"x\"\n\n[[plugins]]\nname = \"p\"\nmarketplace = \"m\"\nsource = \"agents\"\n", true},
		{"a source field that names no source stays an unknown key of its table", "version = \"5.0\"\nname = \"x\"\n\n[[plugins]]\nname = \"p\"\nmarketplace = \"m\"\nsource = \"github.com/example/plugin\"\n", false},
		{"a top-level source is fine", "version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\n\n[[plugins]]\nname = \"p\"\nmarketplace = \"m\"\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := decodeConfigTOML([]byte(tt.toml), "config.toml")
			// Assert
			if tt.wantErr {
				assert.ErrorContains(t, err, "`source` is a top-level key")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLegacyScanIgnoresForeignAgentsFiles(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".agents/config.json", "{}")

	// Act
	legacy := FindLegacyConfig(root)

	// Assert
	assert.Empty(t, legacy, ".agents holds other tools' files; a config.json there is not a V3 ai-rulez config")
}

func TestLayersOfAnAgentsProjectLiveInTheStateDirectory(t *testing.T) {
	tests := []struct {
		name      string
		layerPath string
		wantName  string
	}{
		{"state directory", ".agents/.ai-rulez/config.team.toml", "layered"},
		{"beside ai-rulez.toml", ".agents/config.team.toml", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsFile(t, root, ".agents/ai-rulez.toml", agentsTestConfig)
			writeAgentsFile(t, root, tt.layerPath, "name = \"layered\"\n")

			// Act
			cfg, err := LoadConfig(context.Background(), root, WithLayers("team"), WithResolvers(Resolvers{Agents: keepTree}))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, cfg.Name)
		})
	}
}

func TestMissingDefaultLayerNamesTheDirectoryItGoesIn(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsFile(t, root, ".agents/ai-rulez.toml", agentsTestConfig+"default_layers = [\"team\"]\n")

	// Act
	_, err := LoadConfig(context.Background(), root, WithResolvers(Resolvers{Agents: keepTree}))

	// Assert
	require.Error(t, err)
	assert.Contains(t, oopsHint(err), filepath.Join(".agents", ".ai-rulez", "config.team.toml"))
}

func oopsHint(err error) string {
	var oe oops.OopsError
	if errors.As(err, &oe) {
		return oe.Hint()
	}
	return ""
}

func keepTree(_ context.Context, _ *Config, tree *ContentTree) (*ContentTree, error) {
	return tree, nil
}
