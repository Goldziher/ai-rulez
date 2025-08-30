package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestAddAgentCommand(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	initialConfig := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Agents: []config.Agent{},
	}

	err := config.SaveConfig(initialConfig, configFile)
	if err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	originalDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalDir)
	}()
	_ = os.Chdir(tempDir)

	tests := []struct {
		name           string
		agentName      string
		description    string
		priority       int
		tools          []string
		systemPrompt   string
		expectedError  bool
		validateConfig func(*testing.T, *config.Config)
	}{
		{
			name:          "add basic agent",
			agentName:     "test-agent",
			description:   "Test agent for validation",
			priority:      5,
			tools:         []string{"tool1", "tool2"},
			systemPrompt:  "You are a test agent",
			expectedError: false,
			validateConfig: func(t *testing.T, cfg *config.Config) {
				if len(cfg.Agents) != 1 {
					t.Errorf("Expected 1 agent, got %d", len(cfg.Agents))
				}
				agent := cfg.Agents[0]
				if agent.Name != "test-agent" {
					t.Errorf("Expected agent name 'test-agent', got '%s'", agent.Name)
				}
				if agent.Description != "Test agent for validation" {
					t.Errorf("Expected description 'Test agent for validation', got '%s'", agent.Description)
				}
				if agent.Priority != 5 {
					t.Errorf("Expected priority 5, got %d", agent.Priority)
				}
				if len(agent.Tools) != 2 {
					t.Errorf("Expected 2 tools, got %d", len(agent.Tools))
				}
				if agent.SystemPrompt != "You are a test agent" {
					t.Errorf("Expected system prompt 'You are a test agent', got '%s'", agent.SystemPrompt)
				}
			},
		},
		{
			name:          "add agent with default priority",
			agentName:     "default-agent",
			description:   "Agent with default priority",
			priority:      0,
			tools:         []string{},
			systemPrompt:  "Default agent",
			expectedError: false,
			validateConfig: func(t *testing.T, cfg *config.Config) {
				if len(cfg.Agents) != 2 {
					t.Errorf("Expected 2 agents, got %d", len(cfg.Agents))
				}
				agent := cfg.Agents[1]
				if agent.Priority != 5 {
					t.Errorf("Expected default priority 5, got %d", agent.Priority)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.LoadConfig(configFile)
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			newAgent := config.Agent{
				Name:         tt.agentName,
				Description:  tt.description,
				Priority:     tt.priority,
				Tools:        tt.tools,
				SystemPrompt: tt.systemPrompt,
			}

			if newAgent.Priority == 0 {
				newAgent.Priority = 5
			}

			cfg.Agents = append(cfg.Agents, newAgent)

			err = config.SaveConfig(cfg, configFile)
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			reloadedCfg, err := config.LoadConfig(configFile)
			if err != nil {
				t.Fatalf("Failed to reload config: %v", err)
			}

			tt.validateConfig(t, reloadedCfg)
		})
	}
}

func TestUpdateAgentCommand(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	initialConfig := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Agents: []config.Agent{
			{
				Name:         "existing-agent",
				Description:  "Original description",
				Priority:     3,
				Tools:        []string{"old-tool"},
				SystemPrompt: "Original prompt",
			},
		},
	}

	err := config.SaveConfig(initialConfig, configFile)
	if err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	originalDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalDir)
	}()
	_ = os.Chdir(tempDir)

	t.Run("update existing agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == "existing-agent" {
				agentIndex = i
				break
			}
		}

		if agentIndex == -1 {
			t.Fatal("Agent 'existing-agent' not found")
		}

		cfg.Agents[agentIndex].Description = "Updated description"
		cfg.Agents[agentIndex].Priority = 8
		cfg.Agents[agentIndex].Tools = []string{"new-tool1", "new-tool2"}
		cfg.Agents[agentIndex].SystemPrompt = "Updated prompt"

		err = config.SaveConfig(cfg, configFile)
		if err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}

		reloadedCfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to reload config: %v", err)
		}

		if len(reloadedCfg.Agents) != 1 {
			t.Errorf("Expected 1 agent, got %d", len(reloadedCfg.Agents))
		}

		agent := reloadedCfg.Agents[0]
		if agent.Description != "Updated description" {
			t.Errorf("Expected description 'Updated description', got '%s'", agent.Description)
		}
		if agent.Priority != 8 {
			t.Errorf("Expected priority 8, got %d", agent.Priority)
		}
		if len(agent.Tools) != 2 {
			t.Errorf("Expected 2 tools, got %d", len(agent.Tools))
		}
		if agent.SystemPrompt != "Updated prompt" {
			t.Errorf("Expected system prompt 'Updated prompt', got '%s'", agent.SystemPrompt)
		}
	})

	t.Run("update non-existent agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == "non-existent-agent" {
				agentIndex = i
				break
			}
		}

		if agentIndex != -1 {
			t.Error("Expected agent 'non-existent-agent' not to be found")
		}
	})
}

func TestDeleteAgentCommand(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	initialConfig := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Agents: []config.Agent{
			{
				Name:         "agent-to-delete",
				Description:  "Will be deleted",
				Priority:     3,
				SystemPrompt: "Delete me",
			},
			{
				Name:         "agent-to-keep",
				Description:  "Will be kept",
				Priority:     5,
				SystemPrompt: "Keep me",
			},
		},
	}

	err := config.SaveConfig(initialConfig, configFile)
	if err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	originalDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(originalDir)
	}()
	_ = os.Chdir(tempDir)

	t.Run("delete existing agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == "agent-to-delete" {
				agentIndex = i
				break
			}
		}

		if agentIndex == -1 {
			t.Fatal("Agent 'agent-to-delete' not found")
		}

		cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

		err = config.SaveConfig(cfg, configFile)
		if err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}

		reloadedCfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to reload config: %v", err)
		}

		if len(reloadedCfg.Agents) != 1 {
			t.Errorf("Expected 1 agent after deletion, got %d", len(reloadedCfg.Agents))
		}

		if reloadedCfg.Agents[0].Name != "agent-to-keep" {
			t.Errorf("Expected remaining agent to be 'agent-to-keep', got '%s'", reloadedCfg.Agents[0].Name)
		}
	})

	t.Run("delete non-existent agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		agentIndex := -1
		for i, agent := range cfg.Agents {
			if agent.Name == "non-existent-agent" {
				agentIndex = i
				break
			}
		}

		if agentIndex != -1 {
			t.Error("Expected agent 'non-existent-agent' not to be found")
		}
	})
}
