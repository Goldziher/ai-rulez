package cost

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, presets string) *config.Config {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	write(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"t\"\npresets = "+presets+"\ngitignore = false\n")
	write(".ai-rulez/rules/big.md", "---\ndescription: a big always-on rule\n---\n"+strings.Repeat("Always do the careful thing in every file. ", 60))
	write(".ai-rulez/rules/small.md", "---\ndescription: tiny\n---\nBe kind.\n")
	write(".ai-rulez/rules/scoped.md", "---\ndescription: scoped\npaths: [\"src/**\"]\n---\n"+strings.Repeat("Only for sources. ", 30))
	write(".ai-rulez/skills/heavy/SKILL.md", "---\nname: heavy\ndescription: Use when you need the heavy skill for anything at all today.\n---\n"+strings.Repeat("Step by step detail. ", 200))
	write(".ai-rulez/skills/hidden/SKILL.md", "---\nname: hidden\ndescription: Use when a human asks for the hidden skill.\ndisable-model-invocation: true\n---\n"+strings.Repeat("Hidden detail. ", 50))
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

func build(t *testing.T, cfg *config.Config, o Options) *Report {
	t.Helper()
	c, err := tokens.New("")
	require.NoError(t, err)
	o.Counter = c
	r, err := Build(cfg, o)
	require.NoError(t, err)
	return r
}

func find(r *Report, kind, name string) Item {
	for _, it := range r.Items {
		if it.Kind == kind && it.Name == name {
			return it
		}
	}
	return Item{}
}

func TestBuildSplitsAlwaysConditionalAndOnDemand(t *testing.T) {
	r := build(t, fixture(t, `["claude"]`), Options{})

	assert.Equal(t, "claude", r.Target)
	assert.Positive(t, r.Always)
	big, small, scoped := find(r, KindRule, "big"), find(r, KindRule, "small"), find(r, KindRule, "scoped")
	assert.Greater(t, big.Always, small.Always)
	assert.Zero(t, big.OnDemand)
	assert.Zero(t, scoped.Always, "a path-scoped rule is not always loaded")
	assert.Positive(t, scoped.Conditional)

	heavy, hidden := find(r, KindSkill, "heavy"), find(r, KindSkill, "hidden")
	assert.Positive(t, heavy.Always, "the listing entry")
	assert.Greater(t, heavy.OnDemand, heavy.Always, "the body dwarfs the listing")
	assert.Zero(t, hidden.Always, "a model-invisible skill is not listed")
	assert.Positive(t, hidden.OnDemand)
}

func TestTopOffendersAreOrderedAndBounded(t *testing.T) {
	r := build(t, fixture(t, `["claude"]`), Options{Top: 2})
	require.Len(t, r.TopAlways, 2)
	assert.GreaterOrEqual(t, r.TopAlways[0].Always, r.TopAlways[1].Always)
	assert.Equal(t, "big", r.TopAlways[0].Name)
	require.NotEmpty(t, r.TopOnDemand)
	assert.Equal(t, "heavy", r.TopOnDemand[0].Name)
	for _, it := range r.TopOnDemand {
		assert.Positive(t, it.OnDemand, "items that cost nothing are not offenders")
	}
	assert.GreaterOrEqual(t, r.Items[0].Total(), r.Items[len(r.Items)-1].Total(), "items are most expensive first")
}

func TestBudgetsSetExceeded(t *testing.T) {
	cfg := fixture(t, `["claude"]`)
	loose := build(t, cfg, Options{AlwaysBudget: 1_000_000, OnDemandBudget: 1_000_000})
	assert.False(t, loose.Exceeded())
	require.NotNil(t, loose.AlwaysBudget)

	tight := build(t, cfg, Options{AlwaysBudget: 10})
	assert.True(t, tight.Exceeded())
	assert.True(t, tight.AlwaysBudget.Exceeded)
	assert.Nil(t, tight.OnDemandBudget)

	assert.True(t, build(t, cfg, Options{OnDemandBudget: 10}).Exceeded())
	assert.False(t, build(t, cfg, Options{}).Exceeded(), "no ceiling, no failure")
}

func TestTargetSelection(t *testing.T) {
	cfg := fixture(t, `["claude", "codex"]`)
	assert.Equal(t, "codex", build(t, cfg, Options{Target: "codex"}).Target)
	c, err := tokens.New("")
	require.NoError(t, err)
	_, err = Build(cfg, Options{Target: "nope", Counter: c})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target")
	_, err = Build(cfg, Options{})
	assert.Error(t, err, "a counter is required")
}

func TestWriteFormats(t *testing.T) {
	r := build(t, fixture(t, `["claude"]`), Options{Top: 3, AlwaysBudget: 10})

	var text bytes.Buffer
	require.NoError(t, Write(&text, r, FormatText))
	for _, want := range []string{"Context cost", "always loaded", "Top always-loaded items", "rule big", "OVER BUDGET", "biggest offenders: rule big"} {
		assert.Contains(t, text.String(), want)
	}

	var md bytes.Buffer
	require.NoError(t, Write(&md, r, FormatMarkdown))
	assert.Contains(t, md.String(), "| Item | Tokens | Share |")
	assert.Contains(t, md.String(), "| rule big |")

	var js bytes.Buffer
	require.NoError(t, Write(&js, r, FormatJSON))
	var back Report
	require.NoError(t, json.Unmarshal(js.Bytes(), &back))
	assert.Equal(t, r.Always, back.Always)
	assert.True(t, back.AlwaysBudget.Exceeded)

	assert.Error(t, Write(&bytes.Buffer{}, r, "xml"))
}

func TestNumberFormatting(t *testing.T) {
	assert.Equal(t, "0", comma(0))
	assert.Equal(t, "999", comma(999))
	assert.Equal(t, "1,234,567", comma(1234567))
	assert.Equal(t, "-", share(1, 0))
	assert.Equal(t, "33%", share(1, 3))
}
