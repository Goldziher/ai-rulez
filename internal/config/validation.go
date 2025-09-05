package config

import (
	"strings"

	"github.com/samber/oops"
)

func (c *Config) Validate() error {
	if c.Metadata.Name == "" {
		return oops.
			With("field", "metadata.name").
			With("context", "config metadata").
			Hint("Add the required field 'metadata.name' to your configuration\nAdd a name field to the metadata section\nExample: metadata: {name: 'My Project'}").
			Errorf("required field 'metadata.name' is missing")
	}

	for _, rule := range c.Rules {
		if len(rule.Targets) > 0 {
			for _, target := range rule.Targets {
				if strings.HasPrefix(target, "@") {
					return oops.
						With("field", "rules").
						With("rule_name", rule.Name).
						With("invalid_target", target).
						Hint("Named target references (@target-name) are no longer supported. Use direct glob patterns instead.").
						Errorf("invalid target pattern '%s' in rule '%s'", target, rule.Name)
				}
			}
		}
	}

	for _, section := range c.Sections {
		if len(section.Targets) > 0 {
			for _, target := range section.Targets {
				if strings.HasPrefix(target, "@") {
					return oops.
						With("field", "sections").
						With("section_name", section.Name).
						With("invalid_target", target).
						Hint("Named target references (@target-name) are no longer supported. Use direct glob patterns instead.").
						Errorf("invalid target pattern '%s' in section '%s'", target, section.Name)
				}
			}
		}
	}

	for i := range c.Agents {
		if len(c.Agents[i].Targets) > 0 {
			for _, target := range c.Agents[i].Targets {
				if strings.HasPrefix(target, "@") {
					return oops.
						With("field", "agents").
						With("agent_name", c.Agents[i].Name).
						With("invalid_target", target).
						Hint("Named target references (@target-name) are no longer supported. Use direct glob patterns instead.").
						Errorf("invalid target pattern '%s' in agent '%s'", target, c.Agents[i].Name)
				}
			}
		}
	}

	return ValidateOutputs(c.Outputs)
}
