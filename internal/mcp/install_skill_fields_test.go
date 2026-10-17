package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallSkillToolAcceptsEveryInstalledSkillField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		file string
		want []string
	}{
		{
			name: "shared config",
			args: map[string]any{
				"name": "pack", "source": "https://github.com/owner/repo", "version": "^1.2", "tag_prefix": "v",
				"include_prerelease": true, "min_release_age": "7d", "domain": "backend", "profiles": []string{"dev"},
				"include": []string{"a*"}, "exclude": []string{"*-wip"}, "name_prefix": "p-", "trust": "warn",
				"resources": []string{"evals/**"}, "local_override": "vendor/pack",
			},
			file: "config.toml",
			want: []string{
				"version = '^1.2'", "tag_prefix = 'v'", "include_prerelease = true", "min_release_age = '7d'",
				"domain = 'backend'", "'dev'", "'a*'", "'*-wip'", "name_prefix = 'p-'", "trust = 'warn'",
				"evals/**", "local_override = 'vendor/pack'",
			},
		},
		{
			name: "local overlay",
			args: map[string]any{"name": "pack", "source": "owner/repo", "local": true, "domain": "backend", "version": "^1"},
			file: "config.local.toml",
			want: []string{"domain = 'backend'", "version = '^1'"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := telemetryProject(t)
			session := connect(t, NewServer("test", WithAnyDirectory()))
			args := map[string]any{"working_directory": dir}
			for k, v := range tt.args {
				args[k] = v
			}

			// Act
			res := call(t, session, "install_skill", args)

			// Assert
			require.False(t, res.IsError, "%v", res.Content)
			data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", tt.file))
			require.NoError(t, err)
			for _, s := range tt.want {
				assert.Contains(t, string(data), s)
			}
		})
	}
}
