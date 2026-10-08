package commands

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every AI_RULEZ_* name the code reads is in the Environment Variables section of
// docs/cli.md. Names that end in an underscore are prefixes of a family listed
// there by its members.
func TestEveryEnvironmentVariableIsDocumented(t *testing.T) {
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	require.NoError(t, err)
	section := string(docs)
	_, section, ok := strings.Cut(section, "## Environment Variables")
	require.True(t, ok, "docs/cli.md has no Environment Variables section")
	section, _, _ = strings.Cut(section, "\n## ")

	name := regexp.MustCompile(`AI_RULEZ_[A-Z0-9]+(?:_[A-Z0-9]+)*`)
	roots := []string{filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "pkg")}
	used := map[string]string{}
	for _, root := range roots {
		require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.Contains(filepath.ToSlash(path), "/testutil/") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range name.FindAllString(string(src), -1) {
				used[m] = path
			}
			return nil
		}))
	}
	require.NotEmpty(t, used)

	for env, path := range used {
		if !strings.Contains(section, env) {
			t.Errorf("%s (read in %s) is not documented in the Environment Variables section of docs/cli.md", env, path)
		}
	}
}
