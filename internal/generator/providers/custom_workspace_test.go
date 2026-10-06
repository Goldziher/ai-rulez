package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func TestProviderSpecFactoryReadsThroughTheWorkspace(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*workspace.Mem)
		wantErr string
	}{
		{name: "a spec held in memory", setup: func(m *workspace.Mem) { m.Set("demo.toml", demoSpec, 0o644) }},
		{name: "a spec linked to a file inside the workspace", setup: func(m *workspace.Mem) {
			m.Set("real/demo.toml", demoSpec, 0o644)
			m.Symlink("demo.toml", "real/demo.toml")
		}},
		{name: "a spec linked outside the workspace is refused", setup: func(m *workspace.Mem) {
			m.Symlink("demo.toml", "../../elsewhere/demo.toml")
		}, wantErr: "outside the repository root"},
		{name: "a missing spec", setup: func(*workspace.Mem) {}, wantErr: "read provider spec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			reg := config.NewRegistry()
			providers.Register(reg)
			mem := workspace.NewMem("/virtual/proj")
			tt.setup(mem)

			// Act
			gen, err := reg.Provider(config.Preset{Name: "demo", Provider: "demo.toml"}, "/virtual/proj", workspace.NewView(mem))

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "demo", gen.GetName())
		})
	}
}
