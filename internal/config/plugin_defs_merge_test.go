package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginNamesOf(t *testing.T, list any) []string {
	t.Helper()
	items, ok := list.([]any)
	require.True(t, ok, "expected a list, got %T", list)
	names := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		names = append(names, m["name"].(string))
	}
	return names
}

func TestMergeConfigDocs_MarketplacePluginsMergeByName(t *testing.T) {
	shared := map[string]any{
		"marketplace": map[string]any{
			"name": "acme",
			"plugins": []any{
				map[string]any{"name": "a", "skills": []any{"x"}},
				map[string]any{"name": "b", "skills": []any{"y"}},
			},
		},
	}
	tests := []struct {
		name       string
		local      map[string]any
		wantNames  []string
		wantErr    string
		wantDesc   string
		checkEntry func(t *testing.T, plugins []any)
	}{
		{
			name: "an entry is overridden by name, others are kept",
			local: map[string]any{"marketplace": map[string]any{"plugins": []any{
				map[string]any{"name": "b", "description": "layered"},
			}}},
			wantNames: []string{"a", "b"},
			checkEntry: func(t *testing.T, plugins []any) {
				b := plugins[1].(map[string]any)
				assert.Equal(t, "layered", b["description"])
				assert.Equal(t, []any{"y"}, b["skills"], "unset keys of the shared entry stay")
			},
		},
		{
			name: "a new entry is appended",
			local: map[string]any{"marketplace": map[string]any{"plugins": []any{
				map[string]any{"name": "c", "skills": []any{"z"}},
			}}},
			wantNames: []string{"a", "b", "c"},
		},
		{
			name: "remove drops an entry without restating the others",
			local: map[string]any{"marketplace": map[string]any{"plugins": []any{
				map[string]any{"name": "a", "remove": true},
			}}},
			wantNames: []string{"b"},
		},
		{
			name: "other marketplace keys still merge per key",
			local: map[string]any{"marketplace": map[string]any{
				"description": "layer",
				"plugins":     []any{map[string]any{"name": "c", "skills": []any{"z"}}},
			}},
			wantNames: []string{"a", "b", "c"},
			wantDesc:  "layer",
		},
		{
			name: "a duplicate name in the layer is an error naming the layer",
			local: map[string]any{"marketplace": map[string]any{"plugins": []any{
				map[string]any{"name": "c"}, map[string]any{"name": "c"},
			}}},
			wantErr: "duplicate marketplace.plugins entry \"c\" in layer config.p.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			merged, _, err := MergeConfigDocsFrom(shared, tt.local, LayerSource("config.p.toml"))

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			mkt := merged["marketplace"].(map[string]any)
			assert.Equal(t, tt.wantNames, pluginNamesOf(t, mkt["plugins"]))
			if tt.checkEntry != nil {
				tt.checkEntry(t, mkt["plugins"].([]any))
			}
			assert.Equal(t, "acme", mkt["name"])
			if tt.wantDesc != "" {
				assert.Equal(t, tt.wantDesc, mkt["description"])
			}
		})
	}
}

func TestMergeConfigDocs_PluginAndMarketplaceDefinitionListsMergeByName(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		shared    []any
		local     []any
		wantNames []string
	}{
		{
			name:      "plugins_authoring adds and removes by name",
			key:       "plugins_authoring",
			shared:    []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
			local:     []any{map[string]any{"name": "b", "remove": true}, map[string]any{"name": "c"}},
			wantNames: []string{"a", "c"},
		},
		{
			name:      "marketplaces_authoring overrides by name",
			key:       "marketplaces_authoring",
			shared:    []any{map[string]any{"name": "pub", "plugins": []any{"a"}}},
			local:     []any{map[string]any{"name": "pub", "plugins": []any{"a", "b"}}},
			wantNames: []string{"pub"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			merged, _, err := MergeConfigDocs(map[string]any{tt.key: tt.shared}, map[string]any{tt.key: tt.local})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, pluginNamesOf(t, merged[tt.key]))
		})
	}
}
