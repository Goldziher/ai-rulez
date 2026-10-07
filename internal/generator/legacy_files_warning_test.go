package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func TestGenerate_WarnsAboutLegacyFilesBesideConfigToml(t *testing.T) {
	dir := concurrentProject(t, 0)
	for _, name := range []string{"config.yaml", "mcp.json", "mcp.yml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", name), []byte("{}\n"), 0o644))
	}
	rec := &warnLog{Logger: logger.Discard()}

	cfg, err := config.LoadConfig(t.Context(), dir, config.WithHost(ambient.Host{Env: ambient.MapEnv{Home: t.TempDir()}, Log: rec}))
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))

	all := strings.Join(rec.warn, "\n")
	for _, name := range []string{"config.yaml", "mcp.json", "mcp.yml"} {
		assert.Contains(t, all, name+" is no longer read", name)
	}
	assert.Contains(t, all, "migrate v5")
}
