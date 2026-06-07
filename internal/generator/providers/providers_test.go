package providers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets" // legacy generators, used as the parity baseline
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltin_Claude(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	require.NotNil(t, gen)
	assert.Equal(t, "claude", gen.GetName())

	paths := gen.GetOutputPaths("/repo")
	assert.Contains(t, paths, filepath.Join("/repo", "CLAUDE.md"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude", "skills"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude", "agents"))
}

func TestBuiltinNames(t *testing.T) {
	t.Parallel()

	names, err := providers.BuiltinNames()
	require.NoError(t, err)
	assert.Contains(t, names, "claude")
}

// TestClaudeDSL_MatchesLegacyGenerate is the parity test for commit (a): the
// DSL-backed Generator and the hand-written ClaudePresetGenerator must produce
// the same OutputFile slice (path/IsDir set, content equal) for the canonical
// content trees the legacy tests exercise. If anything drifts here, commit
// (b) will fail at the swap.
func TestClaudeDSL_MatchesLegacyGenerate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content *config.ContentTree
		cfg     *config.Config
	}{
		{
			name: "skills_only",
			content: &config.ContentTree{
				Skills: []config.ContentFile{
					{
						Name:    "test-skill",
						Path:    "/test/skills/test-skill/SKILL.md",
						Content: "Skill content",
						Metadata: &config.Metadata{
							Priority: "medium",
							Extra:    map[string]string{"description": "Test skill"},
						},
					},
				},
			},
			cfg: &config.Config{Name: "test", Description: "test config"},
		},
		{
			name: "agents_only",
			content: &config.ContentTree{
				Agents: []config.ContentFile{
					{
						Name:    "test-agent",
						Content: "Agent system prompt",
						Metadata: &config.Metadata{
							Extra: map[string]string{
								"description": "Test agent description",
								"model":       "haiku",
							},
						},
					},
				},
			},
			cfg: &config.Config{Name: "test", Description: "test config"},
		},
		{
			name:    "empty_tree",
			content: &config.ContentTree{},
			cfg:     &config.Config{Name: "test", Description: "test config"},
		},
		{
			name: "settings_and_plugins_sidecars",
			content: &config.ContentTree{
				Agents: []config.ContentFile{
					{Name: "reviewer", Content: "Review code."},
				},
			},
			cfg: &config.Config{
				Name:        "test",
				Description: "test config",
				MCPServers: map[string]*config.MCPServer{
					"server-a": {Command: "npx", Args: []string{"-y", "a"}},
				},
				Plugins: []config.PluginConfig{
					{Marketplace: "official", Name: "github", Scope: "project"},
				},
			},
		},
	}

	dsl, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	legacy := &presets.ClaudePresetGenerator{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dslOutputs, dslErr := dsl.Generate(tc.content, "/test", tc.cfg)
			legacyOutputs, legacyErr := legacy.Generate(tc.content, "/test", tc.cfg)
			require.NoError(t, dslErr)
			require.NoError(t, legacyErr)

			assertSameOutputSlice(t, legacyOutputs, dslOutputs)
		})
	}
}

func assertSameOutputSlice(t *testing.T, want, got []config.OutputFile) {
	t.Helper()
	wantByPath := indexOutputs(want)
	gotByPath := indexOutputs(got)

	for p, w := range wantByPath {
		g, ok := gotByPath[p]
		if !ok {
			t.Errorf("missing output %q in DSL", p)
			continue
		}
		assert.Equalf(t, w.IsDir, g.IsDir, "IsDir mismatch for %q", p)
		if w.Content != g.Content {
			// Show the first diverging line for an actionable failure.
			diff := firstDifferingLine(w.Content, g.Content)
			t.Errorf("content mismatch for %q:\n  first diff: %s", p, diff)
		}
		if !equalBytes(w.RawContent, g.RawContent) {
			t.Errorf("raw content mismatch for %q", p)
		}
	}
	for p := range gotByPath {
		if _, ok := wantByPath[p]; !ok {
			t.Errorf("unexpected extra output %q in DSL", p)
		}
	}
}

func indexOutputs(outputs []config.OutputFile) map[string]config.OutputFile {
	m := make(map[string]config.OutputFile, len(outputs))
	for _, o := range outputs {
		m[o.Path] = o
	}
	return m
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstDifferingLine(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	n := len(aLines)
	if len(bLines) < n {
		n = len(bLines)
	}
	for i := 0; i < n; i++ {
		if aLines[i] != bLines[i] {
			return "line " + itoa(i+1) + ":\n  legacy: " + aLines[i] + "\n  dsl:    " + bLines[i]
		}
	}
	if len(aLines) != len(bLines) {
		return "line count differs: legacy=" + itoa(len(aLines)) + " dsl=" + itoa(len(bLines))
	}
	return "(no line difference, content bytes still differ)"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
