package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func TestSortAgentsByPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		agents   []config.Agent
		expected []config.Agent
	}{
		{
			name: "sort by priority descending",
			agents: []config.Agent{
				{Name: "low", Priority: 1},
				{Name: "high", Priority: 10},
				{Name: "medium", Priority: 5},
			},
			expected: []config.Agent{
				{Name: "high", Priority: 10},
				{Name: "medium", Priority: 5},
				{Name: "low", Priority: 1},
			},
		},
		{
			name: "equal priority maintains order",
			agents: []config.Agent{
				{Name: "first", Priority: 5},
				{Name: "second", Priority: 5},
				{Name: "third", Priority: 5},
			},
			expected: []config.Agent{
				{Name: "first", Priority: 5},
				{Name: "second", Priority: 5},
				{Name: "third", Priority: 5},
			},
		},
		{
			name: "mixed priorities",
			agents: []config.Agent{
				{Name: "a", Priority: 3},
				{Name: "b", Priority: 7},
				{Name: "c", Priority: 3},
				{Name: "d", Priority: 10},
				{Name: "e", Priority: 7},
			},
			expected: []config.Agent{
				{Name: "d", Priority: 10},
				{Name: "b", Priority: 7},
				{Name: "e", Priority: 7},
				{Name: "a", Priority: 3},
				{Name: "c", Priority: 3},
			},
		},
		{
			name:     "empty list",
			agents:   []config.Agent{},
			expected: []config.Agent{},
		},
		{
			name: "single agent",
			agents: []config.Agent{
				{Name: "only", Priority: 5},
			},
			expected: []config.Agent{
				{Name: "only", Priority: 5},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{
				Metadata: config.Metadata{Name: "Test"},
				Outputs:  []config.Output{{Path: "test.md"}},
				Agents:   tt.agents,
			}

			sortedData := templates.NewTemplateData(cfg)
			assert.Equal(t, tt.expected, sortedData.Agents)
		})
	}
}

func TestNewTemplateData_WithAgents(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Agent Project",
			Version:     "1.0.0",
			Description: "Project with agents",
		},
		Outputs: []config.Output{
			{Path: ".claude/agents/", Type: "agents"},
		},
		Rules: []config.Rule{
			{Name: "Rule1", Priority: 5, Content: "Content1"},
		},
		Sections: []config.Section{
			{Title: "Section1", Priority: 3, Content: "Section content"},
		},
		Agents: []config.Agent{
			{
				Name:         "agent1",
				Description:  "First agent",
				Priority:     10,
				Tools:        []string{"tool1", "tool2"},
				SystemPrompt: "Agent prompt",
			},
			{
				Name:        "agent2",
				Description: "Second agent",
				Priority:    5,
				Template:    "default",
			},
		},
	}

	data := templates.NewTemplateData(cfg)

	assert.Equal(t, "Agent Project", data.ProjectName)
	assert.Equal(t, "1.0.0", data.Version)
	assert.Equal(t, "Project with agents", data.Description)

	assert.Len(t, data.Agents, 2)
	assert.Equal(t, "agent1", data.Agents[0].Name)
	assert.Equal(t, 10, data.Agents[0].Priority)
	assert.Equal(t, "agent2", data.Agents[1].Name)
	assert.Equal(t, 5, data.Agents[1].Priority)

	assert.Len(t, data.Rules, 1)
	assert.Len(t, data.Sections, 1)
	assert.Equal(t, 1, data.RuleCount)
	assert.Equal(t, 1, data.SectionCount)
}

func TestTemplateData_AllContent(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "Test"},
		Outputs:  []config.Output{{Path: "test.md"}},
		Rules: []config.Rule{
			{Name: "Rule1", Priority: 10, Content: "R1"},
			{Name: "Rule2", Priority: 5, Content: "R2"},
		},
		Sections: []config.Section{
			{Title: "Section1", Priority: 8, Content: "S1"},
			{Title: "Section2", Priority: 3, Content: "S2"},
		},
		Agents: []config.Agent{
			{Name: "Agent1", Description: "A1", Priority: 9},
			{Name: "Agent2", Description: "A2", Priority: 7},
		},
	}

	data := templates.NewTemplateData(cfg)

	assert.Equal(t, 2, data.RuleCount)
	assert.Equal(t, 2, data.SectionCount)
	assert.Len(t, data.Agents, 2)

	assert.Equal(t, 10, data.Rules[0].Priority)
	assert.Equal(t, 5, data.Rules[1].Priority)

	assert.Equal(t, 8, data.Sections[0].Priority)
	assert.Equal(t, 3, data.Sections[1].Priority)

	assert.Equal(t, 9, data.Agents[0].Priority)
	assert.Equal(t, 7, data.Agents[1].Priority)
}

func TestTemplateData_EmptyAgents(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "No Agents"},
		Outputs:  []config.Output{{Path: "test.md"}},
		Rules: []config.Rule{
			{Name: "Rule1", Priority: 5, Content: "Content"},
		},
		Agents: []config.Agent{},
	}

	data := templates.NewTemplateData(cfg)

	assert.Empty(t, data.Agents)
	assert.NotNil(t, data.Agents)
	assert.Len(t, data.Rules, 1)
}

func TestTemplateData_UserRulezWithAgents(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Metadata: config.Metadata{Name: "User Rules Test"},
		Outputs:  []config.Output{{Path: "test.md"}},
		Agents: []config.Agent{
			{Name: "main-agent", Description: "Main", Priority: 5},
		},
		UserRulez: &config.UserRulez{
			Agents: []config.Agent{
				{Name: "user-agent", Description: "User", Priority: 10},
			},
			Rules: []config.Rule{
				{Name: "user-rule", Priority: 8, Content: "User content"},
			},
		},
	}

	data := templates.NewTemplateData(cfg)

	assert.Len(t, data.Agents, 2)

	assert.Equal(t, "user-agent", data.Agents[0].Name)
	assert.Equal(t, 10, data.Agents[0].Priority)

	assert.Equal(t, "main-agent", data.Agents[1].Name)
	assert.Equal(t, 5, data.Agents[1].Priority)

	assert.Len(t, data.Rules, 1)
	assert.Equal(t, "user-rule", data.Rules[0].Name)
}
