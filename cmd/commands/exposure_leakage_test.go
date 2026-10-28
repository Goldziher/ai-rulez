package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestRunExposureLeakageRejects(t *testing.T) {
	tests := []struct {
		name  string
		flags func(root string) exposureFlags
		want  string
	}{
		{"unknown plugin mode", func(root string) exposureFlags { return exposureFlags{plugins: "some", root: root} }, `unknown --plugins "some"`},
		{"no area map", func(root string) exposureFlags { return exposureFlags{root: root} }, "no area map"},
		{"a missing areas file", func(root string) exposureFlags {
			return exposureFlags{root: root, areasFile: filepath.Join(root, "nope.toml")}
		}, "AR9W5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.flags(t.TempDir())
			_, err := runExposureLeakage(t.Context(), &f)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestRunExposureLeakageWithoutAConfiguration(t *testing.T) {
	// Arrange
	root := t.TempDir()
	areasFile := filepath.Join(t.TempDir(), "areas.toml")
	require.NoError(t, os.WriteFile(areasFile, []byte("[[area]]\nname = \"billing\"\nterms = [\"invoice\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# Guide\n\nSend the invoice.\n"), 0o600))
	f := exposureFlags{root: root, areasFile: areasFile}

	// Act
	rep, err := runExposureLeakage(t.Context(), &f)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, rep.ArtifactsChecked)
	assert.Equal(t, []exposure.Leak{{
		Severity: exposure.LeakViolation, Code: "AR9W0", Area: "billing", Term: "invoice", Kind: exposure.KindClaudeMD,
		Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: exposure.RootScope, Line: 3, Excerpt: "Send the invoice.",
	}}, rep.Findings)
	assert.Empty(t, rep.StaleAcceptances)
	assert.False(t, rep.OK())
}

func TestWriteLeakageText(t *testing.T) {
	rep := &exposure.LeakageReport{
		ArtifactsChecked: 3,
		Findings: []exposure.Leak{
			{Severity: exposure.LeakViolation, Code: "AR9W0", Area: "search", Term: "ranker", Kind: exposure.KindClaudeMD, Path: "CLAUDE.md", Line: 4, Excerpt: "tune the ranker"},
			{Severity: exposure.LeakViolation, Code: "AR9W0", Area: "billing", Term: "invoice", Kind: exposure.KindMCPServer, Path: ".mcp.json", Line: 1, Excerpt: "invoice-db"},
			{Severity: exposure.LeakViolation, Code: "AR9W0", Area: "billing", Term: "ledger", Kind: exposure.KindClaudeMD, Path: "CLAUDE.md", Line: 9, Excerpt: "the ledger"},
			{Severity: exposure.LeakAccepted, Code: "AR9W0", Area: "billing", Term: "invoice", Kind: exposure.KindSkill, Path: "s", Line: 1, Excerpt: "s: invoices", Reason: "kept on purpose"},
			{Severity: exposure.LeakWarning, Code: "AR9W1", Area: "billing/search", Term: "account", Kind: exposure.KindAgent, Path: "a", Line: 1, Excerpt: "a: account"},
		},
		Unchecked:        []exposure.Unchecked{{Path: ".claude/skills/big/SKILL.md", Kind: exposure.KindSkill, Name: "big", Reason: "over the read limit"}},
		StaleAcceptances: []string{"lint.accept[2] (AR9W0 in CLAUDE.md) accepted no finding; remove it or fix its path or match"},
	}
	var out bytes.Buffer

	require.NoError(t, writeLeakageText(&out, rep))

	for _, want := range []string{
		"Leakage at session start: 3 artifacts checked\n",
		"violations: 3  warnings: 1  accepted: 1\n",
		"violations by area:\n  billing: 2\n  search: 1\n",
		"violations by kind:\n  claude_md: 2\n  mcp_server: 1\n",
		"violations by term:\n  search/ranker: 1\n  billing/invoice: 1\n  billing/ledger: 1\n",
		"  violation AR9W0 search/ranker CLAUDE.md:4: tune the ranker\n",
		"  accepted AR9W0 billing/invoice s:1: s: invoices\n    accepted: kept on purpose\n",
		"  warning AR9W1 billing/search/account a:1: a: account\n",
		"unchecked: skill big .claude/skills/big/SKILL.md: over the read limit\n",
		"stale acceptance: lint.accept[2] (AR9W0 in CLAUDE.md) accepted no finding",
	} {
		assert.Contains(t, out.String(), want)
	}
}

func TestWriteLeakageTextWithNothingFound(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, writeLeakageText(&out, &exposure.LeakageReport{ArtifactsChecked: 1}))

	assert.Equal(t, "Leakage at session start: 1 artifact checked\nviolations: 0  warnings: 0  accepted: 0\n", out.String())
}

func TestExposureLintSourceLeakage(t *testing.T) {
	// Arrange
	root := t.TempDir()
	for name, body := range map[string]string{
		"CLAUDE.md":                       "The invoice flow.\n",
		".claude/skills/lookup/SKILL.md":  "---\nname: lookup\ndescription: Finds an account.\n---\n",
		".mcp.json":                       `{"mcpServers": {"invoice-db": {"type": "http", "url": "https://example.com/mcp"}}}`,
		".claude-plugin/marketplace.json": `{"name": "mkt", "plugins": [{"name": "ops", "source": "./tools/ops"}]}`,
		"tools/ops/skills/probe/SKILL.md": "---\nname: probe\ndescription: Probes.\n---\n",
		"services/billing/CLAUDE.md":      "Every invoice, on demand.\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	cfg := &config.Config{BaseDir: root, Exposure: &config.ExposureConfig{
		Areas:      []config.ExposureArea{{Name: "billing", Paths: []string{"services/billing"}, Terms: []string{"invoice"}, Ambiguous: []string{"account"}}},
		SkillAreas: map[string]string{"lookup": "billing", "ops:probe": "billing", "ghost": "billing"},
	}}
	src := exposureLintSource{ctx: t.Context(), cfg: cfg, cache: &exposureCache{}}

	// Act
	got, ok := src.Leakage()

	// Assert
	require.True(t, ok)
	abs, err := filepath.Abs(root)
	require.NoError(t, err)
	assert.Equal(t, lint.ExposureLeakage{
		Checked: 3,
		Leaks: []lint.ExposureLeak{
			{Area: "billing", Term: "invoice", Kind: "mcp_server", Name: "invoice-db", Path: ".mcp.json", Scope: ".", Line: 1, Excerpt: "invoice-db",
				Message: `mcp_server "invoice-db" at session start names term "invoice" of area "billing"`},
			{Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 1, Excerpt: "The invoice flow.",
				Message: `claude_md "CLAUDE.md" at session start names term "invoice" of area "billing"`},
			{Ambiguous: true, Area: "billing", Term: "account", Kind: "skill", Name: "lookup", Path: ".claude/skills/lookup/SKILL.md", Scope: ".", Line: 1, Excerpt: "lookup: Finds an account.",
				Message: `skill "lookup" at session start names term "account" of area "billing"`},
		},
		UnknownSkills: []lint.ExposureSkillArea{{Skill: "ghost", Area: "billing"}},
	}, got)
	assert.Equal(t, []string{
		filepath.Join(abs, ".claude", "settings.json"),
		filepath.Join(abs, ".claude", "settings.local.json"),
		filepath.Join(abs, ".claude-plugin", "marketplace.json"),
	}, src.Inputs())
	assert.Equal(t, abs, src.Root())
	assert.Len(t, src.cache.invs, 2, "the enabled inventory and, for the unknown key, the all-plugins one")
}

// A start-of-session skill whose file is not read (a symlink to outside the root) loads in every session but
// has no text to check: the leakage lists it as unchecked instead of passing it silently or inventing a leak.
func TestExposureLintSourceLeakageListsTheUncheckedArtifacts(t *testing.T) {
	// Arrange
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	require.NoError(t, os.WriteFile(outside, []byte("---\nname: big\ndescription: Every invoice.\n---\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude", "skills", "big"), 0o750))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, ".claude", "skills", "big", "SKILL.md"))
	cfg := &config.Config{BaseDir: root, Exposure: &config.ExposureConfig{
		Areas: []config.ExposureArea{{Name: "billing", Terms: []string{"invoice"}}},
	}}
	src := exposureLintSource{ctx: t.Context(), cfg: cfg, cache: &exposureCache{}}

	// Act
	got, ok := src.Leakage()

	// Assert
	require.True(t, ok)
	assert.Empty(t, got.Leaks, "the text was never read")
	assert.Equal(t, 1, got.Checked)
	require.Len(t, got.Unchecked, 1)
	u := got.Unchecked[0]
	assert.Equal(t, lint.ExposureUnchecked{Path: ".claude/skills/big/SKILL.md", Kind: "skill", Name: "big", Reason: u.Reason}, u)
	assert.Contains(t, u.Reason, "outside the repository root")
}

// --root below the project inventories that directory, while the area map and its path claims stay the
// project's: the claim services/billing owns what lies at --root.
func TestExposureLintSourceBelowTheProjectRoot(t *testing.T) {
	// Arrange
	root := t.TempDir()
	for name, body := range map[string]string{
		"CLAUDE.md":                  "The invoice flow, at the project root.\n",
		"services/billing/CLAUDE.md": "Every invoice.\n",
		"services/billing/.claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Charges a card.\n---\n",
		"services/billing/.claude/agents/clerk.md":        "---\nname: clerk\ndescription: Charges a card.\n---\n",
		"products/search/.claude/skills/rank/SKILL.md":    "---\nname: rank\ndescription: Charges a card.\n---\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	cfg := &config.Config{BaseDir: root, Exposure: &config.ExposureConfig{
		Areas: []config.ExposureArea{
			{Name: "billing", Paths: []string{"services/billing"}, Terms: []string{"invoice"}},
			{Name: "search", Paths: []string{"products/search"}, Terms: []string{"ranker"}},
		},
	}}
	dir := filepath.Join(root, "services", "billing")
	src := exposureLintSource{ctx: t.Context(), cfg: cfg, dir: dir, cache: &exposureCache{}}

	// Act
	leakage, ok := src.Leakage()
	collisions := src.Collisions(0.1)
	start := src.StartExposure()

	// Assert
	require.True(t, ok)
	var paths []string
	for _, l := range leakage.Leaks {
		paths = append(paths, l.Path)
	}
	assert.Equal(t, []string{"CLAUDE.md"}, paths, "the CLAUDE.md of --root, relative to it; the project root's is not read")
	assert.Equal(t, dir, src.Root())
	assert.Empty(t, collisions, "charge and clerk are both billing by the project's claim, so they do not collide")
	require.NotNil(t, start)
	assert.Equal(t, []string{
		filepath.Join(dir, "CLAUDE.md"),
		filepath.Join(dir, ".claude", "agents", "clerk.md"),
		filepath.Join(dir, ".claude", "skills", "charge", "SKILL.md"),
	}, start.StartPaths)
	assert.Contains(t, src.Inputs(), filepath.Join(dir, ".claude", "settings.json"), "the settings of the session's root")
}

func TestExposureLintSourceWithoutAMap(t *testing.T) {
	for _, exp := range []*config.ExposureConfig{nil, {ContextWindow: 1000}, {Areas: []config.ExposureArea{{Name: "a"}, {Name: "a"}}}} {
		src := exposureLintSource{ctx: t.Context(), cfg: &config.Config{BaseDir: t.TempDir(), Exposure: exp}}
		_, ok := src.Leakage()
		assert.False(t, ok, "%+v", exp)
	}
}

func TestLeakageReport(t *testing.T) {
	leak := lint.ExposureLeak{Area: "billing", Term: "invoice", Kind: "claude_md", Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 2, Excerpt: "x"}
	res := &lint.LeakageResult{
		Checked: 2,
		Leaks: []lint.LeakFinding{
			{ExposureLeak: leak, Code: "AR9W0", Severity: lint.SeverityError},
			{ExposureLeak: leak, Code: "AR9W0", Severity: lint.SeverityWarning},
			{ExposureLeak: leak, Code: "AR9W1", Severity: lint.SeverityError, Accepted: true, Reason: "why"},
		},
		Unchecked: []lint.ExposureUnchecked{{Path: "big.md", Kind: "rule", Name: "big", Reason: "over the read limit"}},
		Stale:     []string{"stale"},
	}

	got := leakageReport(res)

	base := exposure.Leak{Area: "billing", Term: "invoice", Kind: exposure.KindClaudeMD, Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: ".", Line: 2, Excerpt: "x"}
	violation, warning, accepted := base, base, base
	violation.Severity, violation.Code = exposure.LeakViolation, "AR9W0"
	warning.Severity, warning.Code = exposure.LeakWarning, "AR9W0"
	accepted.Severity, accepted.Code, accepted.Reason = exposure.LeakAccepted, "AR9W1", "why"
	assert.Equal(t, &exposure.LeakageReport{
		SchemaVersion:    exposure.SchemaLeakage,
		ArtifactsChecked: 2,
		Findings:         []exposure.Leak{violation, warning, accepted},
		Unchecked:        []exposure.Unchecked{{Path: "big.md", Kind: exposure.KindRule, Name: "big", Reason: "over the read limit"}},
		StaleAcceptances: []string{"stale"},
	}, got)
}

// The leakage run indexes the project root when --root lies inside it, so [[lint.accept]] paths stay relative to
// the project as in validate; a --root elsewhere is indexed on its own.
func TestLeakageTreeRoot(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()

	assert.Equal(t, base, leakageTreeRoot(base, base))
	assert.Equal(t, base, leakageTreeRoot(base, filepath.Join(base, "services", "billing")))
	assert.Equal(t, other, leakageTreeRoot(base, other))
}
