package config

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// Validate validates a configuration
func (c *Config) Validate() error {
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

	if err := c.validateSkillDescriptions(); err != nil {
		return err
	}

	if err := c.validateInstalledSkills(); err != nil {
		return err
	}

	if err := c.validateDefaults(); err != nil {
		return err
	}

	if err := c.validateAgentEffort(); err != nil {
		return err
	}

	// Warn about missing domain references (non-fatal)
	c.warnMissingDomainReferences()

	return nil
}

// validEffortValues lists the reasoning-effort values accepted by Claude Code
// subagent frontmatter. Lowercase only. Empty string means "not set" and is
// always valid; this list governs explicit values only.
var validEffortValues = []string{"low", "medium", "high", "xhigh", "max", "inherit"}

// validateEffort returns nil for the empty string or any value in validEffortValues.
// Returns an oops-wrapped error otherwise. The fieldPath is embedded in the error
// for actionable messages (e.g., "defaults.effort", "agent[my-agent].effort").
func validateEffort(value, fieldPath string) error {
	if value == "" {
		return nil
	}
	for _, v := range validEffortValues {
		if value == v {
			return nil
		}
	}
	return oops.
		With("field", fieldPath).
		With("actual_value", value).
		With("valid_values", validEffortValues).
		Hint("Use one of: low, medium, high, xhigh, max, inherit (lowercase). Available levels depend on the model.").
		Errorf("invalid effort value %q at %s", value, fieldPath)
}

func (c *Config) validateDefaults() error {
	if c.Defaults == nil {
		return nil
	}
	if err := validateEffort(c.Defaults.Effort, "defaults.effort"); err != nil {
		return err
	}
	for preset, value := range c.Defaults.EffortByPreset {
		if !isValidBuiltInPreset(preset) {
			return oops.
				With("field", "defaults.effort_by_preset").
				With("preset", preset).
				With("available_presets", getBuiltInPresetNames()).
				Hint("Use a built-in preset name as the key (e.g. claude, codex, windsurf).").
				Errorf("unknown preset %q in defaults.effort_by_preset", preset)
		}
		fieldPath := fmt.Sprintf("defaults.effort_by_preset.%s", preset)
		if err := validateEffort(value, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateAgentEffort() error {
	if c.Content == nil {
		return nil
	}

	if err := validateAgentEffortSlice(c.Content.Agents, "root"); err != nil {
		return err
	}

	for domainName, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		if err := validateAgentEffortSlice(domain.Agents, "domain "+domainName); err != nil {
			return err
		}
	}

	return nil
}

func validateAgentEffortSlice(agents []ContentFile, scope string) error {
	for _, agent := range agents {
		if agent.Metadata == nil {
			continue
		}
		fieldPath := fmt.Sprintf("%s agent[%s].effort", scope, agent.Name)
		if err := validateEffort(agent.Metadata.Effort, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateSkillDescriptions() error {
	if c.Content == nil {
		return nil
	}

	if err := validateSkillSlice(c.Content.Skills, "root"); err != nil {
		return err
	}

	for domainName, domain := range c.Content.Domains {
		if domain == nil {
			continue
		}
		if err := validateSkillSlice(domain.Skills, "domain "+domainName); err != nil {
			return err
		}
	}

	return nil
}

func validateSkillSlice(skills []ContentFile, scope string) error {
	for _, skill := range skills {
		if SkillDescription(skill.Metadata) != "" {
			continue
		}

		skillID := SkillID(skill)
		logger.Warn("skill missing 'description' field in frontmatter — using skill name as fallback",
			"scope", scope, "skill", skillID, "path", skill.Path)
	}

	return nil
}

// validateVersion checks that version is "3.0" or "4.0"
func (c *Config) validateVersion() error {
	if c.Version != "3.0" && c.Version != "4.0" {
		return oops.
			With("field", "version").
			With("actual_version", c.Version).
			Hint("Set version to \"3.0\" or \"4.0\" in your config file").
			Errorf("invalid version: expected \"3.0\" or \"4.0\", got %q", c.Version)
	}
	return nil
}

// validateName checks that name is non-empty
func (c *Config) validateName() error {
	if c.Name == "" {
		return oops.
			With("field", "name").
			Hint("Add a 'name' field to your config file\nExample: name: my-project").
			Errorf("required field 'name' is missing")
	}
	return nil
}

// validatePresets validates that at least one preset exists and all are valid
func (c *Config) validatePresets() error {
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
func (c *Config) validatePreset(preset *Preset, index int) error {
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
func (c *Config) validateProfiles() error {
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

// validateInstalledSkills validates the installed_skills section
func (c *Config) validateInstalledSkills() error {
	seen := make(map[string]bool)
	for i, skill := range c.InstalledSkills {
		if skill.Name == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].name", i)).
				Hint("Each installed skill must have a non-empty 'name' field").
				Errorf("installed skill at index %d missing required field 'name'", i)
		}
		if skill.Source == "" {
			return oops.
				With("field", fmt.Sprintf("installed_skills[%d].source", i)).
				With("skill_name", skill.Name).
				Hint("Provide a git URL or local path as the 'source'").
				Errorf("installed skill %q missing required field 'source'", skill.Name)
		}
		if seen[skill.Name] {
			return oops.
				With("field", "installed_skills").
				With("skill_name", skill.Name).
				Hint("Each installed skill must have a unique name").
				Errorf("duplicate installed skill name: %q", skill.Name)
		}
		seen[skill.Name] = true
	}
	return nil
}

// warnMissingDomainReferences logs warnings for domains referenced in profiles but not found in content.
// Domains from includes (FromInclude=true) are checked in the merged content tree.
// If includes are configured but a domain is missing, we emit a debug hint instead
// of a warning since the domain may exist in the include source but failed to resolve.
func (c *Config) warnMissingDomainReferences() {
	if c.Content == nil || len(c.Profiles) == 0 {
		return
	}

	hasIncludes := len(c.Includes) > 0

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
			if hasIncludes {
				logger.Debug("profile references domain not found in merged content (may be missing from include source)",
					"domain", domain)
			} else {
				logger.Warn("profile references non-existent domain", "domain", domain)
			}
		}
	}
}

// getBuiltInPresetNames returns a list of built-in preset names
func getBuiltInPresetNames() []string {
	names := make([]string, 0, len(builtInPresets))
	for name := range builtInPresets {
		names = append(names, name)
	}
	return names
}
