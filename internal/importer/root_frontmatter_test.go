package importer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativePlan_RootFileFrontmatterKeys(t *testing.T) {
	tests := []struct {
		name         string
		claude       string
		wantMeta     []string // keys nested under metadata in the planned file
		wantTopLevel []string // keys kept at the top level
		wantAbsent   []string // keys that must not stay at the top level
		wantFinding  bool
		wantBody     string
	}{
		{
			name:        "unknown keys move under metadata",
			claude:      "---\ntitle: Operating contract\napplies_to: the repo\nupdated: 2026-08-10\n---\n\n# CLAUDE.md\n\nBody.\n",
			wantMeta:    []string{"title", "applies_to", "updated"},
			wantAbsent:  []string{"title:", "applies_to:", "updated:"},
			wantFinding: true,
			wantBody:    "Body.",
		},
		{
			name:         "known keys stay and an existing metadata map is extended",
			claude:       "---\ndescription: Repo contract\nmetadata:\n  owner: me\ntitle: T\n---\nBody.\n",
			wantMeta:     []string{"owner", "title"},
			wantTopLevel: []string{"description"},
			wantFinding:  true,
			wantBody:     "Body.",
		},
		{
			name:         "only known keys are left alone",
			claude:       "---\ndescription: Repo contract\npriority: high\n---\nBody.\n",
			wantTopLevel: []string{"description", "priority"},
			wantBody:     "Body.",
		},
		{
			name:     "no frontmatter",
			claude:   "# Project\n\nUse Go.\n",
			wantBody: "Use Go.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fsys := mapFS(map[string]string{"CLAUDE.md": tt.claude})
			// Act
			p := planOf(t, nativeImporter{}, fsys, Options{})
			// Assert
			require.Len(t, p.Items, 1)
			text := string(p.Items[0].Main)
			fm, body, _ := splitFrontmatter(text)
			assert.Contains(t, body, tt.wantBody)
			parsed, _ := parseFrontmatter(fm)
			meta, _ := parsed["metadata"].(map[string]any)
			for _, k := range tt.wantMeta {
				assert.Contains(t, meta, k, "metadata key %s in:\n%s", k, text)
			}
			for _, k := range tt.wantTopLevel {
				assert.Contains(t, parsed, k)
			}
			for _, k := range tt.wantAbsent {
				assert.NotContains(t, fm, "\n"+k, "top-level %s in:\n%s", k, text)
				assert.False(t, strings.HasPrefix(fm, k), "top-level %s in:\n%s", k, text)
			}
			got := findingFor(p, StatusApproximated, "CLAUDE.md", "frontmatter") != nil
			assert.Equal(t, tt.wantFinding, got)
		})
	}
}
