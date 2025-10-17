package agents

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

type AgentResponse interface {
	Validate() error
}

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

type AgentDefinition struct {
	Name        string `json:"name" validate:"required,min=3,max=50,lowercase,alphanum"`
	Description string `json:"description,omitempty" validate:"required,min=10,max=500"`
	Role        string `json:"role,omitempty"`
	Expertise   string `json:"expertise,omitempty"`
}

func (a *AgentDefinition) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	matched, err := regexp.MatchString("^[a-z][a-z0-9-]*$", a.Name)
	if err != nil {
		return fmt.Errorf("failed to validate agent name: %w", err)
	}
	if !matched {
		return fmt.Errorf("agent name must be lowercase with hyphens only")
	}
	description := a.normalizedDescription()
	if description == "" {
		return fmt.Errorf("agent description is required")
	}
	if len(description) < 10 {
		return fmt.Errorf("agent description must be at least 10 characters")
	}
	if len(description) > 500 {
		return fmt.Errorf("agent description must be less than 500 characters")
	}
	a.Description = description
	return nil
}

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

func (a *AgentDefinition) normalizedDescription() string {
	desc := strings.TrimSpace(a.Description)
	if desc != "" {
		return desc
	}

	var parts []string

	if role := strings.TrimSpace(a.Role); role != "" {
		parts = append(parts, role)
	}
	if expertise := strings.TrimSpace(a.Expertise); expertise != "" {
		parts = append(parts, expertise)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " — ")
}

func extractJSON(output string) (string, error) {
	start := strings.Index(output, "{")
	if start == -1 {
		return "", fmt.Errorf("no JSON object found in output")
	}

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
					return output[start : i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("incomplete JSON object in output")
}

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

type ContentSimilarity struct {
	threshold float64
}

func NewContentSimilarity(threshold float64) *ContentSimilarity {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.7
	}
	return &ContentSimilarity{threshold: threshold}
}

func (cs *ContentSimilarity) calculateCosineSimilarity(text1, text2 string) float64 {
	tokens1 := cs.tokenize(text1)
	tokens2 := cs.tokenize(text2)

	tf1 := cs.termFrequency(tokens1)
	tf2 := cs.termFrequency(tokens2)

	allTerms := make(map[string]bool)
	for term := range tf1 {
		allTerms[term] = true
	}
	for term := range tf2 {
		allTerms[term] = true
	}

	var dotProduct, norm1, norm2 float64
	for term := range allTerms {
		freq1 := tf1[term]
		freq2 := tf2[term]

		dotProduct += freq1 * freq2
		norm1 += freq1 * freq1
		norm2 += freq2 * freq2
	}

	if norm1 == 0 || norm2 == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

func (cs *ContentSimilarity) tokenize(text string) []string {
	text = strings.ToLower(text)
	tokens := strings.FieldsFunc(text, func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	})

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true, "use": true, "using": true,
		"all": true, "any": true, "some": true, "this": true, "that": true, "these": true,
		"those": true, "when": true, "where": true, "why": true, "how": true,
	}

	var filtered []string
	for _, token := range tokens {
		if len(token) > 2 && !stopWords[token] {
			filtered = append(filtered, token)
		}
	}

	return filtered
}

func (cs *ContentSimilarity) termFrequency(tokens []string) map[string]float64 {
	tf := make(map[string]float64)
	total := float64(len(tokens))

	for _, token := range tokens {
		tf[token]++
	}

	for term := range tf {
		tf[term] /= total
	}

	return tf
}

func (cs *ContentSimilarity) IsSimilar(content1, content2 string) bool {
	similarity := cs.calculateCosineSimilarity(content1, content2)
	return similarity >= cs.threshold
}

func (cs *ContentSimilarity) FindSimilarRule(newRule RuleResponse, existingRules []RuleResponse) (similarRule *RuleResponse, similarity float64) {
	var bestMatch *RuleResponse
	var bestSimilarity float64

	for i := range existingRules {
		similarity := cs.calculateCosineSimilarity(newRule.Content, existingRules[i].Content)
		if similarity > bestSimilarity && similarity >= cs.threshold {
			bestSimilarity = similarity
			bestMatch = &existingRules[i]
		}
	}

	return bestMatch, bestSimilarity
}

func (cs *ContentSimilarity) FindSimilarSection(newSection SectionResponse, existingSections []SectionResponse) (similarSection *SectionResponse, similarity float64) {
	var bestMatch *SectionResponse
	var bestSimilarity float64

	for i := range existingSections {
		similarity := cs.calculateCosineSimilarity(newSection.Content, existingSections[i].Content)
		if similarity > bestSimilarity && similarity >= cs.threshold {
			bestSimilarity = similarity
			bestMatch = &existingSections[i]
		}
	}

	return bestMatch, bestSimilarity
}

func (cs *ContentSimilarity) MergeRules(rule1, rule2 RuleResponse) RuleResponse {
	if len(rule2.Content) > len(rule1.Content) {
		rule1, rule2 = rule2, rule1
	}

	if rule2.Priority == "critical" && rule1.Priority != "critical" {
		rule1.Priority = rule2.Priority
	}

	if cs.hasUniqueContent(rule1.Content, rule2.Content) {
		rule1.Content = cs.combineContent(rule1.Content, rule2.Content)
	}

	return rule1
}

func (cs *ContentSimilarity) hasUniqueContent(primary, secondary string) bool {
	primaryTokens := cs.tokenize(primary)
	secondaryTokens := cs.tokenize(secondary)

	primarySet := make(map[string]bool)
	for _, token := range primaryTokens {
		primarySet[token] = true
	}

	uniqueTokens := 0
	for _, token := range secondaryTokens {
		if !primarySet[token] {
			uniqueTokens++
		}
	}

	return float64(uniqueTokens)/float64(len(secondaryTokens)) >= 0.2
}

func (cs *ContentSimilarity) combineContent(primary, secondary string) string {
	return primary
}
