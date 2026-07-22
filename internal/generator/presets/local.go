package presets

import (
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// RenderLocalRoot renders the body of a machine-local root override file
// (CLAUDE.local.md, AGENTS.local.md, GEMINI.local.md, ...). It emits the same
// generated-file header plus "## Rules" and "## Context" sections, reusing the
// exact inline-rule and inline-context formatting as the committed root file so
// local overrides read identically to the files they augment.
//
// The local tree carries only rules and context (skills/agents are out of scope
// for local overrides). outputFile is the local variant's path relative to the
// base dir, used only for the header banner.
func RenderLocalRoot(local *config.ContentTree, cfg *config.Config, outputFile string) string {
	var builder strings.Builder

	allRules := allInlineRules(local)
	allContext := allInlineContext(local)

	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    time.Now(),
		ConfigFile:   configFileName(cfg),
		OutputFile:   outputFile,
		Config:       cfg,
		RuleCount:    len(allRules),
		SectionCount: 0,
		AgentCount:   0,
	}
	builder.WriteString(templates.GenerateHeader(data))

	if len(allRules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range allRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n")

			if !cfg.IsCompact() && rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			builder.WriteString(markdown.ProcessEmbeddedContent(rule.Content))
			builder.WriteString("\n\n")
		}
	}

	if len(allContext) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range allContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")

			if !cfg.IsCompact() && ctx.Metadata != nil && ctx.Metadata.Extra["summary"] != "" {
				builder.WriteString(ctx.Metadata.Extra["summary"])
				builder.WriteString("\n\n")
			}

			builder.WriteString(markdown.ProcessEmbeddedContent(ctx.Content))
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}
