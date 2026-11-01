package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigTOMLOpenCodeDependencyEnabled(t *testing.T) {
	for _, value := range []string{"", "true", "false", `"false"`} {
		t.Run("value="+value, func(t *testing.T) {
			root := t.TempDir()
			content := "version = \"5.0\"\nname = \"probe\"\n[plugin]\nname = \"probe\"\nversion = \"1.0.0\"\n"
			if value != "" {
				content += "opencode_dependency_enabled = " + value + "\n"
			}
			path := filepath.Join(root, "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			cfg, err := loadConfigTOML(osView(root), path)
			if value == `"false"` {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "opencode_dependency_enabled")
				return
			}
			require.NoError(t, err)
			data, err := json.Marshal(cfg.Plugin)
			require.NoError(t, err)
			var plugin map[string]any
			require.NoError(t, json.Unmarshal(data, &plugin))
			if value == "" {
				assert.NotContains(t, plugin, "opencode_dependency_enabled")
			} else {
				assert.Equal(t, value == "true", plugin["opencode_dependency_enabled"])
			}
		})
	}
}
