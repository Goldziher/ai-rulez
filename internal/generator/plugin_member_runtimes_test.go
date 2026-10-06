package generator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func monorepoPaths(t *testing.T, g *Generator) map[string]bool {
	t.Helper()
	outputs, err := g.collectPluginOutputs("")
	require.NoError(t, err)
	out := map[string]bool{}
	for _, o := range outputs {
		rel, relErr := filepath.Rel(g.config.BaseDir, o.Path)
		require.NoError(t, relErr)
		out[filepath.ToSlash(rel)] = true
	}
	return out
}

func TestWithMemberRuntimes_LimitsEveryMembersBundle(t *testing.T) {
	t.Parallel()
	// Arrange: alpha ships claude and factory, beta only claude
	cfg, err := config.LoadConfig(context.Background(), "../../tests/fixtures/plugin/monorepo")
	require.NoError(t, err)

	// Act
	all := monorepoPaths(t, NewGenerator(cfg))
	limited := monorepoPaths(t, NewGenerator(cfg).WithMemberRuntimes([]string{"claude"}))

	// Assert
	assert.True(t, all["plugins/alpha/.factory-plugin/plugin.json"])
	assert.True(t, limited["plugins/alpha/.claude-plugin/plugin.json"])
	assert.False(t, limited["plugins/alpha/.factory-plugin/plugin.json"], "the runtime that was not asked for is not rendered")
	assert.True(t, limited["plugins/beta/.claude-plugin/plugin.json"])
}

func TestWithMemberRuntimes_AMemberShippingNoneOfThemIsAnError(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadConfig(context.Background(), "../../tests/fixtures/plugin/monorepo")
	require.NoError(t, err)

	_, err = NewGenerator(cfg).WithMemberRuntimes([]string{"factory"}).collectPluginOutputs("")

	var memberErr *MemberRuntimeError
	require.ErrorAs(t, err, &memberErr)
	assert.Equal(t, "plugins/beta", memberErr.Member)
	assert.Equal(t, []string{"claude"}, memberErr.Ships)
	assert.Contains(t, err.Error(), "none of the requested runtimes factory")
}
