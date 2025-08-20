package config_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Test performance with many rules and complex patterns
func TestFilterPerformance(t *testing.T) {
	t.Parallel()

	// Create many rules with different target patterns
	rules := make([]config.Rule, 1000)
	for i := 0; i < 1000; i++ {
		rules[i] = config.Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Content: fmt.Sprintf("Content %d", i),
			Targets: []string{
				fmt.Sprintf("*.%d", i%10),          // File extension patterns
				fmt.Sprintf("dir%d/*", i%20),       // Directory patterns
				fmt.Sprintf("specific%d.md", i),    // Specific files
				fmt.Sprintf("path/to/file%d.*", i), // Path patterns
			},
		}
	}

	// Test filtering performance
	start := time.Now()
	filtered, err := config.FilterRules(rules, "dir5/file.5", nil)
	duration := time.Since(start)

	assert.NoError(t, err)

	// Should complete quickly even with many rules
	assert.Less(t, duration, 100*time.Millisecond, "Filtering should be fast")
	assert.NotEmpty(t, filtered, "Should match some rules")
}

// Test concurrent access safety
func TestFilterConcurrency(t *testing.T) {
	t.Parallel()

	rules := []config.Rule{
		{Name: "Global", Content: "Global rule", Targets: []string{}},
		{Name: "Markdown", Content: "Markdown rule", Targets: []string{"*.md"}},
		{Name: "Docs", Content: "Docs rule", Targets: []string{"docs/*"}},
	}

	// Run filtering concurrently from multiple goroutines
	const numGoroutines = 100
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(gID int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				path := fmt.Sprintf("docs/file%d_%d.md", gID, j)
				filtered, err := config.FilterRules(rules, path, nil)
				assert.NoError(t, err)
				// Should always get global and docs rules for docs/* paths
				assert.Len(t, filtered, 3, "Should match all rules for path: %s", path)
			}
		}(i)
	}

	wg.Wait()
}

// Test complex real-world scenarios
func TestComplexFilteringScenarios(t *testing.T) {
	t.Parallel()

	// Create a realistic configuration with mixed content types
	rules := []config.Rule{
		{Name: "Global Style Guide", Content: "Applies everywhere", Targets: []string{}},
		{Name: "API Documentation", Content: "For API docs", Targets: []string{"docs/api/*", "*.api.md"}},
		{Name: "Frontend Guidelines", Content: "For frontend", Targets: []string{"src/frontend/*", "*.tsx", "*.jsx"}},
		{Name: "Backend Guidelines", Content: "For backend", Targets: []string{"src/backend/*", "*.go", "*.py"}},
		{Name: "Test Guidelines", Content: "For tests", Targets: []string{"*_test.go", "*_test.py", "test/*", "tests/*"}},
		{Name: "Claude Instructions", Content: "For Claude", Targets: []string{"CLAUDE.md", "claude/*"}},
		{Name: "Config Files", Content: "For config", Targets: []string{"*.yaml", "*.json", "*.toml", "config/*"}},
	}

	sections := []config.Section{
		{Title: "Project Overview", Content: "Overview", Targets: []string{}},
		{Title: "API Reference", Content: "API info", Targets: []string{"docs/api/*"}},
		{Title: "Development Setup", Content: "Setup", Targets: []string{"README.md", "docs/setup.md"}},
	}

	agents := []config.Agent{
		{Name: "General Assistant", Description: "General help", Targets: []string{}},
		{Name: "Code Reviewer", Description: "Reviews code", Targets: []string{"*.go", "*.py", "*.js", "*.ts"}},
		{Name: "Documentation Writer", Description: "Writes docs", Targets: []string{"docs/*", "*.md"}},
	}

	tests := []struct {
		name             string
		outputPath       string
		expectedRules    int
		expectedSections int
		expectedAgents   int
		mustContain      []string // Rule names that must be present
	}{
		{
			name:             "CLAUDE.md gets specific targeting",
			outputPath:       "CLAUDE.md",
			expectedRules:    2, // Global + Claude
			expectedSections: 1, // Project Overview only
			expectedAgents:   2, // General + Documentation Writer
			mustContain:      []string{"Global Style Guide", "Claude Instructions"},
		},
		{
			name:             "API documentation gets comprehensive rules",
			outputPath:       "docs/api/users.md",
			expectedRules:    2, // Global + API
			expectedSections: 2, // Project Overview + API Reference
			expectedAgents:   2, // General + Documentation Writer
			mustContain:      []string{"Global Style Guide", "API Documentation"},
		},
		{
			name:             "Backend Go file gets relevant rules",
			outputPath:       "src/backend/server.go",
			expectedRules:    2, // Global + Backend
			expectedSections: 1, // Project Overview
			expectedAgents:   2, // General + Code Reviewer
			mustContain:      []string{"Global Style Guide", "Backend Guidelines"},
		},
		{
			name:             "Test file gets test-specific rules",
			outputPath:       "src/backend/server_test.go",
			expectedRules:    3, // Global + Backend + Test
			expectedSections: 1, // Project Overview
			expectedAgents:   2, // General + Code Reviewer
			mustContain:      []string{"Global Style Guide", "Backend Guidelines", "Test Guidelines"},
		},
		{
			name:             "Config file gets config rules",
			outputPath:       "config/database.yaml",
			expectedRules:    2, // Global + Config
			expectedSections: 1, // Project Overview
			expectedAgents:   1, // General only
			mustContain:      []string{"Global Style Guide", "Config Files"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filteredRules, err := config.FilterRules(rules, tt.outputPath, nil)
			assert.NoError(t, err)
			filteredSections, err := config.FilterSections(sections, tt.outputPath, nil)
			assert.NoError(t, err)
			filteredAgents, err := config.FilterAgents(agents, tt.outputPath, nil)
			assert.NoError(t, err)

			assert.Len(t, filteredRules, tt.expectedRules, "Wrong number of rules for %s", tt.outputPath)
			assert.Len(t, filteredSections, tt.expectedSections, "Wrong number of sections for %s", tt.outputPath)
			assert.Len(t, filteredAgents, tt.expectedAgents, "Wrong number of agents for %s", tt.outputPath)

			// Check that required rules are present
			ruleNames := make([]string, len(filteredRules))
			for i, rule := range filteredRules {
				ruleNames[i] = rule.Name
			}

			for _, mustHave := range tt.mustContain {
				assert.Contains(t, ruleNames, mustHave, "Rule %s must be present for path %s", mustHave, tt.outputPath)
			}
		})
	}
}

// Test invalid glob patterns and error handling
func TestMatchesTargetErrorHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputPath string
		targets    []string
		expected   bool // Should gracefully handle errors and not match
	}{
		{
			name:       "invalid glob pattern with malformed brackets",
			outputPath: "file.md",
			targets:    []string{"file[.md"}, // Malformed bracket pattern
			expected:   false,
		},
		{
			name:       "valid path with invalid pattern mixed with valid",
			outputPath: "docs/file.md",
			targets:    []string{"docs/*", "file[.md", "*.md"}, // Mixed valid/invalid
			expected:   true,                                   // Should match valid patterns
		},
		{
			name:       "extremely long pattern",
			outputPath: "file.md",
			targets:    []string{strings.Repeat("a", 10000) + "*.md"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Should not panic and should return expected result
			result := config.MatchesTarget(tt.outputPath, tt.targets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark the core MatchesTarget function
func BenchmarkMatchesTarget(b *testing.B) {
	targets := []string{"*.md", "docs/*", "src/**/*.go", "config/*.yaml"}
	path := "docs/api/readme.md"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.MatchesTarget(path, targets)
	}
}

// Benchmark filtering with many rules
func BenchmarkFilterRules(b *testing.B) {
	rules := make([]config.Rule, 1000)
	for i := 0; i < 1000; i++ {
		rules[i] = config.Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Content: fmt.Sprintf("Content %d", i),
			Targets: []string{
				fmt.Sprintf("*.%s", []string{"md", "go", "py", "js", "ts"}[i%5]),
				fmt.Sprintf("dir%d/*", i%50),
			},
		}
	}

	path := "docs/readme.md"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.FilterRules(rules, path, nil)
	}
}
