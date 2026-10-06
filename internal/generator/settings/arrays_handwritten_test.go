package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
)

// TestPermissionsHandWrittenDuplicateSurvivesGenerateAndClean writes the exact
// document ai-rulez would generate by hand, then generates over it: every
// element is already there, so none is claimed and clean returns the file
// byte for byte.
func TestPermissionsHandWrittenDuplicateSurvivesGenerateAndClean(t *testing.T) {
	cases := []struct {
		name   string
		c      permDialectCase
		prefix string // a comment the user wrote, which sends JSON down the JSONC engine
	}{
		{"json", permDialectCase{"qwen", ".qwen/settings.json", docmerge.FormatJSON}, ""},
		{"jsonc", permDialectCase{"devin", ".devin/config.json", docmerge.FormatJSONC}, "// mine\n"},
		{"toml", permDialectCase{"grok", ".grok/config.toml", docmerge.FormatTOML}, ""},
		{"yaml", permDialectCase{"poolside", ".poolside/settings.yaml", docmerge.FormatYAML}, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			captureWarnings(t)
			// Arrange: the output of a first generate, adopted as the user's own file.
			dir := t.TempDir()
			doc := filepath.Join(dir, filepath.FromSlash(tt.c.path))
			require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
			rules := &config.Permissions{Allow: []string{"Bash(git status)"}, Deny: []string{"Bash(rm -rf:*)"}}
			first := renderPermDialect(t, tt.c, permConfig(dir, rules), doc)
			handWritten := tt.prefix + first.Body
			require.NoError(t, os.WriteFile(doc, []byte(handWritten), 0o644))

			// Act: generate over it with no earlier record, then clean.
			second := renderPermDialect(t, tt.c, permConfig(dir, rules), doc)
			clean, err := docmerge.Unmerge(doc, tt.c.format, second.Claims)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, handWritten, second.Body, "nothing to add")
			if clean.Changed {
				assert.Equal(t, handWritten, clean.Body)
			}
		})
	}
}
