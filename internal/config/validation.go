package config

import (
	"github.com/samber/oops"
)

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Metadata.Name == "" {
		return oops.
			With("field", "metadata.name").
			With("context", "config metadata").
			Hint("Add the required field 'metadata.name' to your configuration\nAdd a name field to the metadata section\nExample: metadata: {name: 'My Project'}").
			Errorf("required field 'metadata.name' is missing")
	}

	// Validate target references in rules
	for _, rule := range c.Rules {
		if len(rule.Targets) > 0 {
			_, err := ResolveTargets(rule.Targets, c.Targets)
			if err != nil {
				return oops.
					With("field", "rules").
					With("rule_name", rule.Name).
					With("targets", rule.Targets).
					Hint("Check that all target references in rules are defined in the targets section\nDefine missing targets in the targets section\nExample: targets:\n  backend: ['src/**/*.go']\n  frontend: ['web/**/*.ts']").
					Wrapf(err, "invalid target reference in rule '%s'", rule.Name)
			}
		}
	}

	// Validate target references in sections
	for _, section := range c.Sections {
		if len(section.Targets) > 0 {
			_, err := ResolveTargets(section.Targets, c.Targets)
			if err != nil {
				return oops.
					With("field", "sections").
					With("section_name", section.Name).
					With("targets", section.Targets).
					Hint("Check that all target references in sections are defined in the targets section\nDefine missing targets in the targets section\nExample: targets:\n  backend: ['src/**/*.go']\n  frontend: ['web/**/*.ts']").
					Wrapf(err, "invalid target reference in section '%s'", section.Name)
			}
		}
	}

	// Validate target references in agents
	for i := range c.Agents {
		if len(c.Agents[i].Targets) > 0 {
			_, err := ResolveTargets(c.Agents[i].Targets, c.Targets)
			if err != nil {
				return oops.
					With("field", "agents").
					With("agent_name", c.Agents[i].Name).
					With("targets", c.Agents[i].Targets).
					Hint("Check that all target references in agents are defined in the targets section\nDefine missing targets in the targets section\nExample: targets:\n  backend: ['src/**/*.go']\n  frontend: ['web/**/*.ts']").
					Wrapf(err, "invalid target reference in agent '%s'", c.Agents[i].Name)
			}
		}
	}

	return ValidateOutputs(c.Outputs)
}
