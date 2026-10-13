package lint

import (
	"path/filepath"
	"strings"
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

// The finding points at the definition's own table or file, not at the first line
// that happens to say name = "notify".
func TestStrict_PluginSelectorUnmatchedLocation(t *testing.T) {
	skill := map[string]string{".ai-rulez/skills/doc-a/SKILL.md": "---\nname: doc-a\ndescription: Use when documenting\n---\n# Doc\n"}
	tests := []struct {
		name     string
		config   string
		files    map[string]string
		wantFile string
		wantLine func(config string) int
	}{
		{
			name: "a marketplace with the plugin's name earlier in the file",
			config: `
[[marketplaces_authoring]]
name = "notify"
plugins = ["notify"]

[[plugins_authoring]]
name = "notify"
skills = ["nope"]
`,
			wantFile: ".ai-rulez/config.toml",
			wantLine: func(c string) int { return lineOf(c, "[[plugins_authoring]]") + 1 },
		},
		{
			name:     "a plugin defined in its own file",
			config:   "\n",
			files:    map[string]string{".ai-rulez/plugins/notify.toml": "skills = [\"nope\"]\n"},
			wantFile: ".ai-rulez/plugins/notify.toml",
			wantLine: func(string) int { return 1 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := precisionTree("", tt.config, skill)
			for k, v := range tt.files {
				files[k] = v
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)

			fs := lintDir(t, root)

			var got []Finding
			for _, f := range fs {
				if f.Code == CodePluginSelectorUnmatched {
					got = append(got, f)
				}
			}
			require.Len(t, got, 1, "%s", dump(fs))
			assert.True(t, strings.HasSuffix(filepath.ToSlash(got[0].File), tt.wantFile), got[0].File)
			assert.Equal(t, tt.wantLine(files[".ai-rulez/config.toml"]), got[0].Line)
		})
	}
}

// lineOf is the 1-based line of the first line equal to want.
func lineOf(text, want string) int {
	for i, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) == want {
			return i + 1
		}
	}
	return 0
}
