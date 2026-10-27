package presets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func scopedRule(name, include string, globs ...string) config.ContentFile {
	return config.ContentFile{
		Name: name, Path: name + ".md", Include: include, Content: name + " body",
		Metadata: &config.Metadata{Globs: globs},
	}
}

func skillPaths(skills []config.ContentFile) map[string][]string {
	got := map[string][]string{}
	for i := range skills {
		got[skills[i].ID] = skills[i].Metadata.Paths
	}
	return got
}

func TestScopedFallbackSkills_Grouping(t *testing.T) {
	content := &config.ContentTree{Rules: []config.ContentFile{
		scopedRule("local", "", "src/**/*.go"),
		scopedRule("shared-a", "shared", "api/**/*.go"),
		scopedRule("shared-b", "shared", "web/**/*.ts"),
		scopedRule("other", "other", "docs/**"),
	}}
	cases := []struct {
		name  string
		group string
		want  map[string][]string
	}{
		{"none", config.ScopedFallbackGroupNone, map[string][]string{
			"rule-local": {"src/**/*.go"}, "rule-shared-a": {"api/**/*.go"},
			"rule-shared-b": {"web/**/*.ts"}, "rule-other": {"docs/**"},
		}},
		{"include", config.ScopedFallbackGroupInclude, map[string][]string{
			"rule-local": {"src/**/*.go"}, "scoped-shared": {"api/**/*.go", "web/**/*.ts"}, "rule-other": {"docs/**"},
		}},
		{"directory", config.ScopedFallbackGroupDirectory, map[string][]string{
			"rule-local": {"src/**/*.go"}, "rule-shared-a": {"api/**/*.go"},
			"rule-shared-b": {"web/**/*.ts"}, "rule-other": {"docs/**"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Rules: &config.RulesConfig{ScopedFallbackGroup: tc.group}}
			assert.Equal(t, tc.want, skillPaths(ScopedFallbackSkills(content, cfg, nil)))
		})
	}
}

func TestScopedFallbackSkills_DirectoryGroupKey(t *testing.T) {
	content := &config.ContentTree{Rules: []config.ContentFile{
		scopedRule("a", "", "pkg/api/**/*.go"),
		scopedRule("b", "", "pkg/api/handlers/*.go", "pkg/api/**/*.md"),
		scopedRule("c", "", "**/*.go"),
		scopedRule("d", "", "*.toml"),
	}}
	cfg := &config.Config{Rules: &config.RulesConfig{ScopedFallbackGroup: config.ScopedFallbackGroupDirectory}}
	assert.Equal(t, map[string][]string{
		"scoped-pkg-api": {"pkg/api/**/*.go", "pkg/api/handlers/*.go", "pkg/api/**/*.md"},
		"scoped-root":    {"**/*.go", "*.toml"},
	}, skillPaths(ScopedFallbackSkills(content, cfg, nil)))
}

func TestScopedFallbackSkills_OptOutIsSkipped(t *testing.T) {
	out := scopedRule("out", "", "src/**")
	out.Metadata.Extra = map[string]string{"fallback": "false"}
	in := scopedRule("in", "", "src/**")
	in.Metadata.Extra = map[string]string{"fallback": "true"}
	skills := ScopedFallbackSkills(&config.ContentTree{Rules: []config.ContentFile{out, in}}, &config.Config{}, nil)
	require.Len(t, skills, 1)
	assert.Equal(t, "rule-in", skills[0].ID)
}
