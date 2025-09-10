package agents

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
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

// ContentSimilarity provides semantic similarity analysis for avoiding duplicates
type ContentSimilarity struct {
	threshold float64
}

// NewContentSimilarity creates a new similarity analyzer
func NewContentSimilarity(threshold float64) *ContentSimilarity {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.7 // Default 70% similarity threshold
	}
	return &ContentSimilarity{threshold: threshold}
}

// calculateCosineSimilarity computes cosine similarity between two texts
func (cs *ContentSimilarity) calculateCosineSimilarity(text1, text2 string) float64 {
	// Normalize and tokenize
	tokens1 := cs.tokenize(text1)
	tokens2 := cs.tokenize(text2)

	// Create term frequency maps
	tf1 := cs.termFrequency(tokens1)
	tf2 := cs.termFrequency(tokens2)

	// Get unique terms
	allTerms := make(map[string]bool)
	for term := range tf1 {
		allTerms[term] = true
	}
	for term := range tf2 {
		allTerms[term] = true
	}

	// Calculate cosine similarity
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

// tokenize splits text into meaningful tokens, removing stop words
func (cs *ContentSimilarity) tokenize(text string) []string {
	// Convert to lowercase and split on non-alphanumeric chars
	text = strings.ToLower(text)
	tokens := strings.FieldsFunc(text, func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	})

	// Filter out stop words and short tokens
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

// termFrequency calculates term frequency for tokens
func (cs *ContentSimilarity) termFrequency(tokens []string) map[string]float64 {
	tf := make(map[string]float64)
	total := float64(len(tokens))

	for _, token := range tokens {
		tf[token]++
	}

	// Normalize by total count
	for term := range tf {
		tf[term] /= total
	}

	return tf
}

// IsSimilar checks if two content strings are semantically similar
func (cs *ContentSimilarity) IsSimilar(content1, content2 string) bool {
	similarity := cs.calculateCosineSimilarity(content1, content2)
	return similarity >= cs.threshold
}

// FindSimilarRule finds existing rule that's similar to the new rule
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

// FindSimilarSection finds existing section that's similar to the new section
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

// MergeRules intelligently merges similar rules, keeping the more specific/detailed one
func (cs *ContentSimilarity) MergeRules(rule1, rule2 RuleResponse) RuleResponse {
	// Choose the more specific rule (longer content usually means more specific)
	if len(rule2.Content) > len(rule1.Content) {
		rule1, rule2 = rule2, rule1
	}

	// Use higher priority if different
	if rule2.Priority == "critical" && rule1.Priority != "critical" {
		rule1.Priority = rule2.Priority
	}

	// Combine unique aspects if rule2 has valuable additions
	if cs.hasUniqueContent(rule1.Content, rule2.Content) {
		rule1.Content = cs.combineContent(rule1.Content, rule2.Content)
	}

	return rule1
}

// hasUniqueContent checks if the second content has unique information
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

	// If secondary has at least 20% unique tokens, consider merging
	return float64(uniqueTokens)/float64(len(secondaryTokens)) >= 0.2
}

// combineContent intelligently combines two content strings
func (cs *ContentSimilarity) combineContent(primary, secondary string) string {
	// For now, just use the primary (more detailed) content
	// Future enhancement: intelligent content merging
	return primary
}
