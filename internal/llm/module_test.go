package llm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liter-llm is a normal dependency of the root module now: no build tag, no nested module,
// no workspace file. These pin that, and the minimum version whose behavior the budget relies on.
func TestLiterLLMIsARootDependency(t *testing.T) {
	root := filepath.Join("..", "..")
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	assert.Regexp(t, `github.com/xberg-io/liter-llm/packages/go/v2 v2\.`, string(mod))

	for _, gone := range []string{"literllm.work", filepath.Join("internal", "llm", "literllm")} {
		_, err := os.Stat(filepath.Join(root, gone))
		assert.True(t, os.IsNotExist(err), "%s must not exist", gone)
	}
}

// liter-llm 2.2.0 is the first release that counts Gemini thinking tokens in the usage it returns
// (upstream #253); the budget charges that usage as reported, so an older pin would silently
// under-report Gemini spend.
func TestLiterLLMPinCountsGeminiThinkingTokens(t *testing.T) {
	mod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	require.NoError(t, err)
	m := regexp.MustCompile(`github.com/xberg-io/liter-llm/packages/go/v2 v(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(string(mod))
	require.NotNil(t, m, "go.mod must require the liter-llm binding")
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	assert.True(t, major > 2 || (major == 2 && minor >= 2), "liter-llm %s.%s.%s predates the Gemini thinking-token fix (v2.2.0)", m[1], m[2], m[3])
}
