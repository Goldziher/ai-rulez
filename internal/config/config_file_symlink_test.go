package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const symlinkTestConfig = "version = \"4.0\"\nname = \"proj\"\npresets = [\"claude\"]\n"

// A symlinked config.toml or config.local.toml is a config file the project did
// not author: when its target is outside the repository root it is refused, like
// a content symlink, and never read.
func TestLoadConfigRefusesConfigFilesLinkedOutsideTheProject(t *testing.T) {
	tests := []struct {
		name    string
		link    string // the file in .ai-rulez/ that becomes a symlink
		inside  bool   // the target lives inside the project
		wantErr bool
	}{
		{name: "config.toml linked outside", link: "config.toml", wantErr: true},
		{name: "config.local.toml linked outside", link: "config.local.toml", wantErr: true},
		{name: "config.toml linked inside is followed", link: "config.toml", inside: true},
		{name: "config.local.toml linked inside is followed", link: "config.local.toml", inside: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			cfgDir := filepath.Join(project, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			targetDir := t.TempDir()
			if tt.inside {
				targetDir = filepath.Join(project, "shared")
				require.NoError(t, os.MkdirAll(targetDir, 0o755))
			}
			target := filepath.Join(targetDir, "target.toml")
			content := symlinkTestConfig
			if tt.link == "config.local.toml" {
				content = "name = \"overlaid\"\n"
				require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(symlinkTestConfig), 0o644))
			}
			require.NoError(t, os.WriteFile(target, []byte(content), 0o644))
			testutil.SymlinkOrSkip(t, target, filepath.Join(cfgDir, tt.link))

			// Act
			cfg, err := LoadConfig(t.Context(), project, WithoutRemote())

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.link)
				assert.Contains(t, err.Error(), "outside the repository root")
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, cfg.Name)
		})
	}
}
