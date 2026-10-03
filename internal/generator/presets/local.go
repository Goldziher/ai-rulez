package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// RenderLocalRootRules renders the body of a machine-local root override file
// (CLAUDE.local.md, AGENTS.local.md, GEMINI.local.md, ...). It emits the same
// generated-file header plus "## Rules" and "## Context" sections, reusing the
// exact inline-rule and inline-context formatting as the committed root file so
// local overrides read identically to the files they augment.
//
// allRules is the rules the root inlines: the caller removes the ones written
// as personal rule files. Context comes from the local tree (skills/agents are
// out of scope for local overrides). outputFile is the local variant's path
// relative to the base dir, used only for the header banner.
func RenderLocalRootRules(local *config.ContentTree, allRules []config.ContentFile, cfg *config.Config, outputFile string) string {
	var builder strings.Builder

	sharedRoot := strings.Replace(outputFile, ".local.md", ".md", 1)
	allRules = rulefiles.FilterInline(allRules, rulefiles.RootTarget("", sharedRoot))
	allContext := rulefiles.FilterInline(allInlineContext(local), rulefiles.RootTarget("", sharedRoot))

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
	builder.WriteString(localSections(allRules, allContext, cfg))
	return builder.String()
}

// localSections renders the "## Rules" and "## Context" sections of a local root
// file, with the exact formatting of the committed root files.
func localSections(allRules, allContext []config.ContentFile, cfg *config.Config) string {
	var builder strings.Builder
	rulefiles.WriteInlineRules(&builder, allRules, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true}, nil)
	rulefiles.WriteInlineContext(&builder, allContext, rulefiles.InlineOpts{Compact: cfg.IsCompact(), AppliesTo: true, ContextSummary: true}, nil)
	return builder.String()
}
