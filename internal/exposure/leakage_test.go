package exposure

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func leakMap(t *testing.T) *areas.Map {
	t.Helper()
	m, problems := areas.New([]config.ExposureArea{
		{Name: "billing", Paths: []string{"services/billing"}, Terms: []string{"invoice", "ledger_sync"}, Ambiguous: []string{"account"}},
		{Name: "search", Paths: []string{"products/search"}, Terms: []string{"ranker", "invoice"}, Ambiguous: []string{"account", "query"}},
		{Name: "tooling", Terms: []string{"bazel"}, Ambiguous: []string{"build"}, Shared: true},
	}, nil)
	require.Empty(t, problems)
	return m
}

func startArtifact(kind Kind, name, text string) Artifact {
	return Artifact{Kind: kind, Name: name, QualifiedName: name, Path: name, Scope: RootScope, Phase: PhaseStart, Text: text}
}

func TestLeaks(t *testing.T) {
	long := "invoice " + strings.Repeat("é", 200)
	tests := []struct {
		name string
		arts []Artifact
		want []Leak
	}{
		{
			name: "a term in a CLAUDE.md is a violation on its file line, blank lines counted",
			arts: []Artifact{{Kind: KindClaudeMD, Name: "CLAUDE.md", QualifiedName: "CLAUDE.md", Path: "CLAUDE.md", Scope: RootScope, Phase: PhaseStart, LineOffset: 2,
				Text: "# Guide\n\n   \n  Every   Invoice\tgoes here.\n"}},
			want: []Leak{{Severity: LeakViolation, Area: "billing", Term: "invoice", Kind: KindClaudeMD, Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: RootScope, Line: 6, Excerpt: "Every Invoice goes here."},
				{Severity: LeakViolation, Area: "search", Term: "invoice", Kind: KindClaudeMD, Name: "CLAUDE.md", Path: "CLAUDE.md", Scope: RootScope, Line: 6, Excerpt: "Every Invoice goes here."}},
		},
		{
			name: "terms of one line come sorted by term, then in map order",
			arts: []Artifact{startArtifact(KindRule, "r", "the ranker reads ledger_sync")},
			want: []Leak{
				{Severity: LeakViolation, Area: "billing", Term: "ledger_sync", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 1, Excerpt: "the ranker reads ledger_sync"},
				{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 1, Excerpt: "the ranker reads ledger_sync"},
			},
		},
		{
			name: "a term repeated on later lines is reported on each line",
			arts: []Artifact{startArtifact(KindRule, "r", "ranker one\nranker two")},
			want: []Leak{
				{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 1, Excerpt: "ranker one"},
				{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 2, Excerpt: "ranker two"},
			},
		},
		{
			name: "a shared area never leaks",
			arts: []Artifact{startArtifact(KindClaudeMD, "CLAUDE.md", "run bazel test")},
		},
		{
			name: "a path literal is not naming an area",
			arts: []Artifact{startArtifact(KindClaudeMD, "CLAUDE.md", "see services/billing/invoice.go and `products/search/ranker`")},
		},
		{
			name: "an ambiguous term outside a listing is not reported",
			arts: []Artifact{startArtifact(KindClaudeMD, "CLAUDE.md", "the account page")},
		},
		{
			name: "a listing with only ambiguous terms warns once per term with every owner",
			arts: []Artifact{startArtifact(KindSkill, "s", "s: Build an account query.")},
			want: []Leak{
				{Severity: LeakWarning, Area: "billing/search", Term: "account", Kind: KindSkill, Name: "s", Path: "s", Scope: RootScope, Line: 1, Excerpt: "s: Build an account query."},
				{Severity: LeakWarning, Area: "tooling", Term: "build", Kind: KindSkill, Name: "s", Path: "s", Scope: RootScope, Line: 1, Excerpt: "s: Build an account query."},
				{Severity: LeakWarning, Area: "search", Term: "query", Kind: KindSkill, Name: "s", Path: "s", Scope: RootScope, Line: 1, Excerpt: "s: Build an account query."},
			},
		},
		{
			name: "a listing with a term is a violation and no warning",
			arts: []Artifact{startArtifact(KindAgent, "a", "a: Tunes the ranker per account.")},
			want: []Leak{{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindAgent, Name: "a", Path: "a", Scope: RootScope, Line: 1, Excerpt: "a: Tunes the ranker per account."}},
		},
		{
			name: "a shared term in a listing is no violation and still silences the warnings",
			arts: []Artifact{startArtifact(KindSkill, "s", "s: Runs bazel on the account.")},
		},
		{
			name: "an MCP server name and a plugin name are checked",
			arts: []Artifact{startArtifact(KindMCPServer, "ranker-db", "ranker-db"), startArtifact(KindPlugin, "invoice@tools", "invoice@tools")},
			want: []Leak{
				{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindMCPServer, Name: "ranker-db", Path: "ranker-db", Scope: RootScope, Line: 1, Excerpt: "ranker-db"},
				{Severity: LeakViolation, Area: "billing", Term: "invoice", Kind: KindPlugin, Name: "invoice@tools", Path: "invoice@tools", Scope: RootScope, Line: 1, Excerpt: "invoice@tools"},
				{Severity: LeakViolation, Area: "search", Term: "invoice", Kind: KindPlugin, Name: "invoice@tools", Path: "invoice@tools", Scope: RootScope, Line: 1, Excerpt: "invoice@tools"},
			},
		},
		{
			name: "the excerpt is cut to 140 characters",
			arts: []Artifact{startArtifact(KindRule, "r", long)},
			want: []Leak{
				{Severity: LeakViolation, Area: "billing", Term: "invoice", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 1, Excerpt: string([]rune(long)[:140])},
				{Severity: LeakViolation, Area: "search", Term: "invoice", Kind: KindRule, Name: "r", Path: "r", Scope: RootScope, Line: 1, Excerpt: string([]rune(long)[:140])},
			},
		},
		{
			name: "the qualified name of a plugin skill names it",
			arts: []Artifact{{Kind: KindSkill, Source: SourcePlugin, Name: "probe", QualifiedName: "ops:probe", Path: "p/SKILL.md", Scope: RootScope, Phase: PhaseStart, Text: "ops:probe: Tunes the ranker."}},
			want: []Leak{{Severity: LeakViolation, Area: "search", Term: "ranker", Kind: KindSkill, Name: "ops:probe", Path: "p/SKILL.md", Scope: RootScope, Line: 1, Excerpt: "ops:probe: Tunes the ranker."}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Leaks(tt.arts, leakMap(t))

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInventory_LeaksReadTheStartArtifactsFailingClosed(t *testing.T) {
	// Arrange
	at := func(phase Phase, name string) Artifact {
		a := startArtifact(KindRule, name, "the ranker")
		a.Phase = phase
		return a
	}
	inv := &Inventory{Artifacts: []Artifact{at(PhaseStart, "start"), at(PhaseOnDemand, "later"), at(PhaseManual, "manual"), at(PhaseUnknown, "unknown")}}

	// Act
	got := Leaks(inv.Start(), leakMap(t))

	// Assert
	names := make([]string, 0, len(got))
	for _, l := range got {
		names = append(names, l.Name)
	}
	assert.Equal(t, []string{"start", "unknown"}, names)
}

func TestLeaks_NilMapFindsNothing(t *testing.T) {
	assert.Empty(t, Leaks([]Artifact{startArtifact(KindRule, "r", "invoice")}, nil))
}

func TestLeak_Message(t *testing.T) {
	l := Leak{Kind: KindMCPServer, Name: "srv-ranker", Term: "ranker", Area: "search"}
	assert.Equal(t, `mcp_server "srv-ranker" at session start names term "ranker" of area "search"`, l.Message())
}

func TestLeakageReport_OK(t *testing.T) {
	tests := []struct {
		name string
		rep  LeakageReport
		want bool
	}{
		{name: "empty", want: true},
		{name: "warnings and accepted findings pass", rep: LeakageReport{Findings: []Leak{{Severity: LeakWarning}, {Severity: LeakAccepted}}}, want: true},
		{name: "a violation fails", rep: LeakageReport{Findings: []Leak{{Severity: LeakViolation}}}},
		{name: "a stale acceptance fails", rep: LeakageReport{StaleAcceptances: []string{"x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rep.OK())
		})
	}
}

func TestStartInputs(t *testing.T) {
	assert.Equal(t, []string{".claude/settings.json", ".claude/settings.local.json", ".claude-plugin/marketplace.json"}, StartInputs())
}

func TestUnknownSkillAreas(t *testing.T) {
	inv := &Inventory{Artifacts: []Artifact{
		{Kind: KindSkill, Source: SourceFile, Name: "deploy", QualifiedName: "deploy"},
		{Kind: KindSkill, Source: SourceCommand, Name: "ops:ship", QualifiedName: "ops:ship"},
		{Kind: KindSkill, Source: SourcePlugin, Name: "probe", QualifiedName: "kit:probe", Plugin: "kit"},
		{Kind: KindAgent, Source: SourceFile, Name: "reviewer", QualifiedName: "reviewer"},
	}}
	tests := []struct {
		name string
		keys map[string]string
		want []string
	}{
		{name: "no entries", want: nil},
		{name: "a repository skill and a command", keys: map[string]string{"deploy": "a", "ops:ship": "a"}},
		{name: "a plugin skill by either name", keys: map[string]string{"kit:probe": "a", "probe": "a"}},
		{name: "an agent is not a skill, and the rest come sorted", keys: map[string]string{"reviewer": "a", "ghost": "a", "deploy": "a"}, want: []string{"ghost", "reviewer"}},
		{name: "a command is not known by its bare name", keys: map[string]string{"ship": "a"}, want: []string{"ship"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UnknownSkillAreas(tt.keys, inv))
		})
	}
}

func TestUncheckedArtifacts(t *testing.T) {
	read := finish(markdownArtifact(KindClaudeMD, SourceFile, "CLAUDE.md", RootScope, "CLAUDE.md", "Some text.\n"))
	broken := finish(markdownArtifact(KindSkill, SourceFile, ".claude/skills/odd/SKILL.md", RootScope, "odd", "---\nname: odd\ndescription: Use when: x\n---\nBody.\n"))
	unread := finish(unreadArtifact(KindSkill, SourceFile, ".claude/skills/big/SKILL.md", RootScope, "big", "2000000 bytes, over the read limit of 1048576"))

	got := UncheckedArtifacts([]Artifact{read, broken, unread})

	assert.Equal(t, []Unchecked{{Path: ".claude/skills/big/SKILL.md", Kind: KindSkill, Name: "big", Reason: "2000000 bytes, over the read limit of 1048576"}}, got,
		"only the artifact kept unread; a block that does not parse was still read")
	assert.Empty(t, UncheckedArtifacts([]Artifact{read, broken}))
}
