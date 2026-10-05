package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func marshalValue(t *testing.T, v any) string {
	t.Helper()
	out, err := yaml.Marshal(map[string]any{"k": v})
	require.NoError(t, err)
	return string(out)
}

func TestParseFrontmatter_KeepsTypedExtras(t *testing.T) {
	src := "---\nname: demo\ndisable-model-invocation: true\nversion: 2\nlast_verified: 2026-10-01\n" +
		"quoted: \"true\"\nmetadata:\n  owner: team-a\n  reviewed: {by: alice, date: 2026-10-01}\n  tags: [a, b]\n---\nBody\n"
	meta, body, malformed := parseFrontmatter(src)
	require.False(t, malformed)
	require.NotNil(t, meta)
	assert.Equal(t, "Body\n", body)

	tests := []struct {
		key  string
		want string
	}{
		{"disable-model-invocation", "k: true\n"},
		{"version", "k: 2\n"},
		{"last_verified", "k: 2026-10-01\n"},
		{"quoted", "k: \"true\"\n"},
		{"metadata", "k:\n    owner: team-a\n    reviewed:\n        by: alice\n        date: 2026-10-01\n    tags:\n        - a\n        - b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			v, ok := meta.TypedExtra(tt.key)
			require.True(t, ok)
			assert.Equal(t, tt.want, marshalValue(t, v))
		})
	}

	assert.Equal(t, "2026-10-01", meta.Extra["last_verified"], "the string form keeps the source text, not the Go time rendering")
	assert.Equal(t, "{owner: team-a, reviewed: {by: alice, date: 2026-10-01}, tags: [a, b]}", meta.Extra["metadata"])
}

func TestTypedExtra_StringFallbackAndMissing(t *testing.T) {
	var nilMeta *Metadata
	_, ok := nilMeta.TypedExtra("x")
	assert.False(t, ok)

	m := &Metadata{Extra: map[string]string{"license": "MIT", "empty": ""}}
	v, ok := m.TypedExtra("license")
	assert.True(t, ok)
	assert.Equal(t, "MIT", v)
	_, ok = m.TypedExtra("empty")
	assert.False(t, ok)
	_, ok = m.TypedExtra("absent")
	assert.False(t, ok)
}

func TestExtraBool(t *testing.T) {
	tests := []struct {
		name      string
		frontend  string
		wantValue bool
		wantSet   bool
	}{
		{"true", "k: true", true, true},
		{"false", "k: false", false, true},
		{"yes", "k: yes", true, true},
		{"off", "k: Off", false, true},
		{"one", "k: 1", true, true},
		{"quoted true", "k: \"true\"", true, true},
		{"junk", "k: maybe", false, false},
		{"absent", "other: 1", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, _, malformed := parseFrontmatter("---\n" + tt.frontend + "\n---\nx\n")
			require.False(t, malformed)
			value, set := meta.ExtraBool("k")
			assert.Equal(t, tt.wantValue, value)
			assert.Equal(t, tt.wantSet, set)
		})
	}
	value, set := (*Metadata)(nil).ExtraBool("k")
	assert.False(t, value)
	assert.False(t, set)
}

func TestSkillSpecFields_OnlySpecKeys(t *testing.T) {
	meta, _, _ := parseFrontmatter("---\nlicense: MIT\ncompatibility: x\nallowed-tools: Read\nmetadata: {a: b}\nother: 1\n---\nx\n")
	fields := meta.SkillSpecFields()
	assert.ElementsMatch(t, []string{"license", "compatibility", "allowed-tools", "metadata"}, keysOf(fields))
	assert.Empty(t, (*Metadata)(nil).SkillSpecFields())
}

func TestNormalizeNode_ResolvesAliasesAndFlowStyle(t *testing.T) {
	meta, _, malformed := parseFrontmatter("---\nbase: &b {x: 1}\nmetadata: *b\n---\nx\n")
	require.False(t, malformed)
	v, ok := meta.TypedExtra("metadata")
	require.True(t, ok)
	assert.Equal(t, "k:\n    x: 1\n", marshalValue(t, v))
}

func TestCodexConfig_ProjectDocLimit(t *testing.T) {
	zero, big := 0, 65536
	assert.Equal(t, DefaultCodexProjectDocMaxBytes, (*CodexConfig)(nil).ProjectDocLimit())
	assert.Equal(t, DefaultCodexProjectDocMaxBytes, (&CodexConfig{}).ProjectDocLimit())
	assert.Equal(t, 0, (&CodexConfig{ProjectDocMaxBytes: &zero}).ProjectDocLimit())
	assert.Equal(t, 65536, (&CodexConfig{ProjectDocMaxBytes: &big}).ProjectDocLimit())
}

func TestClaudeConfig_HidesSkillsFromMenu(t *testing.T) {
	assert.False(t, (*ClaudeConfig)(nil).HidesSkillsFromMenu())
	assert.False(t, (&ClaudeConfig{}).HidesSkillsFromMenu())
	assert.True(t, (&ClaudeConfig{Skills: &ClaudeSkills{HideFromMenu: true}}).HidesSkillsFromMenu())
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
