package rulefiles

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
)

// Recorder observes the byte range each inline entry occupies in the output
// builder. It is optional; hand-written presets pass nil.
type Recorder interface {
	// Mark returns the start offset of the entry about to be written.
	Mark(b *strings.Builder) int
	// Section records the bytes written since start as one part.
	Section(kind config.PartKind, label, sourcePath string, start int, b *strings.Builder)
}

// InlineOpts controls how the inline sections are rendered.
type InlineOpts struct {
	// Compact suppresses the per-rule priority line and the per-context summary.
	Compact bool
	// AppliesTo is reserved for rendering path-scope hints; currently unused.
	AppliesTo bool
	// ContextSummary emits the "summary" extra of a context entry (unless
	// Compact) between its heading and body.
	ContextSummary bool
}

// WriteInlineRules writes the "## Rules" section. Nothing is written for an
// empty list.
func WriteInlineRules(b *strings.Builder, rules []config.ContentFile, opts InlineOpts, rec Recorder) {
	if len(rules) == 0 {
		return
	}
	b.WriteString("## Rules\n\n")
	for _, rule := range rules {
		start := mark(rec, b)
		b.WriteString("### ")
		b.WriteString(rule.Name)
		b.WriteString("\n\n")
		if !opts.Compact && rule.Metadata != nil && rule.Metadata.Priority != "" {
			b.WriteString("**Priority:** ")
			b.WriteString(rule.Metadata.Priority)
			b.WriteString("\n\n")
		}
		b.WriteString(markdown.ProcessEmbeddedContent(rule.Content))
		b.WriteString("\n\n")
		if rec != nil {
			rec.Section(config.PartKindRootRule, rule.Name, rule.Path, start, b)
		}
	}
}

// WriteInlineContext writes the "## Context" section. Nothing is written for an
// empty list.
func WriteInlineContext(b *strings.Builder, ctxFiles []config.ContentFile, opts InlineOpts, rec Recorder) {
	if len(ctxFiles) == 0 {
		return
	}
	b.WriteString("## Context\n\n")
	for _, ctx := range ctxFiles {
		start := mark(rec, b)
		b.WriteString("### ")
		b.WriteString(ctx.Name)
		b.WriteString("\n\n")
		if opts.ContextSummary && !opts.Compact && ctx.Metadata != nil && ctx.Metadata.Extra["summary"] != "" {
			b.WriteString(ctx.Metadata.Extra["summary"])
			b.WriteString("\n\n")
		}
		b.WriteString(markdown.ProcessEmbeddedContent(ctx.Content))
		b.WriteString("\n\n")
		if rec != nil {
			rec.Section(config.PartKindRootContext, ctx.Name, ctx.Path, start, b)
		}
	}
}

func mark(rec Recorder, b *strings.Builder) int {
	if rec == nil {
		return 0
	}
	return rec.Mark(b)
}
