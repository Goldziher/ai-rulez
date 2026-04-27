package config

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
