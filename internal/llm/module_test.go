package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The liter-llm binding is cgo-only and needs a native library at link time, so
// the root module must never require it (that would break CGO_ENABLED=0 builds).
func TestRootModuleDoesNotMentionLiterLLM(t *testing.T) {
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join("..", "..", name))
		require.NoError(t, err)
		assert.NotContains(t, strings.ToLower(string(data)), "liter-llm", name)
	}
}
