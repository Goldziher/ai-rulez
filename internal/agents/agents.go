package agents

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type AgentInfo struct {
	ID      string
	Command string
	Display string
}

type Preset string

var SupportedAgents = []AgentInfo{
	{ID: "amp", Command: "amp", Display: "AMP (Sourcegraph)"},
	{ID: "claude", Command: "claude", Display: "Claude (Anthropic)"},
	{ID: "codex", Command: "codex", Display: "Codex (OpenAI)"},
	{ID: "cursor", Command: "cursor-agent", Display: "Cursor Agent (Cursor)"},
	{ID: "gemini", Command: "gemini", Display: "Gemini (Google)"},
}

var PresetOutputs = map[string][]string{
	"claude": {
		"claude",
	},
	"amp": {
		"amp",
	},
	"codex": {
		"codex",
	},
	"cursor": {
		"cursor",
	},
	"gemini": {
		"gemini",
	},
	"windsurf": {
		"windsurf",
	},
	"copilot": {
		"copilot",
	},
	"cline": {
		"cline",
	},
	"continue": {
		"continue-dev",
	},
}

type PresetOptions struct {
	WithAgents   bool
	WithSections bool
	NoComments   bool
}

var PresetConfigs = map[string]PresetOptions{
	"claude": {
		WithAgents:   true,
		WithSections: false,
		NoComments:   false,
	},
	"amp": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"codex": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"cursor": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"gemini": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"windsurf": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"copilot": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"cline": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
	"continue": {
		WithAgents:   false,
		WithSections: false,
		NoComments:   false,
	},
}

func DetectAvailableAgents() ([]AgentInfo, error) {
	var available []AgentInfo

	for _, agent := range SupportedAgents {
		if isCommandAvailable(agent.Command) {
			available = append(available, agent)
		}
	}

	return available, nil
}

func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func GetAgentByID(id string) (*AgentInfo, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, agent := range SupportedAgents {
		if agent.ID == id {
			return &agent, nil
		}
	}
	return nil, fmt.Errorf("unknown agent: %s", id)
}

func ParseAgentList(agentList string) ([]AgentInfo, error) {
	if agentList == "" {
		return nil, nil
	}

	parts := strings.Split(agentList, ",")
	var agents []AgentInfo
	seen := make(map[string]bool)

	for _, part := range parts {
		id := strings.ToLower(strings.TrimSpace(part))
		if id == "" {
			continue
		}

		if seen[id] {
			continue
		}

		agent, err := GetAgentByID(id)
		if err != nil {
			return nil, err
		}

		agents = append(agents, *agent)
		seen[id] = true
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})

	return agents, nil
}

func InvokeAgent(agent AgentInfo, prompt string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch agent.ID {
	case "claude":
		cmd = exec.CommandContext(ctx, agent.Command, "--no-conversation", "--max-tokens", "4000")
		cmd.Stdin = strings.NewReader(prompt)

	case "amp", "codex", "cursor":
		cmd = exec.CommandContext(ctx, agent.Command, prompt)

	case "gemini":
		cmd = exec.CommandContext(ctx, agent.Command, prompt)

	default:
		cmd = exec.CommandContext(ctx, agent.Command)
		cmd.Stdin = strings.NewReader(prompt)
	}

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent timed out after %v", timeout)
		}
		return "", fmt.Errorf("agent failed: %w", err)
	}

	return string(output), nil
}

func ValidatePreset(preset string) error {
	presetLower := strings.ToLower(preset)

	if _, exists := PresetOutputs[presetLower]; exists {
		return nil
	}

	var validPresets []string
	for k := range PresetOutputs {
		validPresets = append(validPresets, k)
	}
	sort.Strings(validPresets)

	return fmt.Errorf("invalid preset: %s (valid options: %s)", preset, strings.Join(validPresets, ", "))
}

func GetPresetProviders(preset string) []string {
	return PresetOutputs[strings.ToLower(preset)]
}

func GetPresetOptions(preset string) PresetOptions {
	return PresetConfigs[strings.ToLower(preset)]
}
