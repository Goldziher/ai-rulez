package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestSkillSourceRoot(t *testing.T) {
	// Arrange: a plain skill directory and a git working tree.
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	checkout := filepath.Join(root, "checkout")
	require.NoError(t, os.MkdirAll(plain, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, ".git"), 0o755))
	home := filepath.Join(root, "home")
	cfg := &config.Config{Host: ambient.Host{Env: ambient.MapEnv{Home: home}}}

	tests := []struct {
		name     string
		source   string
		wantKind SourceType
		wantRoot string
	}{
		{"home directory", "~", SourceTypeLocal, home},
		{"path below home", "~/skills", SourceTypeLocal, filepath.Join(home, "skills")},
		{"file url of a plain directory", "file://" + filepath.ToSlash(plain), SourceTypeLocal, plain},
		{"file url of a git checkout stays git", "file://" + filepath.ToSlash(checkout), SourceTypeGit, "file://" + filepath.ToSlash(checkout)},
		{"https url", "https://github.com/o/r", SourceTypeGit, "https://github.com/o/r"},
		{"relative path", "vendor/skills", SourceTypeLocal, "vendor/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			kind, got, err := SkillSourceRoot(cfg, tt.source)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantRoot, got)
		})
	}

	t.Run("home without a home directory", func(t *testing.T) {
		_, _, err := SkillSourceRoot(&config.Config{Host: ambient.Host{Env: ambient.MapEnv{}}}, "~/skills")
		require.Error(t, err)
	})
}

func TestResolveInstalledSkills_SourceForms(t *testing.T) {
	// Arrange: <root>/project (the project) and <root>/home/skills/demo (a skill
	// outside it, reached as ~/skills).
	t.Setenv(lockfile.EnvAllowFileURLs, "")
	root := t.TempDir()
	project := filepath.Join(root, "project")
	home := filepath.Join(root, "home")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "inside", "demo"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "skills", "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "inside", "demo", "SKILL.md"), []byte("# Inside\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "skills", "demo", "SKILL.md"), []byte("# Home\n"), 0o644))
	env := ambient.MapEnv{Home: home}
	overlay := &config.LocalOverlay{Doc: map[string]any{"installed_skills": []any{map[string]any{"name": "demo", "source": "~/skills"}}}}

	tests := []struct {
		name     string
		source   string
		cfg      config.Config
		wantErr  string
		wantBody string
	}{
		{"home path from the local overlay", "~/skills", config.Config{LocalOverlay: overlay}, "", "# Home"},
		{"home path in user scope", "~/skills", config.Config{UserScope: true}, "", "# Home"},
		{"home path refused in a committed config", "~/skills", config.Config{}, "outside the project", ""},
		{"file url of a directory inside the project", "file://" + filepath.ToSlash(filepath.Join(project, "inside")), config.Config{}, "", "# Inside"},
		{"file url of a directory outside the project", "file://" + filepath.ToSlash(filepath.Join(home, "skills")), config.Config{}, "outside the project", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := tt.cfg
			cfg.BaseDir = project
			cfg.Host = ambient.Host{Env: env}
			cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: "demo", Source: tt.source, Path: "."}}

			// Act
			skills, err := ResolveInstalledSkills(t.Context(), &cfg, "")

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, skills, 1)
			assert.Contains(t, skills[0].Content, tt.wantBody)
		})
	}

	t.Run("committed home path carries a hint", func(t *testing.T) {
		cfg := &config.Config{BaseDir: project, Host: ambient.Host{Env: env},
			InstalledSkills: []config.InstalledSkillConfig{{Name: "demo", Source: "~/skills", Path: "."}}}

		_, err := ResolveInstalledSkills(t.Context(), cfg, "")

		require.Error(t, err)
		oopsErr, ok := oops.AsOops(err)
		require.True(t, ok)
		assert.Contains(t, oopsErr.Hint(), "config.local.toml")
	})
}
