package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitignoreExcludeDecodesAndRoundTrips(t *testing.T) {
	// Arrange
	src := "version = \"5.0\"\nname = \"x\"\ngitignore = true\ngitignore_exclude = [\"AGENTS.md\", \"**/CLAUDE.md\"]\n"

	// Act
	cfg, err := decodeConfigTOML([]byte(src), "x")
	require.NoError(t, err)
	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := decodeConfigTOML(out, "x")
	require.NoError(t, err)

	// Assert
	assert.Equal(t, []string{"AGENTS.md", "**/CLAUDE.md"}, cfg.GitignoreExclude)
	assert.Equal(t, cfg.GitignoreExclude, again.GitignoreExclude, "a crud rewrite keeps the key")
	assert.NotNil(t, cfg.Gitignore, "gitignore stays a plain bool")
}

func TestValidateGitignoreExclude(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		wantErr string
	}{
		{name: "none"},
		{name: "plain patterns", entries: []string{"AGENTS.md", "**/CLAUDE.md", ".claude/skills/"}},
		{name: "empty entry", entries: []string{"AGENTS.md", "  "}, wantErr: "is empty"},
		{name: "negation", entries: []string{"!AGENTS.md"}, wantErr: "not a comment or a negation"},
		{name: "comment", entries: []string{"# AGENTS.md"}, wantErr: "not a comment or a negation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{GitignoreExclude: tt.entries}

			// Act
			err := cfg.validateGitignoreExclude()

			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
