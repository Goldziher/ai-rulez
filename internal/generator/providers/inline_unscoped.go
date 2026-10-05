package providers

import (
	"strconv"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// planSplit routes rules and context between the root file and the rule files of
// a split spec. With inline_unscoped, the items the tool's rule files cannot apply
// automatically never reach rulefiles.Plan: they stay inline in the root file, in
// source order, next to what Plan leaves inline.
func (g *Generator) planSplit(spec *OutputSpec, rules, ctx []config.ContentFile, target *rulefiles.Target,
	routing rulefiles.Routing, cfg *config.Config, reg *rulefiles.Registry,
) (files []rulefiles.Item, inlineRules, inlineContext []config.ContentFile, err error) {
	if !spec.InlineUnscoped {
		return rulefiles.Plan(rules, ctx, target, routing, rulefiles.ScopeOf(cfg), reg)
	}
	ruleCands, ruleStay := g.splitUnscoped(rules, "rule", rulefiles.KindRule, *target)
	ctxCands, ctxStay := g.splitUnscoped(ctx, "context", rulefiles.KindContext, *target)
	files, planRules, planContext, err := rulefiles.Plan(ruleCands, ctxCands, target, routing, rulefiles.ScopeOf(cfg), reg)
	if err != nil {
		return nil, nil, nil, err
	}
	return files, mergeInline(rulefiles.KindRule, rules, planRules, ruleStay),
		mergeInline(rulefiles.KindContext, ctx, planContext, ctxStay), nil
}

// splitUnscoped separates the items rule files may take (candidates) from the ones
// that stay in the root file. Of the latter, only those whose targets still allow
// the root file are returned as inline; an item aimed solely at the rules folder is
// reported and dropped, since the folder cannot apply it.
func (g *Generator) splitUnscoped(all []config.ContentFile, noun string, kind rulefiles.Kind, target rulefiles.Target,
) (candidates, inline []config.ContentFile) {
	stay := func(cf config.ContentFile) {
		if rulefiles.InlineAllowed(cf, target) {
			inline = append(inline, cf)
			return
		}
		if rulefiles.FileAllowed(cf, target, kind) {
			logger.Warn(noun+" \""+cf.Name+"\" is targeted only at "+target.Dir+" but "+g.Spec.DisplayName+
				" cannot apply it automatically there; omitted", "path", cf.Path)
		}
	}
	for _, cf := range all {
		switch mode := rulefiles.EffectiveModeOf(cf); {
		case mode == config.ActivationAuto || mode == config.ActivationManual:
			stay(cf)
		case rulefiles.OnlyNegatedGlobs(cf):
			rulefiles.WarnOnlyNegated(noun, cf, g.Spec.Root.File)
			stay(cf)
		default:
			candidates = append(candidates, cf)
		}
	}
	return candidates, inline
}

// mergeInline is what Plan left inline plus the items it never saw, in source
// order.
func mergeInline(kind rulefiles.Kind, all, planned, direct []config.ContentFile) []config.ContentFile {
	key := func(cf config.ContentFile) string {
		return strconv.Itoa(int(kind)) + "\x00" + cf.Path + "\x00" + cf.Name
	}
	want := make(map[string]struct{}, len(planned)+len(direct))
	for _, cf := range planned {
		want[key(cf)] = struct{}{}
	}
	for _, cf := range direct {
		want[key(cf)] = struct{}{}
	}
	var out []config.ContentFile
	for _, cf := range all {
		if _, ok := want[key(cf)]; ok {
			out = append(out, cf)
		}
	}
	return out
}
