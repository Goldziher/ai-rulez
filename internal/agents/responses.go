package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// AgentResponse is the base interface for all agent responses
type AgentResponse interface {
	Validate() error
}

// MetadataResponse represents the response for project description
type MetadataResponse struct {
	Description string `json:"description" validate:"required,min=10,max=500"`
}

func (m *MetadataResponse) Validate() error {
	if m.Description == "" {
		return fmt.Errorf("description is required")
	}
	if len(m.Description) < 10 {
		return fmt.Errorf("description must be at least 10 characters")
	}
	if len(m.Description) > 500 {
		return fmt.Errorf("description must be less than 500 characters")
	}
	return nil
}

// RuleResponse represents a single coding rule
type RuleResponse struct {
	Name     string `json:"name" validate:"required,min=3,max=100"`
	Priority string `json:"priority" validate:"required,oneof=critical high medium low minimal"`
	Content  string `json:"content" validate:"required,min=10,max=1000"`
}

func (r *RuleResponse) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if r.Content == "" {
		return fmt.Errorf("rule content is required")
	}
	validPriorities := map[string]bool{
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
		"minimal":  true,
	}
	if !validPriorities[r.Priority] {
		return fmt.Errorf("priority must be one of: critical, high, medium, low, minimal")
	}
	return nil
}

// RulesResponse represents multiple coding rules
type RulesResponse struct {
	Rules []RuleResponse `json:"rules" validate:"required,min=1,max=10,dive"`
}

func (r *RulesResponse) Validate() error {
	if len(r.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	return nil
}

// SectionResponse represents a documentation section
type SectionResponse struct {
	Name     string `json:"name" validate:"required,min=3,max=100"`
	Priority string `json:"priority" validate:"required,oneof=high medium low"`
	Content  string `json:"content" validate:"required,min=10,max=5000"`
}

func (s *SectionResponse) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("section name is required")
	}
	if s.Content == "" {
		return fmt.Errorf("section content is required")
	}
	validPriorities := map[string]bool{
		"high":   true,
		"medium": true,
		"low":    true,
	}
	if !validPriorities[s.Priority] {
		return fmt.Errorf("priority must be one of: high, medium, low")
	}
	return nil
}

// SectionsResponse represents multiple documentation sections
type SectionsResponse struct {
	Sections []SectionResponse `json:"sections" validate:"required,min=1,max=10,dive"`
}

func (s *SectionsResponse) Validate() error {
	if len(s.Sections) == 0 {
		return fmt.Errorf("at least one section is required")
	}
	for i, section := range s.Sections {
		if err := section.Validate(); err != nil {
			return fmt.Errorf("section %d: %w", i+1, err)
		}
	}
	return nil
}

// AgentDefinition represents an AI agent definition
type AgentDefinition struct {
	Name      string `json:"name" validate:"required,min=3,max=50,lowercase,alphanum"`
	Role      string `json:"role" validate:"required,min=5,max=200"`
	Expertise string `json:"expertise" validate:"required,min=10,max=500"`
}

func (a *AgentDefinition) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	// Check name is lowercase with hyphens
	matched, err := regexp.MatchString("^[a-z][a-z0-9-]*$", a.Name)
	if err != nil {
		return fmt.Errorf("failed to validate agent name: %w", err)
	}
	if !matched {
		return fmt.Errorf("agent name must be lowercase with hyphens only")
	}
	if a.Role == "" {
		return fmt.Errorf("agent role is required")
	}
	if a.Expertise == "" {
		return fmt.Errorf("agent expertise is required")
	}
	return nil
}

// AgentsResponse represents multiple agent definitions
type AgentsResponse struct {
	Agents []AgentDefinition `json:"agents" validate:"required,min=1,max=5,dive"`
}

func (a *AgentsResponse) Validate() error {
	if len(a.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}
	for i, agent := range a.Agents {
		if err := agent.Validate(); err != nil {
			return fmt.Errorf("agent %d: %w", i+1, err)
		}
	}
	return nil
}

// extractJSON attempts to extract a JSON object from agent output
func extractJSON(output string) (string, error) {
	// Try to find JSON object in the output
	// Look for content between { and }
	start := strings.Index(output, "{")
	if start == -1 {
		return "", fmt.Errorf("no JSON object found in output")
	}

	// Find the matching closing brace
	braceCount := 0
	inString := false
	escaped := false

	for i := start; i < len(output); i++ {
		ch := output[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' && !escaped {
			inString = !inString
			continue
		}

		if !inString {
			switch ch {
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					// Found matching closing brace
					return output[start : i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("incomplete JSON object in output")
}

// ParseAgentOutput parses agent output and returns the appropriate response type
func ParseAgentOutput(output string, responseType string) (interface{}, error) {
	jsonStr, err := extractJSON(output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract JSON: %w", err)
	}

	switch responseType {
	case "metadata":
		var resp MetadataResponse
		if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse metadata response: %w", err)
		}
		if err := resp.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
		return &resp, nil

	case "rules":
		var resp RulesResponse
		if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse rules response: %w", err)
		}
		if err := resp.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
		return &resp, nil

	case "sections":
		var resp SectionsResponse
		if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse sections response: %w", err)
		}
		if err := resp.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
		return &resp, nil

	case "agents":
		var resp AgentsResponse
		if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse agents response: %w", err)
		}
		if err := resp.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
		return &resp, nil

	default:
		return nil, fmt.Errorf("unknown response type: %s", responseType)
	}
}
