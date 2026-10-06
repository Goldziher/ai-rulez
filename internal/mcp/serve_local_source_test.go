package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A committed project config must not point the skills server at another user's
// skill directory; the same path typed with --source is the user's own choice.
func TestServeSetup_LocalSourceOfTheProjectConfigStaysInsideTheProject(t *testing.T) {
	// Arrange
	outside := t.TempDir()
	writeFile(t, outside, "stolen/SKILL.md", skillFile("stolen", "Lives outside the project", ""))
	localConfig := func(url string) string {
		return baseConfig + "\n[[skill_sources]]\nname = \"loc\"\nurl = \"" + filepath.ToSlash(url) + "\"\n"
	}

	tests := []struct {
		name    string
		config  string
		files   map[string]string
		sources []string
		want    []string
		wantErr string
	}{
		{"absolute path outside", localConfig(outside), nil, nil, nil, "outside the project"},
		{"parent traversal", localConfig("../../.."), nil, nil, nil, "outside the project"},
		{"directory inside", localConfig("vendor-skills"), nil, nil, []string{"inner"}, ""},
		{"--source outside is allowed", baseConfig, nil, []string{outside}, []string{"stolen"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := project(t, tt.config, tt.files)
			writeFile(t, root, "vendor-skills/inner/SKILL.md", skillFile("inner", "Inside the project", ""))
			setup := &ServeSetup{WorkDir: root, NoWatch: true, Sources: tt.sources, CacheDir: filepath.Join(t.TempDir(), "cache")}

			// Act
			srv, err := setup.NewServer(context.Background())

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, catalogNames(srv.Catalog()))
		})
	}
}
