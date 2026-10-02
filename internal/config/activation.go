package config

import "strings"

// Trigger constants for Windsurf rules (matches Windsurf's actual field names)
const (
	TriggerManual        = "manual"         // Manual activation via @mention (default, no frontmatter needed)
	TriggerAlwaysOn      = "always_on"      // Always active in every interaction
	TriggerModelDecision = "model_decision" // AI decides based on context
	TriggerGlob          = "glob"           // Activate based on file path patterns
)

// IsValidTriggerMode checks if the trigger mode is valid
func IsValidTriggerMode(mode string) bool {
	switch mode {
	case TriggerManual, TriggerAlwaysOn, TriggerModelDecision, TriggerGlob:
		return true
	default:
		return false
	}
}

// GetTriggerMode retrieves the trigger mode from metadata
// Returns "manual" as default if not specified or invalid
func (m *Metadata) GetTriggerMode() string {
	if m == nil {
		return TriggerManual
	}

	mode, ok := m.Extra["trigger"]
	if !ok {
		return TriggerManual // default value
	}

	if !IsValidTriggerMode(mode) {
		// Invalid mode, return default without blocking
		// The generator will log a warning
		return TriggerManual
	}

	return mode
}

// GetTriggerDescription retrieves the description for model_decision mode
func (m *Metadata) GetTriggerDescription() string {
	if m == nil {
		return ""
	}
	return m.Extra["description"]
}

// GetTriggerGlob retrieves the glob pattern for glob mode
func (m *Metadata) GetTriggerGlob() string {
	if m == nil {
		return ""
	}
	return m.Extra["glob"]
}

// GetTriggerKeywords retrieves the trigger keywords for manual mode.
// Returns a copy of the typed Keywords slice (already sorted by the scanner).
func (m *Metadata) GetTriggerKeywords() []string {
	if m == nil || len(m.Keywords) == 0 {
		return nil
	}
	return append([]string(nil), m.Keywords...)
}

// ShouldRenderTriggerFrontmatter checks if trigger frontmatter should be rendered
// Returns true if trigger mode is non-default or has additional config
func (m *Metadata) ShouldRenderTriggerFrontmatter() bool {
	if m == nil {
		return false
	}

	mode := m.GetTriggerMode()
	desc := m.GetTriggerDescription()
	glob := m.GetTriggerGlob()

	// Render if non-default mode or has extra config
	return mode != TriggerManual || desc != "" || glob != ""
}

const (
	boolTrue  = "true"
	boolFalse = "false"
)

// ActivationMode says when a rule or context file applies.
type ActivationMode string

// Activation modes.
const (
	ActivationAlways ActivationMode = "always" // applies in every interaction
	ActivationGlob   ActivationMode = "glob"   // applies when matching files are in play
	ActivationAuto   ActivationMode = "auto"   // the model decides from the description
	ActivationManual ActivationMode = "manual" // applies only when explicitly invoked
)

// Sources of a resolved activation.
const (
	ActivationSourceActivation  = "activation"
	ActivationSourceTrigger     = "trigger"
	ActivationSourceAlwaysApply = "alwaysApply"
	ActivationSourceDerived     = "derived"
)

// IsValid reports whether the mode is one of the four known modes.
func (a ActivationMode) IsValid() bool {
	switch a {
	case ActivationAlways, ActivationGlob, ActivationAuto, ActivationManual:
		return true
	default:
		return false
	}
}

// Activation is the resolved activation of a rule or context file.
type Activation struct {
	Mode        ActivationMode
	Globs       []string
	Description string
	// Source records which field decided Mode: "activation", "trigger",
	// "alwaysApply", or "derived".
	Source string
}

// NormalizeGlobs splits each element on commas that are not inside braces or
// brackets and not escaped with a backslash, trims whitespace, drops empties
// and removes duplicates, keeping first-seen order. "*.{ts,tsx}" and
// "src/[a,b]*.ts" stay one glob, a backslash-escaped comma is kept verbatim,
// and "a, b" becomes two. An unbalanced "{" or "[" suppresses splitting for
// the rest of that element.
func NormalizeGlobs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	var out []string
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		for _, part := range splitGlobList(item) {
			g := strings.TrimSpace(part)
			if g == "" {
				continue
			}
			if _, dup := seen[g]; dup {
				continue
			}
			seen[g] = struct{}{}
			out = append(out, g)
		}
	}
	return out
}

// splitGlobList splits one string on top-level, unescaped commas.
func splitGlobList(item string) []string {
	var parts []string
	braces, brackets, start := 0, 0, 0
	for i := 0; i < len(item); i++ {
		switch item[i] {
		case '\\':
			i++ // skip the escaped byte
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case ',':
			if braces == 0 && brackets == 0 {
				parts = append(parts, item[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, item[start:])
}

// ActivationValue returns the declared `activation`, trimmed and lowercased.
func (m *Metadata) ActivationValue() ActivationMode {
	if m == nil {
		return ""
	}
	return ActivationMode(strings.ToLower(strings.TrimSpace(m.Activation)))
}

// ResolveActivation resolves when the item applies. Precedence: explicit
// `activation`, legacy Windsurf `trigger`, legacy Cursor `alwaysApply`, then
// derived (globs present means glob, otherwise always). A description alone
// never implies auto. Safe on a nil receiver (always).
func (m *Metadata) ResolveActivation() Activation {
	if m == nil {
		return Activation{Mode: ActivationAlways, Source: ActivationSourceDerived}
	}

	globs := m.PathScope()
	if legacy := m.Extra["glob"]; legacy != "" {
		globs = NormalizeGlobs(append(append([]string(nil), globs...), legacy))
	}
	act := Activation{Globs: globs, Description: m.Extra["description"]}

	switch {
	case m.ActivationValue().IsValid():
		act.Mode, act.Source = m.ActivationValue(), ActivationSourceActivation
	case triggerMode(m.Extra["trigger"]) != "":
		act.Mode, act.Source = triggerMode(m.Extra["trigger"]), ActivationSourceTrigger
	default:
		act.Mode, act.Source = m.alwaysApplyMode(act)
	}
	return act
}

// alwaysApplyMode maps the legacy Cursor alwaysApply flag, falling back to the
// derived default when it is absent or unparseable.
func (m *Metadata) alwaysApplyMode(act Activation) (mode ActivationMode, source string) {
	switch strings.ToLower(strings.TrimSpace(m.Extra["alwaysApply"])) {
	case boolTrue:
		return ActivationAlways, ActivationSourceAlwaysApply
	case boolFalse:
		switch {
		case len(act.Globs) > 0:
			return ActivationGlob, ActivationSourceAlwaysApply
		case act.Description != "":
			return ActivationAuto, ActivationSourceAlwaysApply
		default:
			return ActivationManual, ActivationSourceAlwaysApply
		}
	}
	if len(act.Globs) > 0 {
		return ActivationGlob, ActivationSourceDerived
	}
	return ActivationAlways, ActivationSourceDerived
}

// triggerMode maps a Windsurf trigger value to an ActivationMode ("" if unknown).
func triggerMode(trigger string) ActivationMode {
	switch strings.ToLower(strings.TrimSpace(trigger)) {
	case TriggerAlwaysOn:
		return ActivationAlways
	case TriggerGlob:
		return ActivationGlob
	case TriggerModelDecision:
		return ActivationAuto
	case TriggerManual:
		return ActivationManual
	default:
		return ""
	}
}
