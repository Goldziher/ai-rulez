package commands

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/watch"
)

func TestWatchTargets_RootLayout(t *testing.T) {
	tests := []struct {
		name    string
		overlay bool
		want    func(root string) []watch.Target
	}{
		{
			"the file alone", false,
			func(root string) []watch.Target {
				return []watch.Target{{Path: filepath.Join(root, "ai-rulez.toml"), File: true}, {Path: filepath.Join(root, "shared"), File: false}}
			},
		},
		{
			"the file, its overlay and the local skill source", true,
			func(root string) []watch.Target {
				return []watch.Target{
					{Path: filepath.Join(root, "ai-rulez.toml"), File: true},
					{Path: filepath.Join(root, "ai-rulez.local.toml"), File: true},
					{Path: filepath.Join(root, "shared"), File: false},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutConfig+"\n[[installed_skills]]\nname = \"demo\"\nsource = \"https://github.com/example/skills\"\nlocal_override = \"./shared\"\n")
			if tt.overlay {
				writeFile(t, filepath.Join(root, "ai-rulez.local.toml"), "description = \"mine\"\n")
			}
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
			require.NoError(t, err)

			// Act
			got := watchTargets(cfg)

			// Assert: the project tree is never watched, only the config files and sources
			assert.Equal(t, tt.want(root), got)
		})
	}
}

func TestWatchTargets_RootLayoutWithIncludes(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutConfig+"\n[[includes]]\nname = \"shared\"\nsource = \"./inc\"\n")
	writeFile(t, filepath.Join(root, "inc", ".ai-rulez", "rules", "r.md"), "# r\n\nBe terse.\n")
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	require.True(t, cfg.Contentless())
	require.Len(t, cfg.Includes, 1)

	// Act
	got := watchTargets(cfg)

	// Assert: the config file and the include source, never the project tree
	assert.Equal(t, []watch.Target{
		{Path: filepath.Join(root, "ai-rulez.toml"), File: true},
		{Path: filepath.Join(root, "inc"), File: false},
	}, got)
}

func TestFallbackTargets_RootLayout(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  func(root string) []watch.Target
	}{
		{
			"ai-rulez.toml with an overlay",
			map[string]string{"ai-rulez.toml": rootLayoutConfig, "ai-rulez.local.toml": "description = \"x\"\n"},
			func(root string) []watch.Target {
				return []watch.Target{{Path: filepath.Join(root, "ai-rulez.toml"), File: true}, {Path: filepath.Join(root, "ai-rulez.local.toml"), File: true}}
			},
		},
		{
			"a config directory beats the file",
			map[string]string{"ai-rulez.toml": rootLayoutConfig, ".ai-rulez/config.toml": rootLayoutConfig},
			func(root string) []watch.Target { return []watch.Target{{Path: filepath.Join(root, ".ai-rulez")}} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetWatchFlags(t)
			root := t.TempDir()
			for rel, body := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}
			chdir(t, root)
			resolved, err := filepath.EvalSymlinks(root)
			require.NoError(t, err)

			// Act
			got := fallbackTargets()

			// Assert
			want := tt.want(resolved)
			require.Len(t, got, len(want))
			for i := range want {
				assert.Equal(t, want[i].File, got[i].File)
				gotPath, evalErr := filepath.EvalSymlinks(got[i].Path)
				require.NoError(t, evalErr)
				assert.Equal(t, want[i].Path, gotPath)
			}
		})
	}
}

func TestWatchIgnore_RootLayoutManifests(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/p/.ai-rulez-manifest.json", true},
		{"/p/.ai-rulez-manifest.local.json", true},
		{"/p/.generated-manifest.json", true},
		{"/p/.gitignore", true},
		{"/p/ai-rulez.toml", false},
		{"/p/ai-rulez.local.toml", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, watchIgnore(tt.path))
		})
	}
}
