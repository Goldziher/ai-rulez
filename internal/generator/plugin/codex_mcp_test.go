package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codexMCPOutput(t *testing.T, existing string) config.OutputFile {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), fixtureDir)
	require.NoError(t, err)
	m, err := BuildManifest(cfg, cfg.Content)
	require.NoError(t, err)

	dir := t.TempDir()
	if existing != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0o644))
	}
	outs, err := Generate(m, dir)
	require.NoError(t, err)
	for _, o := range outs {
		if o.Path == filepath.Join(dir, ".mcp.json") {
			return o
		}
	}
	t.Fatal("no .mcp.json output")
	return config.OutputFile{}
}

func TestCodexMCPFile_RecordsClaimsAndKeepsHandWrittenContent(t *testing.T) {
	tests := []struct {
		name          string
		existing      string
		wantPartially bool
	}{
		{"fresh file is wholly generated", "", false},
		{"hand-written server and key survive", `{"note": "keep", "mcpServers": {"mine": {"command": "x"}}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			out := codexMCPOutput(t, tt.existing)

			// Assert
			body := string(out.RawContent)
			doc := parseJSON(t, out.RawContent)
			servers := doc["mcpServers"].(map[string]any)
			assert.Contains(t, servers, "basemind")
			assert.NotEmpty(t, out.MergeClaims, "the file records what it wrote")
			assert.Equal(t, tt.wantPartially, out.PartiallyOwned)
			if tt.existing != "" {
				assert.Contains(t, servers, "mine", body)
				assert.Equal(t, "keep", doc["note"])
			}
		})
	}
}
