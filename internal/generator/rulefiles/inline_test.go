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
		{"applies to unused", []config.ContentFile{withPrio}, InlineOpts{AppliesTo: true},
			"## Rules\n\n### r1\n\n**Priority:** high\n\nbody1\n\n"},
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
