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

func TestFilterPerformance(t *testing.T) {
	t.Parallel()

	rules := make([]config.Rule, 1000)
	for i := 0; i < 1000; i++ {
		rules[i] = config.Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Content: fmt.Sprintf("Content %d", i),
			Targets: []string{
				fmt.Sprintf("*.%d", i%10),
				fmt.Sprintf("dir%d/*", i%20),
				fmt.Sprintf("specific%d.md", i),
				fmt.Sprintf("path/to/file%d.*", i),
			},
		}
	}

	start := time.Now()
	filtered, err := config.FilterRules(rules, "dir5/file.5", nil)
	duration := time.Since(start)

	assert.NoError(t, err)

	assert.Less(t, duration, 100*time.Millisecond, "Filtering should be fast")
	assert.NotEmpty(t, filtered, "Should match some rules")
}

func TestFilterConcurrency(t *testing.T) {
	t.Parallel()

	rules := []config.Rule{
		{Name: "Global", Content: "Global rule", Targets: []string{}},
		{Name: "Markdown", Content: "Markdown rule", Targets: []string{"*.md"}},
		{Name: "Docs", Content: "Docs rule", Targets: []string{"docs/*"}},
	}

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
				assert.Len(t, filtered, 3, "Should match all rules for path: %s", path)
			}
		}(i)
	}

	wg.Wait()
}

func TestComplexFilteringScenarios(t *testing.T) {
	t.Parallel()

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
		mustContain      []string
	}{
		{
			name:             "CLAUDE.md gets specific targeting",
			outputPath:       "CLAUDE.md",
			expectedRules:    2,
			expectedSections: 1,
			expectedAgents:   2,
			mustContain:      []string{"Global Style Guide", "Claude Instructions"},
		},
		{
			name:             "API documentation gets comprehensive rules",
			outputPath:       "docs/api/users.md",
			expectedRules:    2,
			expectedSections: 2,
			expectedAgents:   2,
			mustContain:      []string{"Global Style Guide", "API Documentation"},
		},
		{
			name:             "Backend Go file gets relevant rules",
			outputPath:       "src/backend/server.go",
			expectedRules:    2,
			expectedSections: 1,
			expectedAgents:   2,
			mustContain:      []string{"Global Style Guide", "Backend Guidelines"},
		},
		{
			name:             "Test file gets test-specific rules",
			outputPath:       "src/backend/server_test.go",
			expectedRules:    3,
			expectedSections: 1,
			expectedAgents:   2,
			mustContain:      []string{"Global Style Guide", "Backend Guidelines", "Test Guidelines"},
		},
		{
			name:             "Config file gets config rules",
			outputPath:       "config/database.yaml",
			expectedRules:    2,
			expectedSections: 1,
			expectedAgents:   1,
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

func TestMatchesTargetErrorHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputPath string
		targets    []string
		expected   bool
	}{
		{
			name:       "invalid glob pattern with malformed brackets",
			outputPath: "file.md",
			targets:    []string{"file[.md"},
			expected:   false,
		},
		{
			name:       "valid path with invalid pattern mixed with valid",
			outputPath: "docs/file.md",
			targets:    []string{"docs/*", "file[.md", "*.md"},
			expected:   true,
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
			result := config.MatchesTarget(tt.outputPath, tt.targets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkMatchesTarget(b *testing.B) {
	targets := []string{"*.md", "docs/*", "src/**/*.go", "config/*.yaml"}
	path := "docs/api/readme.md"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.MatchesTarget(path, targets)
	}
}

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
