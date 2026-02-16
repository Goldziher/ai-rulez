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
func (m *MetadataV3) GetTriggerMode() string {
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
func (m *MetadataV3) GetTriggerDescription() string {
	if m == nil {
		return ""
	}
	return m.Extra["description"]
}

// GetTriggerGlob retrieves the glob pattern for glob mode
func (m *MetadataV3) GetTriggerGlob() string {
	if m == nil {
		return ""
	}
	return m.Extra["glob"]
}

// GetTriggerKeywords retrieves the trigger keywords for manual mode
func (m *MetadataV3) GetTriggerKeywords() []string {
	if m == nil {
		return nil
	}

	// Keywords can be stored as "keywords" in Extra
	keywordsStr, ok := m.Extra["keywords"]
	if !ok {
		return nil
	}

	// For now, return as single-element slice
	// In the future, we might want to parse comma-separated values
	if keywordsStr != "" {
		return []string{keywordsStr}
	}

	return nil
}

// ShouldRenderTriggerFrontmatter checks if trigger frontmatter should be rendered
// Returns true if trigger mode is non-default or has additional config
func (m *MetadataV3) ShouldRenderTriggerFrontmatter() bool {
	if m == nil {
		return false
	}

	mode := m.GetTriggerMode()
	desc := m.GetTriggerDescription()
	glob := m.GetTriggerGlob()

	// Render if non-default mode or has extra config
	return mode != TriggerManual || desc != "" || glob != ""
}
