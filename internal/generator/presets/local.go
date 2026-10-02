package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
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
		Timestamp:    cfg.HeaderTimestamp(),
		ConfigFile:   configFileName(cfg),
		OutputFile:   outputFile,
		Config:       cfg,
		RuleCount:    len(allRules),
		SectionCount: 0,
		AgentCount:   0,
	}
	builder.WriteString(templates.GenerateHeader(data))

	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), ContextSummary: true}, nil)

	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), ContextSummary: true}, nil)

	return builder.String()
}
