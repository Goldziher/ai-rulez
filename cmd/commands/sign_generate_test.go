package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

func TestSkillSignatureGate(t *testing.T) {
	tests := []struct {
		name     string
		requires []string
		wantErr  string
	}{
		{name: "off when [signing] does not require skill"},
		{name: "unsigned installed skill is refused", requires: []string{"skill"}, wantErr: "AR720"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
			t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
			_, pub, err := sigstore.GenerateKeyPair(nil)
			require.NoError(t, err)
			dir := filepath.Join(root, ".ai-rulez", "skills", "deploy")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "keys"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: deploy\n---\nShip.\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "keys", "p.pub"), pub, 0o644))
			cfg := &config.Config{
				BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
				InstalledSkills: []config.InstalledSkillConfig{{Name: "deploy", Source: "https://example.org/s.git"}},
				Content:         &config.ContentTree{Skills: []config.ContentFile{{Name: "deploy", Path: filepath.Join(dir, "SKILL.md")}}},
				Signing: &config.SigningConfig{
					Require: tt.requires,
					Trust:   []config.SigningTrust{{Subject: "skill", Source: "deploy", KeyFile: "keys/p.pub"}},
				},
			}

			// Act
			err = skillSignatureGate(cfg)

			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "deploy")
		})
	}
}
