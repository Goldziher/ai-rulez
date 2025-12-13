package crud_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
)

func createTestConfig(t *testing.T) (string, *config.Config) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
	}

	err := config.SaveConfig(cfg, configFile)
	require.NoError(t, err)
	return configFile, cfg
}

type TestEntityOps[T any] struct {
	GetSlice func(*config.Config) []T
	SetSlice func(*config.Config, []T)
}

var (
	ruleOps = TestEntityOps[config.Rule]{
		GetSlice: crud.RuleOps.GetSlice,
		SetSlice: crud.RuleOps.SetSlice,
	}

	sectionOps = TestEntityOps[config.Section]{
		GetSlice: crud.SectionOps.GetSlice,
		SetSlice: crud.SectionOps.SetSlice,
	}

	agentOps = TestEntityOps[config.Agent]{
		GetSlice: crud.AgentOps.GetSlice,
		SetSlice: crud.AgentOps.SetSlice,
	}

	outputOps = TestEntityOps[config.Output]{
		GetSlice: crud.OutputOps.GetSlice,
		SetSlice: crud.OutputOps.SetSlice,
	}
)

type TestCase[T any] struct {
	name   string
	setup  []T
	verify func(*testing.T, []T)
}

func runEntityTests[T any](t *testing.T, ops TestEntityOps[T], cases []TestCase[T]) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configFile, _ := createTestConfig(t)

			cfg, err := config.LoadConfig(configFile)
			require.NoError(t, err)

			ops.SetSlice(cfg, tc.setup)
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)

			tc.verify(t, ops.GetSlice(reloaded))
		})
	}
}

func TestRuleCRUD(t *testing.T) {
	cases := []TestCase[config.Rule]{
		{
			name: "add rule",
			setup: []config.Rule{{
				Name:     "test-rule",
				Content:  "Test rule content",
				Priority: config.PriorityMedium,
			}},
			verify: func(t *testing.T, rules []config.Rule) {
				assert.Len(t, rules, 1)
				assert.Equal(t, "test-rule", rules[0].Name)
				assert.Equal(t, "Test rule content", rules[0].Content)
				assert.Equal(t, config.PriorityMedium, rules[0].Priority)
			},
		},
		{
			name: "multiple rules",
			setup: []config.Rule{
				{Name: "rule1", Content: "Content1", Priority: config.PriorityHigh},
				{Name: "rule2", Content: "Content2", Priority: config.PriorityLow},
			},
			verify: func(t *testing.T, rules []config.Rule) {
				assert.Len(t, rules, 2)
				assert.Equal(t, "rule1", rules[0].Name)
				assert.Equal(t, "rule2", rules[1].Name)
				assert.Equal(t, config.PriorityHigh, rules[0].Priority)
				assert.Equal(t, config.PriorityLow, rules[1].Priority)
			},
		},
		{
			name: "rule with targets",
			setup: []config.Rule{{
				Name:     "go-rule",
				Content:  "Go specific rule",
				Priority: config.PriorityHigh,
				Targets:  []string{"*.go", "go.mod"},
			}},
			verify: func(t *testing.T, rules []config.Rule) {
				assert.Len(t, rules, 1)
				assert.Equal(t, "go-rule", rules[0].Name)
				assert.Equal(t, config.PriorityHigh, rules[0].Priority)
				assert.Equal(t, []string{"*.go", "go.mod"}, rules[0].Targets)
			},
		},
	}

	runEntityTests(t, ruleOps, cases)
}

func TestSectionCRUD(t *testing.T) {
	cases := []TestCase[config.Section]{
		{
			name: "add section",
			setup: []config.Section{{
				Name:     "Test Section",
				Content:  "Section content",
				Priority: config.PriorityMedium,
			}},
			verify: func(t *testing.T, sections []config.Section) {
				assert.Len(t, sections, 1)
				assert.Equal(t, "Test Section", sections[0].Name)
				assert.Equal(t, "Section content", sections[0].Content)
				assert.Equal(t, config.PriorityMedium, sections[0].Priority)
			},
		},
		{
			name: "section with targets",
			setup: []config.Section{{
				Name:     "docs-section",
				Content:  "Documentation section",
				Priority: config.PriorityLow,
				Targets:  []string{"docs/**", "*.md"},
			}},
			verify: func(t *testing.T, sections []config.Section) {
				assert.Len(t, sections, 1)
				assert.Equal(t, "docs-section", sections[0].Name)
				assert.Equal(t, config.PriorityLow, sections[0].Priority)
				assert.Equal(t, []string{"docs/**", "*.md"}, sections[0].Targets)
			},
		},
	}

	runEntityTests(t, sectionOps, cases)
}

func TestAgentCRUD(t *testing.T) {
	cases := []TestCase[config.Agent]{
		{
			name: "add agent",
			setup: []config.Agent{{
				Name:         "test-agent",
				Description:  "Test agent",
				Priority:     config.PriorityMedium,
				Tools:        []string{"read", "write"},
				SystemPrompt: "You are a test agent",
			}},
			verify: func(t *testing.T, agents []config.Agent) {
				assert.Len(t, agents, 1)
				assert.Equal(t, "test-agent", agents[0].Name)
				assert.Equal(t, "Test agent", agents[0].Description)
				assert.ElementsMatch(t, []string{"read", "write"}, agents[0].Tools)
				assert.Equal(t, "You are a test agent", agents[0].SystemPrompt)
			},
		},
		{
			name: "agent with targets",
			setup: []config.Agent{{
				Name:         "typescript-agent",
				Description:  "TS specialist",
				Priority:     config.PriorityCritical,
				Tools:        []string{"Read", "Edit"},
				SystemPrompt: "You are a TypeScript specialist",
				Targets:      []string{"**/*.ts", "**/*.tsx"},
			}},
			verify: func(t *testing.T, agents []config.Agent) {
				assert.Len(t, agents, 1)
				assert.Equal(t, "typescript-agent", agents[0].Name)
				assert.Equal(t, config.PriorityCritical, agents[0].Priority)
				assert.Equal(t, []string{"**/*.ts", "**/*.tsx"}, agents[0].Targets)
			},
		},
	}

	runEntityTests(t, agentOps, cases)
}

func TestOutputCRUD(t *testing.T) {
	cases := []TestCase[config.Output]{
		{
			name: "add output",
			setup: []config.Output{
				{Path: "test.md"},
				{
					Path: "new-output.md",
					Template: map[string]interface{}{
						"type":  "builtin",
						"value": "minimal",
					},
				},
			},
			verify: func(t *testing.T, outputs []config.Output) {
				assert.Len(t, outputs, 2)
				assert.Equal(t, "new-output.md", outputs[1].Path)
				template, err := outputs[1].GetTemplate()
				require.NoError(t, err)
				assert.Equal(t, "minimal", template.Value)
			},
		},
		{
			name: "update output template",
			setup: []config.Output{{
				Path: "output.md",
				Template: map[string]interface{}{
					"type":  "builtin",
					"value": "documentation",
				},
			}},
			verify: func(t *testing.T, outputs []config.Output) {
				assert.Len(t, outputs, 1)
				template, err := outputs[0].GetTemplate()
				require.NoError(t, err)
				assert.Equal(t, "documentation", template.Value)
			},
		},
	}

	runEntityTests(t, outputOps, cases)
}

func TestDefaultPriorities(t *testing.T) {
	// V2 configuration is no longer supported - this test is skipped
	// Default priorities are now managed in V3 configuration loader
	t.Skip("V2 configuration (ai-rulez.yaml) is no longer supported - use V3 (.ai-rulez/) configuration")
}
