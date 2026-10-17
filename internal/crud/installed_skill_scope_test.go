package crud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallSkill_ScopeAndVersionFields(t *testing.T) {
	req := InstallSkillRequest{
		Name: "pack", Source: "https://github.com/owner/repo", Version: "^1.2", TagPrefix: "v",
		IncludePrerelease: true, MinReleaseAge: "7d", Domain: "backend", Profiles: []string{"dev", "ci"},
		LocalOverride: "vendor/pack",
	}
	want := []string{
		"tag_prefix = 'v'", "include_prerelease = true", "min_release_age = '7d'", "domain = 'backend'",
		"local_override = 'vendor/pack'", "'dev'", "'ci'", "version = '^1.2'",
	}
	tests := []struct {
		name  string
		local bool
		file  string
	}{
		{name: "shared config", file: "config.toml"},
		{name: "local overlay", local: true, file: "config.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)
			if tt.local {
				op = op.Local()
			}
			install := req

			// Act
			err = op.InstallSkill(t.Context(), &install)

			// Assert
			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", tt.file))
			require.NoError(t, err)
			for _, s := range want {
				assert.Contains(t, string(data), s)
			}
		})
	}
}

func TestInstallSkill_RejectsABadVersionKey(t *testing.T) {
	tests := []struct {
		name string
		req  InstallSkillRequest
		want string
	}{
		{name: "tag prefix without a version", req: InstallSkillRequest{Name: "x", Source: "owner/repo", TagPrefix: "v"}, want: "tag_prefix"},
		{name: "bad release age", req: InstallSkillRequest{Name: "x", Source: "owner/repo", Version: "^1", MinReleaseAge: "soon"}, want: "min_release_age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestProject(t)
			op, err := NewOperator(dir)
			require.NoError(t, err)

			err = op.InstallSkill(t.Context(), &tt.req)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
