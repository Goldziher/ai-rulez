package rulefiles

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/markdown"
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
	// AppliesTo renders each entry's activation under its heading: an
	// "_Applies to: ..._" line for glob entries and a "_When relevant: ..._"
	// line for auto entries. Manual entries cannot be expressed inline, so they
	// render as always-on and are reported in one aggregated warning per call.
	AppliesTo bool
	// ContextSummary emits the "summary" extra of a context entry (unless
	// Compact) between its heading and body.
	ContextSummary bool
	// Diag collects the downgrade notes of the run; nil drops them.
	Diag *diag.Collector
}

// WriteInlineRules writes the "## Rules" section. Nothing is written for an
// empty list.
func WriteInlineRules(b *strings.Builder, rules []config.ContentFile, opts InlineOpts, rec Recorder) {
	if len(rules) == 0 {
		return
	}
	b.WriteString("## Rules\n\n")
	for i := range rules {
		rule := &rules[i]
		start := mark(rec, b)
		b.WriteString("### ")
		b.WriteString(rule.Name)
		b.WriteString("\n\n")
		if opts.AppliesTo {
			writeActivation(b, opts.Diag, "rules", rule)
		}
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
	for i := range ctxFiles {
		ctx := &ctxFiles[i]
		start := mark(rec, b)
		b.WriteString("### ")
		b.WriteString(ctx.Name)
		b.WriteString("\n\n")
		if opts.AppliesTo {
			writeActivation(b, opts.Diag, "context", ctx)
		}
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

// writeActivation writes the scope or trigger hint for f. Manual activation
// has no inline form, so it is recorded as a downgrade and rendered as
// always-on.
func writeActivation(b *strings.Builder, d *diag.Collector, kind string, f *config.ContentFile) {
	act := f.Metadata.ResolveActivation()
	switch act.Mode {
	case config.ActivationGlob:
		if len(act.Globs) == 0 {
			return
		}
		spans := make([]string, len(act.Globs))
		for i, g := range act.Globs {
			spans[i] = codeSpan(g)
		}
		b.WriteString("_Applies to: " + strings.Join(spans, ", ") + "_\n\n")
	case config.ActivationAuto:
		desc := sanitizeDescription(act.Description)
		if desc == "" {
			return
		}
		b.WriteString("_When relevant: " + desc + "_\n\n")
	case config.ActivationManual:
		d.RecordDowngrade(kind, f.Name, string(config.ActivationManual))
	}
}

// sanitizeDescription flattens a description to one line and strips trailing
// characters that would close or escape the surrounding emphasis.
func sanitizeDescription(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(strings.TrimRight(s, "_\\ "))
}

// codeSpan renders s as a Markdown code span whose fence is one backtick
// longer than the longest backtick run in s.
func codeSpan(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

func mark(rec Recorder, b *strings.Builder) int {
	if rec == nil {
		return 0
	}
	return rec.Mark(b)
}
