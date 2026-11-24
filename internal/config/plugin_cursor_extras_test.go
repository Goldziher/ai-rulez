package config

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestCursorExtrasHookRoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cursor *CursorExtras
		want   string
	}{
		{"no cursor table", nil, "."},
		{"fallback unset", &CursorExtras{Marketplace: true}, "."},
		{"fallback set", &CursorExtras{HookRootFallback: "$HOME/.cursor/plugins/demo"}, "$HOME/.cursor/plugins/demo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.cursor.HookRoot())
		})
	}
}

// The JSON schema (validate) and Config.Validate (generate) must agree on which
// [plugin.cursor] hook_root_fallback values are accepted, and an accepted value
// must decode into CursorExtras.
func TestCursorHookRootFallback_SchemaAndValidateAgree(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"$HOME/.cursor/plugins/demo", true},
		{"/opt/demo-plugin", true},
		{"${HOME}/demo", false},
		{"C:/Users/dev/plugins/demo", true},
		{"../demo", true},
		{"~/.cursor/plugins/demo", false},
		{"/opt/my plugin", false},
		{" /opt/demo", false},
		{`/opt/"demo"`, false},
		{"/opt/demo'", false},
		{"/opt/$(whoami)", false},
		{"/opt/`whoami`", false},
		{"/opt/demo}", false},
		{`C:\plugins\demo`, false},
		{"/opt/demo;rm", false},
		{"$1/demo", false},
		{"/opt/$", false},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			body := "version = \"5.0\"\nname = \"x\"\n\n[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\n" +
				"description = \"d\"\nruntimes = [\"cursor\"]\n\n[plugin.cursor]\nhook_root_fallback = " +
				strconv.Quote(tc.value) + "\n"
			schemaErr := schema.ValidateFile(writeConfigTOML(t, body))
			cfg, err := decodeConfigTOML([]byte(body), "config.toml")
			require.NoError(t, err)
			require.NotNil(t, cfg.Plugin)
			require.NotNil(t, cfg.Plugin.Cursor)
			assert.Equal(t, tc.value, cfg.Plugin.Cursor.HookRootFallback)
			validateErr := cfg.validatePluginAuthoring()
			assert.Equal(t, tc.ok, schemaErr == nil, "schema: %v", schemaErr)
			assert.Equal(t, tc.ok, validateErr == nil, "validatePluginAuthoring: %v", validateErr)
			if !tc.ok && validateErr != nil {
				assert.Contains(t, validateErr.Error(), "invalid cursor hook_root_fallback")
			}
		})
	}
}
