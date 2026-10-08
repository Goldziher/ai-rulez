package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

func llmsTxtTree(base string) *config.ContentTree {
	file := func(kind, name, content string, meta *config.Metadata) config.ContentFile {
		return config.ContentFile{Name: name, Path: filepath.Join(base, ".ai-rulez", kind, name+".md"), Content: content, Metadata: meta}
	}
	return &config.ContentTree{
		Rules:   []config.ContentFile{file("rules", "zeta", "# Zeta\n\nLast rule.\n", nil), file("rules", "alpha", "No heading here.\n\nSecond.", &config.Metadata{Extra: map[string]string{"description": "Explicit [text]"}})},
		Context: []config.ContentFile{file("context", "ctx", "# Ctx\n\n- list\n\nProse line.\n", nil)},
		Agents:  []config.ContentFile{file("agents", "bot", "# Bot\n\nHelps.\n", nil)},
		Domains: map[string]*config.Domain{"web": {Name: "web", Rules: []config.ContentFile{file("rules", "react", "# React\n\nHooks.\n", nil)}}},
	}
}

func generateLLMsTxt(t *testing.T, cfg *config.Config) map[string]string {
	t.Helper()
	base := t.TempDir()
	cfg.BaseDir = base
	outs, err := (&LLMsTxtPresetGenerator{}).Generate(llmsTxtTree(base), base, cfg)
	require.NoError(t, err)
	got := map[string]string{}
	for _, o := range outs {
		rel, relErr := filepath.Rel(base, o.Path)
		require.NoError(t, relErr)
		got[filepath.ToSlash(rel)] = string(o.RawContent)
	}
	return got
}

func TestLLMsTxtPresetRendersValidIndex(t *testing.T) {
	got := generateLLMsTxt(t, &config.Config{Name: "demo", Description: "A demo."})
	require.Len(t, got, 1)
	index := got["llms.txt"]
	assert.Contains(t, index, "- [alpha](.ai-rulez/rules/alpha.md): Explicit \\[text\\]\n")
	assert.Contains(t, index, "- [Zeta](.ai-rulez/rules/zeta.md): Last rule.\n")
	assert.Contains(t, index, "- [React](.ai-rulez/rules/react.md): Domain web. Hooks.\n")
	assert.Contains(t, index, "- [Ctx](.ai-rulez/context/ctx.md): Prose line.\n")
	assert.NotContains(t, index, "Optional", "agents are not included by default")
	assert.Empty(t, llmstxt.Validate([]byte(index)))
	assert.Less(t, strings.Index(index, "alpha"), strings.Index(index, "Zeta"), "items are sorted by name")
	assert.Less(t, strings.Index(index, "Zeta"), strings.Index(index, "React"), "domains follow the root content")
}

func TestLLMsTxtPresetOptions(t *testing.T) {
	cfg := &config.Config{Name: "demo", LLMsTxt: &config.LLMsTxtConfig{
		Dir: "docs", Title: "Custom", Summary: "Sum", Full: true, Include: []string{"agents", "rules"},
	}}
	got := generateLLMsTxt(t, cfg)
	require.Len(t, got, 2)
	index := got["docs/llms.txt"]
	assert.True(t, strings.HasPrefix(index, "# Custom\n\n> Sum\n"))
	assert.Contains(t, index, "(../.ai-rulez/rules/zeta.md)")
	assert.Contains(t, index, "## Optional\n\n- [Bot](../.ai-rulez/agents/bot.md): Helps.\n")
	assert.NotContains(t, index, "Context")
	assert.Empty(t, llmstxt.Validate([]byte(index)))
	full := got["docs/llms-full.txt"]
	assert.Contains(t, full, "## Zeta\n\nSource: .ai-rulez/rules/zeta.md\n\n### Zeta\n\nLast rule.\n")
	assert.Equal(t, got, generateLLMsTxt(t, cfg), "output is deterministic")
}
