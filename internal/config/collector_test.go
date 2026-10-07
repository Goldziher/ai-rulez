package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// A watch or a live-reload loop loads the same project every cycle. The warnings
// of a load are about the project, not the cycle: with a collector that outlives
// the loads each is said once, not on every reload.
func TestWithCollectorSaysEachLoadWarningOnce(t *testing.T) {
	tests := []struct {
		name     string
		shared   bool
		wantMalf int
	}{
		{"a fresh collector per load repeats them", false, 3},
		{"one collector across the loads says them once", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := workspace.NewMem("/virtual/watch")
			ws.Set(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"watch\"\npresets = [\"claude\"]\n", 0o644)
			ws.Set(".ai-rulez/rules/broken.md", "---\ntitle: a: b: c\n---\n# Broken\n", 0o644)
			rec := &testutil.LogRecorder{}
			collector := diag.New(nil)

			// Act
			for range 3 {
				opts := []LoadOption{WithWorkspace(ws), WithoutRemote(), WithHost(ambient.Host{Log: rec})}
				if tt.shared {
					opts = append(opts, WithCollector(collector))
				}
				_, err := LoadConfig(t.Context(), "/virtual/watch", opts...)
				require.NoError(t, err)
			}

			// Assert
			got := rec.Level("WARN")
			assert.Equal(t, tt.wantMalf, countContaining(got, "malformed YAML frontmatter"), "%v", got)
		})
	}
}
