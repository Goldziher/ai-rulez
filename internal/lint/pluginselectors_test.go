package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginSelectorRuleIsRegistered(t *testing.T) {
	rule, ok := lookupRule(CodePluginSelectorUnmatched)
	require.True(t, ok, CodePluginSelectorUnmatched)
	assert.Equal(t, "plugin-selector-unmatched", rule.Name)
	assert.Equal(t, SeverityWarning, rule.Default)
	assert.Equal(t, AnalyzerInfo{AnalyzerPlugin, ScopeBundle}, AnalyzerFor(CodePluginSelectorUnmatched))
}

// AR9T1 warns for each selector of a plugin definition that matches nothing, in
// both spellings of a definition.
func TestStrict_PluginSelectorUnmatched(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   int
		field  string
	}{
		{
			name: "every selector matches",
			config: `
[[plugins_authoring]]
name = "notify"
skills = ["doc-*"]
`,
			want: 0,
		},
		{
			name: "a skills glob that matches no skill",
			config: `
[[plugins_authoring]]
name = "notify"
skills = ["notfy-*"]
`,
			want:  1,
			field: "notfy-*",
		},
		{
			name: "the marketplace.plugins spelling is checked too",
			config: `
[marketplace]
name = "m"
[[marketplace.plugins]]
name = "notify"
skills = ["doc-*", "nope"]
agents = ["missing-agent"]
`,
			want: 2,
		},
		{
			name: "a domain that does not exist",
			config: `
[[plugins_authoring]]
name = "notify"
domains = ["ghost"]
`,
			want:  1,
			field: "ghost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := precisionTree("", tt.config, map[string]string{
				".ai-rulez/skills/doc-a/SKILL.md": "---\nname: doc-a\ndescription: Use when documenting\n---\n# Doc\n",
			})
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Equal(t, tt.want, countCode(fs, CodePluginSelectorUnmatched), "%s", dump(fs))
			if tt.field != "" {
				found := false
				for _, f := range fs {
					if f.Code == CodePluginSelectorUnmatched {
						assert.Contains(t, f.Message, tt.field)
						assert.Contains(t, f.Message, `plugin "notify"`)
						found = true
					}
				}
				assert.True(t, found)
			}
		})
	}
}
