package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func TestLocalIncludeIsReadThroughTheProjectWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		setup     func(*workspace.Mem)
		wantRules []string
		wantErr   string
	}{
		{
			name:      "bare structure inside the project",
			source:    "shared",
			wantRules: []string{"base", "shared-rule"},
		},
		{
			name:   "a symlinked rule inside the include is not followed",
			source: "shared",
			setup: func(ws *workspace.Mem) {
				ws.Symlink("shared/rules/link.md", "../../local-secret.md")
				ws.Set("local-secret.md", "SECRET\n", 0o644)
			},
			wantRules: []string{"base", "shared-rule"},
		},
		{
			name:    "a symlinked include directory pointing outside the project is refused",
			source:  "escape",
			setup:   func(ws *workspace.Mem) { ws.Symlink("escape", "../elsewhere") },
			wantErr: "outside the project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the project exists only in memory.
			ws := workspace.NewMem("/virtual/proj")
			ws.Set(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n", 0o644)
			ws.Set(".ai-rulez/rules/base.md", "# Base\n", 0o644)
			ws.Set("shared/rules/shared-rule.md", "# Shared\n", 0o644)
			if tt.setup != nil {
				tt.setup(ws)
			}
			cfg, err := config.LoadConfig(t.Context(), ".", config.WithWorkspace(ws), config.WithoutRemote())
			require.NoError(t, err)
			cfg.Includes = []config.IncludeConfig{{Name: "shared", Source: tt.source}}

			// Act
			tree, err := NewResolver(cfg.BaseDir, "").ResolveIncludes(t.Context(), cfg)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var rules []string
			for _, r := range tree.Rules {
				rules = append(rules, r.Name)
			}
			assert.ElementsMatch(t, tt.wantRules, rules)
		})
	}
}
