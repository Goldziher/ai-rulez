package crud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSkillSource_Type(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "https URL", source: "https://github.com/org/repo", want: "git"},
		{name: "scp-style URL", source: "git@github.com:org/repo.git", want: "git"},
		{name: "owner/repo shorthand", source: "org/repo", want: "git"},
		{name: "github: shorthand", source: "github:org/repo", want: "git"},
		{name: "ssh URL", source: "ssh://git@example.com/org/repo.git", want: "git"},
		{name: "relative directory", source: "./vendor/skills", want: "local"},
		{name: "owner/repo naming a directory", source: "vendor/skills", want: "local"},
		{name: "home directory", source: "~/skills", want: "local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(base, "vendor", "skills"), 0o755))

			// Act
			got, err := validateSkillSource(base, tt.source)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
