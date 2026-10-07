package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// A command that fails on malformed frontmatter (Validate names the files) asks
// the load not to warn about the same files first; every other command keeps
// the warning, exactly once per file.
func TestMalformedFrontmatterIsReportedOncePerFile(t *testing.T) {
	tests := []struct {
		name      string
		opts      []LoadOption
		wantWarns int
	}{
		{"default load warns", nil, 2},
		{"load for a validating command stays silent", []LoadOption{WithFrontmatterErrors()}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := workspace.NewMem("/virtual/fm2")
			ws.Set(".ai-rulez/config.toml", memConfigTOML, 0o644)
			ws.Set(".ai-rulez/rules/broken.md", "---\ntitle: a: b: c\n---\n# Broken\n", 0o644)
			ws.Set(".ai-rulez/agents/bad.md", "---\nname: x: y: z\n---\nAgent\n", 0o644)
			rec := &testutil.LogRecorder{}
			opts := append([]LoadOption{WithWorkspace(ws), WithoutRemote(), WithHost(ambient.Host{Log: rec})}, tt.opts...)

			// Act
			cfg, err := LoadConfig(t.Context(), "/virtual/fm2", opts...)
			require.NoError(t, err)
			valErr := cfg.Validate()

			// Assert
			require.Error(t, valErr)
			assert.Contains(t, valErr.Error(), "rules/broken.md")
			assert.Contains(t, valErr.Error(), "agents/bad.md")
			assert.Len(t, rec.Level("WARN"), tt.wantWarns, rec.String())
		})
	}
}
