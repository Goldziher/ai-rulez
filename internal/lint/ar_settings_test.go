package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSettingsPosture(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		wantCode string
		wantLine int
	}{
		{"unrestricted Bash", `{"permissions":{"allow":["Bash(*)"]}}`, CodeSettingsPermissionsBroad, 1},
		{"bare Bash", `{"permissions":{"allow":["Read","Bash"]}}`, CodeSettingsPermissionsBroad, 1},
		{"every tool", `{"permissions":{"allow":["*"]}}`, CodeSettingsPermissionsBroad, 1},
		{"bypass mode", "{\n  \"permissions\": {\n    \"defaultMode\": \"bypassPermissions\"\n  }\n}", CodeSettingsPermissionsBroad, 3},
		{"a narrow allow list", `{"permissions":{"allow":["Bash(go test:*)","Read(src/**)"]}}`, "", 0},
		{"the default mode", `{"permissions":{"defaultMode":"acceptEdits"}}`, "", 0},
		{"a misspelled top-level key", "{\n  \"permissionz\": {\"deny\": [\"Read(.env)\"]}\n}", CodeSettingsKeyTypo, 2},
		{"a misspelled permissions key", "{\n  \"permissions\": {\n    \"alow\": [\"Read\"]\n  }\n}", CodeSettingsKeyTypo, 3},
		{"a key from a newer release", `{"someFutureSetting": true}`, "", 0},
		{"known keys", `{"env":{"A":"b"},"model":"opus","hooks":{}}`, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml": baseConfig,
				".claude/settings.json": tt.settings + "\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			got := 0
			for _, f := range fs {
				if f.Code == CodeSettingsPermissionsBroad || f.Code == CodeSettingsKeyTypo {
					got++
					assert.Equal(t, tt.wantCode, f.Code)
					assert.Equal(t, tt.wantLine, f.Line)
				}
			}
			if tt.wantCode == "" {
				assert.Zero(t, got, dump(fs))
			} else {
				assert.Equal(t, 1, got, dump(fs))
			}
		})
	}
}
