package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/settings"
)

func TestHookKeys_ReadACommentedDocument(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		doc     string
		body    string
		want    []string
		notWant []string
	}{
		{
			name: "devin keeps a user hook next to a comment", harness: config.HarnessDevin,
			doc:  filepath.Join(".config", "devin", "config.json"),
			body: "{\n  // mine\n  \"hooks\": {\"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"mine\"}]}],},\n}\n",
			want: []string{"mine", "echo done"},
		},
		{
			name: "cursor does not re-add the version of a commented document", harness: config.HarnessCursor,
			doc:     filepath.Join(".cursor", "hooks.json"),
			body:    "{\n  // mine\n  \"version\": 1,\n  \"hooks\": {}\n}\n",
			want:    []string{"hooks.stop=", "echo done"},
			notWant: []string{"version="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			doc := filepath.Join(base, tt.doc)
			require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
			require.NoError(t, os.WriteFile(doc, []byte(tt.body), 0o644))
			cfg := &config.Config{BaseDir: base, UserScope: true, Hooks: []config.HookGroup{
				{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done"}}},
			}}

			// Act
			keys, err := settings.HookKeys(cfg, tt.harness, doc)
			require.NoError(t, err)

			// Assert
			var dump []string
			for _, key := range keys {
				raw, err := json.Marshal(key.Value)
				require.NoError(t, err)
				dump = append(dump, strings.Join(key.Path, ".")+"="+string(raw))
			}
			joined := strings.Join(dump, "\n")
			for _, want := range tt.want {
				assert.Contains(t, joined, want)
			}
			for _, not := range tt.notWant {
				assert.NotContains(t, joined, not)
			}
		})
	}
}

func TestHookKeys_WarnWhenARequiredKeyHoldsAnotherValue(t *testing.T) {
	// Arrange
	warnings := captureWarnings(t)
	base := t.TempDir()
	doc := filepath.Join(base, ".zcode", "cli", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(doc), 0o755))
	require.NoError(t, os.WriteFile(doc, []byte(`{"hooks": {"enabled": false}}`), 0o644))
	cfg := &config.Config{BaseDir: base, UserScope: true, Hooks: []config.HookGroup{
		{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done"}}},
	}}

	// Act
	_, err := settings.HookKeys(cfg, config.HarnessZCode, doc)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, strings.Join(*warnings, "\n"), "hooks.enabled")
}
