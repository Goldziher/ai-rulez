package rulefiles

import "github.com/Goldziher/ai-rulez/internal/config"

const (
	keyDescription = "description"
	keyTrigger     = "trigger"
	keyAlwaysApply = "alwaysApply"
)

// Frontmatter returns the frontmatter fields a dialect emits for an item and
// notes for activation modes the dialect cannot express (those fall back to
// always-on). The map is nil when the dialect emits no fields.
func Frontmatter(d Dialect, it Item) (fm map[string]any, notes []string) {
	act := it.Activation
	mode := act.Mode
	if mode == "" || (mode == config.ActivationGlob && len(act.Globs) == 0) {
		mode = config.ActivationAlways
	}

	joined := ""
	if mode == config.ActivationGlob && (d == DialectCursor || d == DialectTrigger || d == DialectCopilot) {
		joined, notes = joinExpanded(it.File.Name, act.Globs)
	}

	switch d {
	case DialectClaude, DialectCline:
		return listPathsFrontmatter(d, it, mode)
	case DialectCursor:
		return cursorFrontmatter(act, mode, joined), notes
	case DialectTrigger:
		return triggerFrontmatter(act, mode, joined), notes
	case DialectCopilot:
		return copilotFrontmatter(act, mode, joined), notes
	case DialectContinue:
		return continueFrontmatter(it, mode), nil
	case DialectJunie:
		return junieNotes(it, mode)
	}
	return nil, nil
}

func fallbackNote(d Dialect, it Item, mode config.ActivationMode) []string {
	return []string{"rule \"" + it.File.Name + "\": activation " + string(mode) +
		" not supported by " + string(d) + "; loaded always"}
}

func listPathsFrontmatter(d Dialect, it Item, mode config.ActivationMode) (fm map[string]any, notes []string) {
	switch mode {
	case config.ActivationGlob:
		return map[string]any{"paths": append([]string(nil), it.Activation.Globs...)}, nil
	case config.ActivationAuto, config.ActivationManual:
		return nil, fallbackNote(d, it, mode)
	}
	return nil, nil
}

func junieNotes(it Item, mode config.ActivationMode) (fm map[string]any, notes []string) {
	if mode == config.ActivationAuto || mode == config.ActivationManual {
		return nil, fallbackNote(DialectJunie, it, mode)
	}
	return nil, nil
}

func cursorFrontmatter(act config.Activation, mode config.ActivationMode, globs string) map[string]any {
	switch mode {
	case config.ActivationGlob:
		return map[string]any{"globs": globs, keyAlwaysApply: false}
	case config.ActivationAuto:
		return map[string]any{keyDescription: act.Description}
	case config.ActivationManual:
		return map[string]any{keyAlwaysApply: false}
	}
	return map[string]any{keyAlwaysApply: true}
}

func triggerFrontmatter(act config.Activation, mode config.ActivationMode, globs string) map[string]any {
	switch mode {
	case config.ActivationGlob:
		return map[string]any{keyTrigger: config.TriggerGlob, "globs": globs}
	case config.ActivationAuto:
		return map[string]any{keyTrigger: config.TriggerModelDecision, keyDescription: act.Description}
	case config.ActivationManual:
		return map[string]any{keyTrigger: config.TriggerManual}
	}
	return map[string]any{keyTrigger: config.TriggerAlwaysOn}
}

func copilotFrontmatter(act config.Activation, mode config.ActivationMode, globs string) map[string]any {
	switch mode {
	case config.ActivationGlob:
		return map[string]any{"applyTo": globs}
	case config.ActivationAuto:
		return map[string]any{keyDescription: act.Description}
	case config.ActivationManual:
		return nil
	}
	return map[string]any{"applyTo": "**"}
}

// continueFrontmatter always carries the rule name, which Continue uses as the
// rule title.
func continueFrontmatter(it Item, mode config.ActivationMode) map[string]any {
	fm := map[string]any{"name": it.File.Name}
	switch mode {
	case config.ActivationGlob:
		fm["globs"] = append([]string(nil), it.Activation.Globs...)
		fm[keyAlwaysApply] = false
	case config.ActivationAuto:
		fm[keyDescription] = it.Activation.Description
	case config.ActivationManual:
		fm[keyAlwaysApply] = false
	default:
		fm[keyAlwaysApply] = true
	}
	return fm
}
