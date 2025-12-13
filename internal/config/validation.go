package config

import (
	"fmt"
	"strings"

	"github.com/samber/oops"
)

func (c *Config) Validate() error {
	if err := c.validateMetadata(); err != nil {
		return err
	}

	if err := c.validateRuleTargets(); err != nil {
		return err
	}

	if err := c.validateSectionTargets(); err != nil {
		return err
	}

	if err := c.validateAgentTargets(); err != nil {
		return err
	}

	if err := c.validateOutputsOrPresets(); err != nil {
		return err
	}

	return ValidateOutputs(c.Outputs)
}

func (c *Config) validateMetadata() error {
	if c.Metadata.Name == "" {
		return oops.
			With("field", "metadata.name").
			With("context", "config metadata").
			Hint("Add the required field 'metadata.name' to your configuration\nAdd a name field to the metadata section\nExample: metadata: {name: 'My Project'}").
			Errorf("required field 'metadata.name' is missing")
	}
	return nil
}

func (c *Config) validateRuleTargets() error {
	for _, rule := range c.Rules {
		if err := validateTargets(rule.Targets, "rules", rule.Name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateSectionTargets() error {
	for _, section := range c.Sections {
		if err := validateTargets(section.Targets, "sections", section.Name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateAgentTargets() error {
	for i := range c.Agents {
		if err := validateTargets(c.Agents[i].Targets, "agents", c.Agents[i].Name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateOutputsOrPresets() error {
	if len(c.Outputs) == 0 && len(c.Presets) == 0 {
		return oops.
			With("field", "outputs/presets").
			With("context", "configuration").
			Hint("Add either 'outputs' or 'presets' to your configuration\nExample with outputs: outputs: [{path: 'CLAUDE.md'}]\nExample with presets: presets: ['popular']").
			Errorf("either 'outputs' or 'presets' is required")
	}
	return nil
}

func validateTargets(targets []string, fieldType, itemName string) error {
	if len(targets) == 0 {
		return nil
	}

	for _, target := range targets {
		if strings.HasPrefix(target, "@") {
			return oops.
				With("field", fieldType).
				With(fieldType[:len(fieldType)-1]+"_name", itemName).
				With("invalid_target", target).
				Hint("Named target references (@target-name) are no longer supported. Use direct glob patterns instead.").
				Errorf("invalid target pattern '%s' in %s '%s'", target, fieldType[:len(fieldType)-1], itemName)
		}
	}
	return nil
}

func ValidateOutputs(outputs []Output) error {
	if len(outputs) == 0 {
		return nil
	}

	for i, output := range outputs {
		if output.Path == "" {
			return oops.
				With("field", fmt.Sprintf("outputs[%d].path", i)).
				With("context", "output configuration").
				With("output_index", i).
				Hint(fmt.Sprintf("Add the required field 'outputs[%d].path' to your configuration\nSpecify a path for output[%d]\nExample: path: 'CLAUDE.md'", i, i)).
				Errorf("required field 'outputs[%d].path' is missing", i)
		}
	}

	return nil
}
