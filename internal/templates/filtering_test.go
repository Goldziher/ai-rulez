package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func TestNewTemplateDataForOutput(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Project",
			Version:     "1.0.0",
			Description: "Test configuration",
		},
		Rules: []config.Rule{
			{
				Name:     "Global Rule",
				Priority: 10,
				Content:  "Applies everywhere",
				Targets:  []string{}, // No targets
			},
			{
				Name:     "Claude Only Rule",
				Priority: 20,
				Content:  "Only for CLAUDE.md",
				Targets:  []string{"CLAUDE.md"},
			},
			{
				Name:     "Markdown Rule",
				Priority: 15,
				Content:  "All markdown files",
				Targets:  []string{"*.md"},
			},
			{
				Name:     "Docs Rule",
				Priority: 5,
				Content:  "Files in docs directory",
				Targets:  []string{"docs/*"},
			},
		},
		Sections: []config.Section{
			{
				Title:    "Global Section",
				Priority: 100,
				Content:  "Appears everywhere",
				Targets:  []string{}, // No targets
			},
			{
				Title:    "Claude Intro",
				Priority: 90,
				Content:  "Only in CLAUDE.md",
				Targets:  []string{"CLAUDE.md"},
			},
			{
				Title:    "Docs Header",
				Priority: 80,
				Content:  "For docs directory",
				Targets:  []string{"docs/*"},
			},
		},
		Agents: []config.Agent{
			{
				Name:        "Universal Agent",
				Priority:    50,
				Description: "Works everywhere",
				Targets:     []string{}, // No targets
			},
			{
				Name:        "Claude Agent",
				Priority:    40,
				Description: "Only for CLAUDE.md",
				Targets:     []string{"CLAUDE.md"},
			},
		},
	}

	tests := []struct {
		name           string
		outputPath     string
		expectedRules  []string
		expectedSects  []string
		expectedAgents []string
	}{
		{
			name:           "CLAUDE.md gets global, claude-specific, and markdown items",
			outputPath:     "CLAUDE.md",
			expectedRules:  []string{"Global Rule", "Claude Only Rule", "Markdown Rule"},
			expectedSects:  []string{"Global Section", "Claude Intro"},
			expectedAgents: []string{"Universal Agent", "Claude Agent"},
		},
		{
			name:           "docs/api.md gets global, markdown, and docs items",
			outputPath:     "docs/api.md",
			expectedRules:  []string{"Global Rule", "Markdown Rule", "Docs Rule"},
			expectedSects:  []string{"Global Section", "Docs Header"},
			expectedAgents: []string{"Universal Agent"},
		},
		{
			name:           "other.txt gets only global items",
			outputPath:     "other.txt",
			expectedRules:  []string{"Global Rule"},
			expectedSects:  []string{"Global Section"},
			expectedAgents: []string{"Universal Agent"},
		},
		{
			name:           "empty output path gets all items",
			outputPath:     "",
			expectedRules:  []string{"Global Rule", "Claude Only Rule", "Markdown Rule", "Docs Rule"},
			expectedSects:  []string{"Global Section", "Claude Intro", "Docs Header"},
			expectedAgents: []string{"Universal Agent", "Claude Agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := templates.NewTemplateDataForOutput(cfg, tt.outputPath)
			require.NotNil(t, data)

			// Check metadata is preserved
			assert.Equal(t, "Test Project", data.ProjectName)
			assert.Equal(t, "1.0.0", data.Version)
			assert.Equal(t, "Test configuration", data.Description)

			// Check counts match filtered items
			assert.Equal(t, len(tt.expectedRules), data.RuleCount)
			assert.Equal(t, len(tt.expectedSects), data.SectionCount)
			assert.Equal(t, len(tt.expectedAgents), data.AgentCount)

			// Extract names for comparison
			var ruleNames []string
			for _, rule := range data.Rules {
				ruleNames = append(ruleNames, rule.Name)
			}
			assert.ElementsMatch(t, tt.expectedRules, ruleNames)

			var sectionTitles []string
			for _, section := range data.Sections {
				sectionTitles = append(sectionTitles, section.Title)
			}
			assert.ElementsMatch(t, tt.expectedSects, sectionTitles)

			var agentNames []string
			for _, agent := range data.Agents {
				agentNames = append(agentNames, agent.Name)
			}
			assert.ElementsMatch(t, tt.expectedAgents, agentNames)

			// Check AllContent is correctly filtered and merged
			expectedAllContentCount := len(tt.expectedRules) + len(tt.expectedSects)
			assert.Equal(t, expectedAllContentCount, len(data.AllContent))
		})
	}
}

func TestNewTemplateDataForOutputWithUserRulez(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Test with UserRulez",
		},
		Rules: []config.Rule{
			{
				Name:     "Main Rule",
				Priority: 10,
				Content:  "Main rule content",
				Targets:  []string{"*.md"},
			},
		},
		UserRulez: &config.UserRulez{
			Rules: []config.Rule{
				{
					Name:     "User Rule",
					Priority: 20,
					Content:  "User-specific rule",
					Targets:  []string{"CLAUDE.md"},
				},
				{
					Name:     "Main Rule", // Override main rule
					Priority: 15,
					Content:  "Overridden content",
					Targets:  []string{"*.md"},
				},
			},
		},
	}

	data := templates.NewTemplateDataForOutput(cfg, "CLAUDE.md")
	require.NotNil(t, data)

	// Should have 2 rules: User Rule + overridden Main Rule
	assert.Equal(t, 2, data.RuleCount)
	assert.Len(t, data.Rules, 2)

	// Find the rules
	var userRule, mainRule *config.Rule
	for i := range data.Rules {
		switch data.Rules[i].Name {
		case "User Rule":
			userRule = &data.Rules[i]
		case "Main Rule":
			mainRule = &data.Rules[i]
		}
	}

	require.NotNil(t, userRule, "User Rule should be present")
	require.NotNil(t, mainRule, "Main Rule should be present")

	// Check that Main Rule was overridden by UserRulez
	assert.Equal(t, "Overridden content", mainRule.Content)
	assert.Equal(t, 15, mainRule.Priority)
}

func TestNewTemplateDataBackwardCompatibility(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "Backward Compatibility Test"},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
		},
	}

	// Test that NewTemplateData (without output path) still works
	data := templates.NewTemplateData(cfg)
	require.NotNil(t, data)

	// Should get all rules since no filtering is applied
	assert.Equal(t, 2, data.RuleCount)
	assert.Len(t, data.Rules, 2)

	// Test that it's equivalent to calling NewTemplateDataForOutput with empty path
	dataForOutput := templates.NewTemplateDataForOutput(cfg, "")
	require.NotNil(t, dataForOutput)

	assert.Equal(t, data.RuleCount, dataForOutput.RuleCount)
	assert.Equal(t, data.SectionCount, dataForOutput.SectionCount)
	assert.Equal(t, data.AgentCount, dataForOutput.AgentCount)
}
