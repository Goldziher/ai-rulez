package llm

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEveryProviderClientIsBuiltByTheCredentialAwareConstructor keeps Bedrock working in every
// command that calls a model: the liter-llm client is created in one place (newLiterLLM, behind
// New), which resolves and refreshes the AWS credential chain. A second construction site would
// silently skip that for the command that uses it.
func TestEveryProviderClientIsBuiltByTheCredentialAwareConstructor(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir // Tool state can contain independent Git worktrees.
			}
			switch d.Name() {
			case "node_modules", "vendor", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path) //nolint:gosec // walking the repository's own sources
		if rerr != nil {
			return rerr
		}
		text := string(data)
		if strings.Contains(text, "lit.CreateClient") || strings.Contains(text, "CreateClientWithOptions(") || strings.Contains(text, "CreateClientFromJSON(") {
			rel, _ := filepath.Rel(root, path)
			if filepath.ToSlash(rel) != "internal/llm/client.go" {
				offenders = append(offenders, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, offenders, "build model clients with llm.New so the Bedrock credential chain applies")
}
