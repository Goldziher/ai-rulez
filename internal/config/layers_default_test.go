package config

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultLayersMain is layerMain with default_layers set above the first table.
func defaultLayersMain(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	return strings.Replace(layerMain, "presets =", "default_layers = ["+strings.Join(quoted, ", ")+"]\npresets =", 1)
}

func TestLayers_DefaultLayers(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		env        string
		wantActive []string
		wantOrigin map[string]string
		wantErr    []string
		wantCmd    map[string]string
	}{
		{
			name: "default layer is active without environment",
			files: map[string]string{
				"config.toml":      defaultLayersMain("lint"),
				"config.lint.toml": "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-lint\"\n",
			},
			wantActive: []string{"lint"},
			wantOrigin: map[string]string{"lint": "default"},
			wantCmd:    map[string]string{"base": "from-lint"},
		},
		{
			name: "env layers follow default layers and repeated names keep the first origin",
			files: map[string]string{
				"config.toml":      defaultLayersMain("lint", "lint"),
				"config.lint.toml": "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-lint\"\n",
				"config.ci.toml":   "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-ci\"\n",
			},
			env:        "ci,lint",
			wantActive: []string{"lint", "ci"},
			wantOrigin: map[string]string{"lint": "default", "ci": "env"},
			wantCmd:    map[string]string{"base": "from-ci"},
		},
		{
			name:    "a listed layer without a file is an error naming the file",
			files:   map[string]string{"config.toml": defaultLayersMain("lint")},
			wantErr: []string{"default_layers", "config.lint.toml"},
		},
		{
			name:  "an env layer without a file is skipped",
			files: map[string]string{"config.toml": layerMain},
			env:   "nothere",
		},
		{
			name:    "a reserved default layer name is rejected",
			files:   map[string]string{"config.toml": defaultLayersMain("local")},
			wantErr: []string{"reserved"},
		},
		{
			name: "default_layers inside a layer is ignored",
			files: map[string]string{
				"config.toml":      defaultLayersMain("lint"),
				"config.lint.toml": "default_layers = [\"other\"]\n",
			},
			wantActive: []string{"lint"},
			wantOrigin: map[string]string{"lint": "default"},
			wantCmd:    map[string]string{"base": "echo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layerProject(t, tt.files)
			t.Setenv("AI_RULEZ_LAYERS", tt.env)

			// Act
			cfg, err := LoadConfig(context.Background(), root)

			// Assert
			if len(tt.wantErr) > 0 {
				require.Error(t, err)
				for _, want := range tt.wantErr {
					assert.Contains(t, err.Error(), want)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantActive, cfg.ActiveLayers)
			assert.Equal(t, tt.wantOrigin, cfg.LayerOrigins)
			for _, s := range cfg.MCPServersRaw {
				if want, ok := tt.wantCmd[s.Name]; ok {
					assert.Equal(t, want, s.Command, s.Name)
				}
			}
		})
	}
}

func TestLayers_WithoutLayersSkipsDefaultsAndAllowsWrite(t *testing.T) {
	root := layerProject(t, map[string]string{
		"config.toml":      defaultLayersMain("lint"),
		"config.lint.toml": "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-lint\"\n",
	})
	cfg, err := LoadConfig(context.Background(), root, WithoutLayers())
	require.NoError(t, err)
	assert.Empty(t, cfg.ActiveLayers)
	data, err := MarshalTOML(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(data), "default_layers")
}

func TestLayers_MessagesNameTheLayerFile(t *testing.T) {
	tests := []struct {
		name    string
		layer   string
		wantAll []string
		wantNot []string
	}{
		{name: "unknown key", layer: "bogus = 1\n", wantAll: []string{"unknown key(s) in layer config.lint.toml: bogus"}, wantNot: []string{"local config"}},
		{name: "table type", layer: "lint = 3\n", wantAll: []string{"layer config.lint.toml key lint has type"}, wantNot: []string{"local config"}},
		{name: "decode type error", layer: "name = 3\n", wantAll: []string{"config.lint.toml", "layer files"}, wantNot: []string{"local overlay", "config.<layer>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layerProject(t, map[string]string{
				"config.toml":      defaultLayersMain("lint"),
				"config.lint.toml": tt.layer,
			})

			// Act
			_, err := LoadConfig(context.Background(), root)

			// Assert
			require.Error(t, err)
			rendered := fmt.Sprintf("%+v", err)
			if o, ok := oops.AsOops(err); ok {
				rendered += " " + o.Hint()
			}
			for _, want := range tt.wantAll {
				assert.Contains(t, rendered, want)
			}
			for _, not := range tt.wantNot {
				assert.NotContains(t, rendered, not)
			}
		})
	}
}
