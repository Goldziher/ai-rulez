package parity_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/parity"
)

var updateDocs = flag.Bool("update-docs", false, "rewrite the parity table in docs/mcp-server.md from internal/parity")

// docsPath is docs/mcp-server.md, found from this test file's directory.
func docsPath(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "docs", "mcp-server.md"))
	require.NoError(t, err)
	return dir
}

// The capability table in the docs is generated from the table the tests
// check, so the docs cannot claim a tool or a command the code does not have.
func TestDocsMatchTheTable(t *testing.T) {
	path := docsPath(t)
	data, err := os.ReadFile(path) //nolint:gosec // the repository's own doc
	require.NoError(t, err)
	doc := string(data)
	begin, end := strings.Index(doc, parity.DocsBegin), strings.Index(doc, parity.DocsEnd)
	generated := parity.DocsBegin + "\n\n" + parity.Markdown(parity.Capabilities()) + "\n" + parity.DocsEnd

	if *updateDocs {
		require.GreaterOrEqual(t, begin, 0, "docs/mcp-server.md needs the %s marker", parity.DocsBegin)
		require.Greater(t, end, begin)
		updated := doc[:begin] + generated + doc[end+len(parity.DocsEnd):]
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
		return
	}

	require.GreaterOrEqual(t, begin, 0, "docs/mcp-server.md has no parity block")
	require.Greater(t, end, begin)
	require.Equal(t, generated, doc[begin:end+len(parity.DocsEnd)],
		"the parity table in docs/mcp-server.md is stale: go test ./tests/parity -run TestDocsMatchTheTable -update-docs")
}
