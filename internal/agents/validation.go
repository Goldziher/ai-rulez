package agents

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"gopkg.in/yaml.v3"
)

func ValidateGeneratedConfig(yamlContent string) ([]string, error) {
	var errors []string

	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &rawConfig); err != nil {
		return []string{fmt.Sprintf("Invalid YAML: %v", err)}, fmt.Errorf("invalid YAML: %w", err)
	}

	if schema, ok := rawConfig["$schema"]; !ok {
		errors = append(errors, "Missing required field: $schema")
	} else if schemaStr, ok := schema.(string); !ok || !strings.Contains(schemaStr, "ai-rulez") {
		errors = append(errors, "Invalid or missing schema URL")
	}

	var cfg config.Config
	if err := yaml.Unmarshal([]byte(yamlContent), &cfg); err != nil {
		return []string{fmt.Sprintf("Invalid config structure: %v", err)}, fmt.Errorf("invalid config: %w", err)
	}

	if cfg.Metadata.Name == "" {
		errors = append(errors, "Missing required field: metadata.name")
	}
	if cfg.Metadata.Version == "" {
		errors = append(errors, "Missing required field: metadata.version")
	}
	if cfg.Metadata.Description == "" {
		errors = append(errors, "Missing required field: metadata.description")
	}

	if len(cfg.Outputs) == 0 {
		errors = append(errors, "Must have at least one output configuration")
	}

	for i, output := range cfg.Outputs {
		if output.File == "" && output.Path == "" {
			errors = append(errors, fmt.Sprintf("Output %d must have either 'file' or 'path' field", i+1))
		}
		if output.Path != "" && output.Type == "" {
			errors = append(errors, fmt.Sprintf("Output %d with 'path' must specify 'type'", i+1))
		}
	}

	if len(cfg.Rules) < 5 {
		errors = append(errors, fmt.Sprintf("Must have at least 5 rules (found %d)", len(cfg.Rules)))
	}

	for i, rule := range cfg.Rules {
		if rule.Name == "" {
			errors = append(errors, fmt.Sprintf("Rule %d missing 'name' field", i+1))
		}
		if rule.Priority < 1 || rule.Priority > 10 {
			errors = append(errors, fmt.Sprintf("Rule '%s' priority must be between 1 and 10 (got %d)", rule.Name, rule.Priority))
		}
		if rule.Content == "" {
			errors = append(errors, fmt.Sprintf("Rule '%s' missing content", rule.Name))
		}
	}

	for i := range cfg.Agents {
		if cfg.Agents[i].Name == "" {
			errors = append(errors, fmt.Sprintf("Agent %d missing 'name' field", i+1))
		}
		if cfg.Agents[i].Description == "" {
			errors = append(errors, fmt.Sprintf("Agent '%s' missing description", cfg.Agents[i].Name))
		}
		if cfg.Agents[i].SystemPrompt == "" {
			errors = append(errors, fmt.Sprintf("Agent '%s' missing system_prompt", cfg.Agents[i].Name))
		}
	}

	if len(errors) > 0 {
		return errors, fmt.Errorf("validation failed with %d errors", len(errors))
	}

	return nil, nil
}

func ExtractYAMLFromResponse(response string) string {
	response = strings.TrimSpace(response)

	if strings.Contains(response, "```yaml") {
		start := strings.Index(response, "```yaml") + 7
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	if strings.Contains(response, "```yml") {
		start := strings.Index(response, "```yml") + 6
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	if strings.HasPrefix(response, "```") && strings.HasSuffix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	if strings.HasPrefix(response, "$schema:") {
		return response
	}

	lines := strings.Split(response, "\n")
	var yamlStart = -1
	for i, line := range lines {
		if strings.HasPrefix(line, "$schema:") {
			yamlStart = i
			break
		}
	}

	if yamlStart >= 0 {
		return strings.Join(lines[yamlStart:], "\n")
	}

	return response
}
