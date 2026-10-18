package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// A local domain named like an enabled builtin pack keeps the pack from loading.
// That must be a warning naming the items it costs, not a debug line (#446).
func TestLoadConfig_BuiltinPackSkippedByLocalDomainWarns(t *testing.T) {
	tests := []struct {
		name         string
		domain       string
		localSkills  []string
		builtins     string
		wantWarn     bool
		wantContains []string
		wantMissing  []string
	}{
		{
			name:         "unrelated local skill loses the whole pack",
			domain:       "testing",
			localSkills:  []string{"local-one"},
			builtins:     `["testing"]`,
			wantWarn:     true,
			wantContains: []string{"testing", "skills/testing-conventions", "skills/tdd-workflow"},
		},
		{
			name:         "a same-named local skill is not reported as lost",
			domain:       "testing",
			localSkills:  []string{"testing-conventions"},
			builtins:     `["testing"]`,
			wantWarn:     true,
			wantContains: []string{"skills/tdd-workflow"},
			wantMissing:  []string{"skills/testing-conventions"},
		},
		{
			name:        "a domain named like no builtin pack is not reported",
			domain:      "payments",
			localSkills: []string{"local-one"},
			builtins:    `["testing"]`,
			wantWarn:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			configDir := filepath.Join(root, aiRulezDirName)
			for _, skill := range tc.localSkills {
				dir := filepath.Join(configDir, domainsDir, tc.domain, "skills", skill)
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"),
					[]byte("---\nname: "+skill+"\ndescription: Local skill in a domain called testing.\n---\n# P\n"), 0o644))
			}
			require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename),
				[]byte("version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\nbuiltins = "+tc.builtins+"\n"), 0o644))
			rec := &recordingLogger{}

			// Act
			_, err := LoadConfig(context.Background(), root, WithHost(ambient.Host{Log: rec}))

			// Assert
			require.NoError(t, err)
			var skipped []string
			for _, w := range rec.warns {
				if strings.Contains(w, "Builtin pack skipped") {
					skipped = append(skipped, w)
				}
			}
			if !tc.wantWarn {
				assert.Empty(t, skipped)
				return
			}
			require.Len(t, skipped, 1, "one warning per pack")
			for _, want := range tc.wantContains {
				assert.Contains(t, skipped[0], want)
			}
			for _, missing := range tc.wantMissing {
				assert.NotContains(t, skipped[0], missing)
			}
		})
	}
}
