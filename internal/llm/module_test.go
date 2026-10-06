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

// The literllm bridge is a nested module that the -tags literllm build imports by
// path. A path outside the /v5 module root cannot be resolved by the go.work file.
func TestLiterLLMBridgeModulePathMatchesImport(t *testing.T) {
	const want = "github.com/Goldziher/ai-rulez/v5/internal/llm/literllm"

	mod, err := os.ReadFile(filepath.Join("literllm", "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(mod), "module "+want+"\n")

	src, err := os.ReadFile("native_literllm.go")
	require.NoError(t, err)
	assert.Contains(t, string(src), `"`+want+`"`)

	work, err := os.ReadFile(filepath.Join("..", "..", "literllm.work"))
	require.NoError(t, err)
	assert.Contains(t, string(work), "./internal/llm/literllm")
}
