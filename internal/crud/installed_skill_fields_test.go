package crud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallSkill_AllFields(t *testing.T) {
	tests := []struct {
		name    string
		local   bool
		req     InstallSkillRequest
		wantErr string
		want    []string // substrings of the written file
	}{
		{
			name: "shared config keeps every field",
			req: InstallSkillRequest{
				Name: "pack", Source: "owner/repo", Include: []string{"a*", "b"}, Exclude: []string{"*-wip"},
				NamePrefix: "p-", Trust: "warn", Resources: []string{"evals/**"}, Version: "^1.2",
			},
			want: []string{"source = 'owner/repo'", "name_prefix = 'p-'", "trust = 'warn'", "version = '^1.2'", "evals/**", "a*", "*-wip"},
		},
		{
			name:  "local overlay keeps every field",
			local: true,
			req: InstallSkillRequest{
				Name: "pack", Source: "~/skills", Include: []string{"a*"}, Trust: "warn", Resources: []string{"evals/**"},
			},
			want: []string{"source = '~/skills'", "trust = 'warn'", "evals/**", "a*"},
		},
		{name: "invalid trust", req: InstallSkillRequest{Name: "x", Source: "owner/repo", Trust: "loud"}, wantErr: "trust"},
		{name: "invalid glob", req: InstallSkillRequest{Name: "x", Source: "owner/repo", Include: []string{"[bad"}}, wantErr: "glob"},
		{name: "version with ref", req: InstallSkillRequest{Name: "x", Source: "https://h/r", Ref: "main", Version: "^1"}, wantErr: "version"},
		{name: "absolute resource", req: InstallSkillRequest{Name: "x", Source: "owner/repo", Resources: []string{"/etc/passwd"}}, wantErr: "resources"},
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

			// Act
			err = op.InstallSkill(t.Context(), &tt.req)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			file := filepath.Join(dir, ".ai-rulez", "config.toml")
			if tt.local {
				file = filepath.Join(dir, ".ai-rulez", "config.local.toml")
			}
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			for _, s := range tt.want {
				assert.Contains(t, string(data), s)
			}
		})
	}
}

func TestInstallSkill_KeepsExistingShorthand(t *testing.T) {
	// Arrange
	dir := setupTestProject(t)
	op, err := NewOperator(dir)
	require.NoError(t, err)
	require.NoError(t, op.InstallSkill(t.Context(), &InstallSkillRequest{Name: "one", Source: "github:owner/one"}))

	// Act
	require.NoError(t, op.InstallSkill(t.Context(), &InstallSkillRequest{Name: "two", Source: "https://github.com/owner/two"}))

	// Assert
	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "github:owner/one")
}
