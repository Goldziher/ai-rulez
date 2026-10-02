package rulefiles

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

type call struct {
	kind  config.PartKind
	label string
	path  string
	text  string
}

type fakeRecorder struct {
	calls []call
}

func (f *fakeRecorder) Mark(b *strings.Builder) int { return b.Len() }

func (f *fakeRecorder) Section(kind config.PartKind, label, sourcePath string, start int, b *strings.Builder) {
	f.calls = append(f.calls, call{kind, label, sourcePath, b.String()[start:]})
}

func TestID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Go Style", "Go-Style"},
		{"a_b/c\\d", "a-b-c-d"},
		{"--x!@#y--", "xy"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ID(tt.in); got != tt.want {
			t.Errorf("ID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteInlineRules(t *testing.T) {
	withPrio := config.ContentFile{
		Name: "r1", Path: "rules/r1.md", Content: "body1",
		Metadata: &config.Metadata{Priority: "high"},
	}
	noPrio := config.ContentFile{Name: "r2", Path: "rules/r2.md", Content: "body2"}
	globRule := config.ContentFile{
		Name: "g", Content: "bodyg",
		Metadata: &config.Metadata{Priority: "high", Globs: []string{"src/**/*.go"}},
	}
	multiGlob := config.ContentFile{
		Name: "m", Content: "bodym",
		Metadata: &config.Metadata{Paths: []string{"a/*.ts", "b/*.ts"}},
	}
	activationGlob := config.ContentFile{
		Name: "ag", Content: "bodyag",
		Metadata: &config.Metadata{Activation: "glob", Globs: []string{"*.md"}},
	}
	autoRule := config.ContentFile{
		Name: "a", Content: "bodya",
		Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "when editing SQL"}},
	}
	manualRule := config.ContentFile{
		Name: "mn", Content: "bodymn",
		Metadata: &config.Metadata{Activation: "manual"},
	}

	tests := []struct {
		name  string
		rules []config.ContentFile
		opts  InlineOpts
		want  string
	}{
		{"empty", nil, InlineOpts{}, ""},
		{"priority", []config.ContentFile{withPrio, noPrio}, InlineOpts{},
			"## Rules\n\n### r1\n\n**Priority:** high\n\nbody1\n\n### r2\n\nbody2\n\n"},
		{"compact", []config.ContentFile{withPrio, noPrio}, InlineOpts{Compact: true},
			"## Rules\n\n### r1\n\nbody1\n\n### r2\n\nbody2\n\n"},
		{"applies to off ignores scope", []config.ContentFile{globRule}, InlineOpts{},
			"## Rules\n\n### g\n\n**Priority:** high\n\nbodyg\n\n"},
		{"glob", []config.ContentFile{globRule}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### g\n\n_Applies to: `src/**/*.go`_\n\n**Priority:** high\n\nbodyg\n\n"},
		{"multiple globs", []config.ContentFile{multiGlob}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### m\n\n_Applies to: `a/*.ts`, `b/*.ts`_\n\nbodym\n\n"},
		{"explicit glob activation", []config.ContentFile{activationGlob}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### ag\n\n_Applies to: `*.md`_\n\nbodyag\n\n"},
		{"auto", []config.ContentFile{autoRule}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### a\n\n_When relevant: when editing SQL_\n\nbodya\n\n"},
		{"manual renders as always", []config.ContentFile{manualRule}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### mn\n\nbodymn\n\n"},
		{"always", []config.ContentFile{withPrio, noPrio}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### r1\n\n**Priority:** high\n\nbody1\n\n### r2\n\nbody2\n\n"},
		{"compact keeps scope line", []config.ContentFile{globRule}, InlineOpts{AppliesTo: true, Compact: true},
			"## Rules\n\n### g\n\n_Applies to: `src/**/*.go`_\n\nbodyg\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteInlineRules(&b, tt.rules, tt.opts, nil)
			if b.String() != tt.want {
				t.Errorf("got %q, want %q", b.String(), tt.want)
			}
		})
	}
}

func TestWriteInlineContext(t *testing.T) {
	withSummary := config.ContentFile{
		Name: "c1", Path: "context/c1.md", Content: "ctx1",
		Metadata: &config.Metadata{Extra: map[string]string{"summary": "sum"}},
	}
	plain := config.ContentFile{Name: "c2", Path: "context/c2.md", Content: "ctx2"}
	items := []config.ContentFile{withSummary, plain}

	tests := []struct {
		name  string
		items []config.ContentFile
		opts  InlineOpts
		want  string
	}{
		{"empty", nil, InlineOpts{ContextSummary: true}, ""},
		{"no summary option", items, InlineOpts{},
			"## Context\n\n### c1\n\nctx1\n\n### c2\n\nctx2\n\n"},
		{"summary", items, InlineOpts{ContextSummary: true},
			"## Context\n\n### c1\n\nsum\n\nctx1\n\n### c2\n\nctx2\n\n"},
		{"summary compact", items, InlineOpts{ContextSummary: true, Compact: true},
			"## Context\n\n### c1\n\nctx1\n\n### c2\n\nctx2\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteInlineContext(&b, tt.items, tt.opts, nil)
			if b.String() != tt.want {
				t.Errorf("got %q, want %q", b.String(), tt.want)
			}
		})
	}
}

func TestRecorderCalls(t *testing.T) {
	rules := []config.ContentFile{{Name: "r1", Path: "p1", Content: "b1"}, {Name: "r2", Path: "p2", Content: "b2"}}
	ctxs := []config.ContentFile{{Name: "c1", Path: "p3", Content: "x"}}

	rec := &fakeRecorder{}
	var b strings.Builder
	WriteInlineRules(&b, rules, InlineOpts{}, rec)
	WriteInlineContext(&b, ctxs, InlineOpts{}, rec)

	want := []call{
		{config.PartKindRootRule, "r1", "p1", "### r1\n\nb1\n\n"},
		{config.PartKindRootRule, "r2", "p2", "### r2\n\nb2\n\n"},
		{config.PartKindRootContext, "c1", "p3", "### c1\n\nx\n\n"},
	}
	if len(rec.calls) != len(want) {
		t.Fatalf("got %d calls, want %d", len(rec.calls), len(want))
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, rec.calls[i], want[i])
		}
	}

	rec = &fakeRecorder{}
	var empty strings.Builder
	WriteInlineRules(&empty, nil, InlineOpts{}, rec)
	WriteInlineContext(&empty, nil, InlineOpts{}, rec)
	if len(rec.calls) != 0 {
		t.Errorf("expected no recorder calls for empty lists, got %d", len(rec.calls))
	}
}

func TestWriteInlineContext_AppliesTo(t *testing.T) {
	items := []config.ContentFile{
		{Name: "g", Content: "cg", Metadata: &config.Metadata{Globs: []string{"docs/**"}}},
		{Name: "a", Content: "ca", Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "for API work"}}},
		{Name: "p", Content: "cp"},
	}
	var b strings.Builder
	WriteInlineContext(&b, items, InlineOpts{AppliesTo: true}, nil)
	want := "## Context\n\n### g\n\n_Applies to: `docs/**`_\n\ncg\n\n" +
		"### a\n\n_When relevant: for API work_\n\nca\n\n### p\n\ncp\n\n"
	if b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}

func TestWriteInlineRules_AppliesToEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		meta *config.Metadata
		want string
	}{
		{"glob without globs", &config.Metadata{Activation: "glob"}, "### x\n\nbody\n\n"},
		{"auto without description", &config.Metadata{Activation: "auto"}, "### x\n\nbody\n\n"},
		{"auto blank description", &config.Metadata{Activation: "auto", Extra: map[string]string{"description": " \n "}}, "### x\n\nbody\n\n"},
		{"auto multiline injection", &config.Metadata{Activation: "auto", Extra: map[string]string{
			"description": "first\n\n# Heading\n- item"}}, "### x\n\n_When relevant: first # Heading - item_\n\nbody\n\n"},
		{"auto trailing underscore", &config.Metadata{Activation: "auto", Extra: map[string]string{
			"description": "ends here_\\_"}}, "### x\n\n_When relevant: ends here_\n\nbody\n\n"},
		{"glob with backtick", &config.Metadata{Globs: []string{"a`b", "`c"}},
			"### x\n\n_Applies to: ``a`b``, `` `c ``_\n\nbody\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			WriteInlineRules(&b, []config.ContentFile{{Name: "x", Content: "body", Metadata: tt.meta}}, InlineOpts{AppliesTo: true}, nil)
			want := "## Rules\n\n" + tt.want
			if b.String() != want {
				t.Errorf("got %q, want %q", b.String(), want)
			}
		})
	}
}

func TestCodeSpan(t *testing.T) {
	tests := []struct{ in, want string }{
		{"*.go", "`*.go`"},
		{"a`b", "``a`b``"},
		{"a``b", "```a``b```"},
		{"`a", "`` `a ``"},
		{"a`", "`` a` ``"},
		{"a\nb", "`a b`"},
	}
	for _, tt := range tests {
		if got := codeSpan(tt.in); got != tt.want {
			t.Errorf("codeSpan(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteInlineRules_AppliesToWithRecorder(t *testing.T) {
	rule := config.ContentFile{Name: "g", Path: "rules/g.md", Content: "body", Metadata: &config.Metadata{Globs: []string{"*.go"}}}
	rec := &fakeRecorder{}
	var b strings.Builder
	WriteInlineRules(&b, []config.ContentFile{rule}, InlineOpts{AppliesTo: true}, rec)
	if len(rec.calls) != 1 {
		t.Fatalf("got %d sections, want 1", len(rec.calls))
	}
	want := "### g\n\n_Applies to: `*.go`_\n\nbody\n\n"
	if rec.calls[0].text != want {
		t.Errorf("section = %q, want %q", rec.calls[0].text, want)
	}
}

func TestDowngrades_AggregatedIntoOneWarning(t *testing.T) {
	var calls [][]any
	orig := warn
	warn = func(_ string, args ...any) { calls = append(calls, args) }
	t.Cleanup(func() { warn = orig })

	manual := config.ContentFile{Name: "m", Content: "b", Metadata: &config.Metadata{Activation: "manual"}}
	other := config.ContentFile{Name: "a", Content: "b", Metadata: &config.Metadata{Activation: "manual"}}
	ResetDowngrades()
	for range 3 { // the same rule rendered into several root files
		var b strings.Builder
		WriteInlineRules(&b, []config.ContentFile{manual, other}, InlineOpts{AppliesTo: true}, nil)
		WriteInlineContext(&b, []config.ContentFile{manual}, InlineOpts{AppliesTo: true}, nil)
	}
	FlushDowngrades()
	FlushDowngrades() // already cleared: must not warn again

	if len(calls) != 1 {
		t.Fatalf("got %d warnings, want 1", len(calls))
	}
	if got, want := calls[0][1], "context m, rules a, rules m"; got != want {
		t.Errorf("items = %q, want %q", got, want)
	}
}
