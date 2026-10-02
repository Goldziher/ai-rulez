package rulefiles

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

const (
	keyDescription = "description"
	keyTrigger     = "trigger"
	keyAlwaysApply = "alwaysApply"
)

// Note reports something a caller should surface about one rendered item: an
// activation the dialect cannot express, an edge case that was resolved to a
// safe default, or a soft limit overrun.
type Note struct {
	Kind    Kind                  // rule or context
	Name    string                // item name
	Dialect string                // dialect of the target
	Mode    config.ActivationMode // resolved activation of the item
	Text    string
	// Downgrade marks an activation the dialect cannot express, so the item
	// is loaded always. Downgrades are aggregated once per generate run.
	Downgrade bool
}

func newNote(d Dialect, it Item, text string, downgrade bool) Note {
	return Note{
		Kind: it.Kind, Name: it.File.Name, Dialect: string(d), Mode: it.Activation.Mode,
		Text: kindLabel(it.Kind) + " \"" + it.File.Name + "\": " + text, Downgrade: downgrade,
	}
}

// effectiveMode resolves the activation the frontmatter is built from. A glob
// activation without globs (legacy `trigger: glob` with no glob) and an auto
// activation without a description cannot work, so they become manual.
func effectiveMode(d Dialect, it Item) (config.ActivationMode, []Note) {
	act := it.Activation
	switch {
	case act.Mode == "":
		return config.ActivationAlways, nil
	case act.Mode == config.ActivationGlob && len(act.Globs) == 0:
		return config.ActivationManual, []Note{newNote(d, it, "glob activation without globs; treated as manual", false)}
	case act.Mode == config.ActivationAuto && strings.TrimSpace(act.Description) == "":
		return config.ActivationManual, []Note{newNote(d, it, "auto activation without a description; treated as manual", false)}
	}
	return act.Mode, nil
}

// EffectiveMode is the activation an item is rendered with: its resolved mode,
// except that a glob mode without globs and an auto mode without a description
// become manual.
func EffectiveMode(it Item) config.ActivationMode {
	mode, _ := effectiveMode("", it)
	return mode
}

// EffectiveModeOf is EffectiveMode for a content file that is not yet an Item.
func EffectiveModeOf(cf config.ContentFile) config.ActivationMode {
	return EffectiveMode(Item{Activation: cf.Metadata.ResolveActivation()})
}

// OnlyNegatedGlobs reports whether a glob-scoped content file has globs and
// every one of them is negated ("!x").
func OnlyNegatedGlobs(cf config.ContentFile) bool {
	act := cf.Metadata.ResolveActivation()
	if act.Mode != config.ActivationGlob || len(act.Globs) == 0 {
		return false
	}
	kept, _ := splitNegated(act.Globs)
	return len(kept) == 0
}

// Frontmatter returns the frontmatter fields a dialect emits for an item and
// notes for activation modes the dialect cannot express (those fall back to
// always-on) or that had to be adjusted. The map is nil when the dialect emits
// no fields.
func Frontmatter(d Dialect, it Item) (fm map[string]any, notes []Note) {
	act := it.Activation
	mode, notes := effectiveMode(d, it)

	joined := ""
	if mode == config.ActivationGlob && (d == DialectCursor || d == DialectTrigger || d == DialectCopilot) {
		globs := act.Globs
		if d == DialectCopilot {
			var dropped []string
			globs, dropped = splitNegated(globs)
			if len(dropped) > 0 {
				notes = append(notes, newNote(d, it, "Copilot has no negated globs; dropped "+
					strings.Join(dropped, ", "), false))
			}
			if len(globs) == 0 {
				globs = []string{"**"}
			}
		}
		var braceNotes []string
		joined, braceNotes = joinExpanded(it.File.Name, globs)
		for _, text := range braceNotes {
			notes = append(notes, newNote(d, it, text, false))
		}
	}

	var extra []Note
	switch d {
	case DialectClaude, DialectCline:
		fm, extra = listPathsFrontmatter(d, it, mode)
	case DialectCursor:
		fm = cursorFrontmatter(act, mode, joined)
	case DialectTrigger:
		fm = triggerFrontmatter(act, mode, joined)
	case DialectCopilot:
		fm, extra = copilotFrontmatter(it, mode, joined)
	case DialectContinue:
		fm = continueFrontmatter(it, mode)
	case DialectJunie:
		fm, extra = junieNotes(it, mode)
	}
	return fm, mergeNotes(notes, extra)
}

// mergeNotes combines the notes from resolving the effective mode with those
// from the dialect. When the mode had to be adjusted, the dialect's own note
// describes the same cause, so only the specific one is kept; it counts as a
// downgrade if the dialect's note did.
func mergeNotes(modeNotes, dialectNotes []Note) []Note {
	if len(modeNotes) == 0 || len(dialectNotes) == 0 {
		return append(modeNotes, dialectNotes...)
	}
	for _, n := range dialectNotes {
		if n.Downgrade {
			modeNotes[0].Downgrade = true
		}
	}
	return modeNotes
}

// splitNegated separates negated globs ("!x") from the rest.
func splitNegated(globs []string) (kept, dropped []string) {
	for _, g := range globs {
		if strings.HasPrefix(g, "!") {
			dropped = append(dropped, g)
			continue
		}
		kept = append(kept, g)
	}
	return kept, dropped
}

func fallbackNote(d Dialect, it Item, mode config.ActivationMode) []Note {
	return []Note{newNote(d, it, "activation "+string(mode)+" not supported by "+string(d)+"; loaded always", true)}
}

func listPathsFrontmatter(d Dialect, it Item, mode config.ActivationMode) (fm map[string]any, notes []Note) {
	switch mode {
	case config.ActivationGlob:
		return map[string]any{"paths": append([]string(nil), it.Activation.Globs...)}, nil
	case config.ActivationAuto, config.ActivationManual:
		return nil, fallbackNote(d, it, mode)
	}
	return nil, nil
}

func junieNotes(it Item, mode config.ActivationMode) (fm map[string]any, notes []Note) {
	if mode == config.ActivationAuto || mode == config.ActivationManual {
		return nil, fallbackNote(DialectJunie, it, mode)
	}
	return nil, nil
}

// cursorFrontmatter keeps a description on always and glob rules because Cursor
// shows it in its rule UI. It is left off manual rules: in Cursor a description
// without globs or alwaysApply makes a rule agent-requested, not manual.
func cursorFrontmatter(act config.Activation, mode config.ActivationMode, globs string) map[string]any {
	var fm map[string]any
	switch mode {
	case config.ActivationGlob:
		fm = map[string]any{"globs": globs, keyAlwaysApply: false}
	case config.ActivationAuto:
		return map[string]any{keyDescription: act.Description, keyAlwaysApply: false}
	case config.ActivationManual:
		return map[string]any{keyAlwaysApply: false}
	default:
		fm = map[string]any{keyAlwaysApply: true}
	}
	if act.Description != "" {
		fm[keyDescription] = act.Description
	}
	return fm
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

// copilotFrontmatter has no way to express auto or manual activation for
// GitHub.com: without applyTo a file is only used by Copilot code review and
// chat when explicitly attached.
func copilotFrontmatter(it Item, mode config.ActivationMode, globs string) (map[string]any, []Note) {
	const noApplyTo = " is not applied automatically on GitHub.com (no applyTo)"
	switch mode {
	case config.ActivationGlob:
		return map[string]any{"applyTo": globs}, nil
	case config.ActivationAuto:
		return map[string]any{keyDescription: it.Activation.Description},
			[]Note{newNote(DialectCopilot, it, "auto activation"+noApplyTo, false)}
	case config.ActivationManual:
		return nil, []Note{newNote(DialectCopilot, it, "manual activation"+noApplyTo, false)}
	}
	return map[string]any{"applyTo": "**"}, nil
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
