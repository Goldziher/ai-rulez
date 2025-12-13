package config

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// ValidateV3 validates a V3 configuration
func (c *ConfigV3) ValidateV3() error {
	if err := c.validateVersion(); err != nil {
		return err
	}

	if err := c.validateName(); err != nil {
		return err
	}

	if err := c.validatePresets(); err != nil {
		return err
	}

	if err := c.validateProfiles(); err != nil {
		return err
	}

	// Warn about missing domain references (non-fatal)
	c.warnMissingDomainReferences()

	return nil
}

// validateVersion checks that version is "3.0"
func (c *ConfigV3) validateVersion() error {
	if c.Version != "3.0" {
		return oops.
			With("field", "version").
			With("actual_version", c.Version).
			With("expected_version", "3.0").
			Hint("Set version to \"3.0\" in your config file\nV3 configurations must use version \"3.0\"").
			Errorf("invalid version: expected \"3.0\", got %q", c.Version)
	}
	return nil
}

// validateName checks that name is non-empty
func (c *ConfigV3) validateName() error {
	if c.Name == "" {
		return oops.
			With("field", "name").
			Hint("Add a 'name' field to your config file\nExample: name: my-project").
			Errorf("required field 'name' is missing")
	}
	return nil
}

// validatePresets validates that at least one preset exists and all are valid
func (c *ConfigV3) validatePresets() error {
	if len(c.Presets) == 0 {
		return oops.
			With("field", "presets").
			Hint("Add at least one preset to your config file\nExample: presets: [claude]\nAvailable built-in presets: claude, cursor, gemini, windsurf, copilot, continue-dev, cline").
			Errorf("at least one preset is required")
	}

	for i := range c.Presets {
		if err := c.validatePreset(&c.Presets[i], i); err != nil {
			return err
		}
	}

	return nil
}

// validatePreset validates a single preset
func (c *ConfigV3) validatePreset(preset *PresetV3, index int) error {
	// Check if it's a built-in preset
	if preset.IsBuiltIn() {
		if !isValidBuiltInPreset(preset.BuiltIn) {
			return oops.
				With("field", fmt.Sprintf("presets[%d]", index)).
				With("preset", preset.BuiltIn).
				With("available_presets", getBuiltInPresetNames()).
				Hint(fmt.Sprintf("Use a valid built-in preset name\nAvailable presets: %s", getBuiltInPresetNames())).
				Errorf("unknown built-in preset: %q", preset.BuiltIn)
		}
		return nil
	}

	// Custom preset validation
	if preset.Name == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].name", index)).
			Hint("Custom presets must have a 'name' field\nExample: {name: my-preset, type: markdown, path: CUSTOM.md}").
			Errorf("custom preset missing required field 'name'")
	}

	if preset.Type == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].type", index)).
			With("preset_name", preset.Name).
			Hint("Custom presets must have a 'type' field\nValid types: markdown, directory, json").
			Errorf("custom preset %q missing required field 'type'", preset.Name)
	}

	// Validate preset type
	validTypes := []PresetType{PresetTypeMarkdown, PresetTypeDirectory, PresetTypeJSON}
	isValidType := false
	for _, validType := range validTypes {
		if preset.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return oops.
			With("field", fmt.Sprintf("presets[%d].type", index)).
			With("preset_name", preset.Name).
			With("actual_type", preset.Type).
			With("valid_types", validTypes).
			Hint("Use a valid preset type: markdown, directory, or json").
			Errorf("custom preset %q has invalid type: %q", preset.Name, preset.Type)
	}

	if preset.Path == "" {
		return oops.
			With("field", fmt.Sprintf("presets[%d].path", index)).
			With("preset_name", preset.Name).
			Hint("Custom presets must have a 'path' field\nExample: path: docs/AI_GUIDE.md").
			Errorf("custom preset %q missing required field 'path'", preset.Name)
	}

	return nil
}

// validateProfiles validates the profiles section
func (c *ConfigV3) validateProfiles() error {
	// If default is specified, profiles must be defined
	if c.Default != "" && len(c.Profiles) == 0 {
		return oops.
			With("field", "default").
			With("default_profile", c.Default).
			Hint("If you specify a default profile, you must define profiles\nRemove the 'default' field or add a 'profiles' section").
			Errorf("default profile %q specified but no profiles defined", c.Default)
	}

	// If default is specified, it must exist in profiles
	if c.Default != "" {
		if _, exists := c.Profiles[c.Default]; !exists {
			profileNames := make([]string, 0, len(c.Profiles))
			for name := range c.Profiles {
				profileNames = append(profileNames, name)
			}
			return oops.
				With("field", "default").
				With("default_profile", c.Default).
				With("available_profiles", profileNames).
				Hint(fmt.Sprintf("Set default to one of the defined profiles: %v\nOr add a profile named %q", profileNames, c.Default)).
				Errorf("default profile %q does not exist in profiles", c.Default)
		}
	}

	return nil
}

// warnMissingDomainReferences logs warnings for domains referenced in profiles but not found in content
func (c *ConfigV3) warnMissingDomainReferences() {
	if c.Content == nil || len(c.Profiles) == 0 {
		return
	}

	// Collect all domain names referenced in profiles
	referencedDomains := make(map[string]bool)
	for _, domains := range c.Profiles {
		for _, domain := range domains {
			referencedDomains[domain] = true
		}
	}

	// Check which domains are missing
	for domain := range referencedDomains {
		if _, exists := c.Content.Domains[domain]; !exists {
			logger.Warn("profile references non-existent domain", "domain", domain)
		}
	}
}

// getBuiltInPresetNames returns a list of built-in preset names
func getBuiltInPresetNames() []string {
	names := make([]string, 0, len(builtInPresetsV3))
	for name := range builtInPresetsV3 {
		names = append(names, name)
	}
	return names
}
