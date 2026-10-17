package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestNormalizeInstalledSkillSources(t *testing.T) {
	// Arrange: vendor/skills exists as a directory under the project.
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "vendor", "skills"), 0o755))

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"github prefix", "github:anthropics/skills", "https://github.com/anthropics/skills"},
		{"bare shorthand", "anthropics/skills", "https://github.com/anthropics/skills"},
		{"bare shorthand naming an existing directory stays local", "vendor/skills", "vendor/skills"},
		{"full https url", "https://github.com/anthropics/skills", "https://github.com/anthropics/skills"},
		{"scp url", "git@github.com:anthropics/skills.git", "git@github.com:anthropics/skills.git"},
		{"dot-relative path", "./vendor/skills", "./vendor/skills"},
		{"absolute path", "/srv/skills", "/srv/skills"},
		{"home path", "~/skills", "~/skills"},
		{"file url", "file:///srv/skills", "file:///srv/skills"},
		{"malformed github prefix is left for validation", "github:nope", "github:nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			skills := []InstalledSkillConfig{{Name: "s", Source: tt.source}}

			// Act
			NormalizeInstalledSkillSources(base, skills)

			// Assert
			assert.Equal(t, tt.want, skills[0].Source)
		})
	}
}

func TestLocalSkillSource(t *testing.T) {
	// Arrange
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	checkout := filepath.Join(root, "checkout")
	require.NoError(t, os.MkdirAll(plain, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, ".git"), 0o755))
	env := ambient.MapEnv{Home: filepath.Join(root, "home")}

	tests := []struct {
		name      string
		source    string
		env       ambient.Env
		wantDir   string
		wantLocal bool
		wantErr   bool
	}{
		{"home", "~", env, env.Home, true, false},
		{"below home", "~/skills", env, filepath.Join(env.Home, "skills"), true, false},
		{"no home directory", "~/skills", ambient.MapEnv{}, "", false, true},
		{"file url of a directory", "file://" + filepath.ToSlash(plain), env, plain, true, false},
		{"file url of a git checkout", "file://" + filepath.ToSlash(checkout), env, "file://" + filepath.ToSlash(checkout), false, false},
		{"https url", "https://github.com/o/r", env, "https://github.com/o/r", false, false},
		{"relative path", "vendor/skills", env, "vendor/skills", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			dir, local, err := LocalSkillSource(tt.env, tt.source)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDir, dir)
			assert.Equal(t, tt.wantLocal, local)
		})
	}
}
