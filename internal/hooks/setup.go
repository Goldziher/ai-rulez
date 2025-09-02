package hooks

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	lefthookSystem  = "lefthook"
	preCommitSystem = "pre-commit"
	huskySystem     = "husky"
)

// SetupHooks sets up git hooks for the detected hook system
func SetupHooks() error {
	hookSystem := DetectGitHooks()
	switch hookSystem {
	case lefthookSystem:
		return setupLefthook()
	case preCommitSystem:
		return setupPreCommit()
	case huskySystem:
		return setupHusky()
	default:
		return fmt.Errorf("no git hook system detected (lefthook, pre-commit, or husky)")
	}
}

func setupLefthook() error {
	configFile := ""
	files := []string{"lefthook.yaml", "lefthook.yml", ".lefthook.yml", ".lefthook.yaml"}
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			configFile = file
			break
		}
	}

	if configFile == "" {
		return fmt.Errorf("lefthook configuration file not found")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read lefthook config: %w", err)
	}

	var configData map[string]interface{}
	if err := yaml.Unmarshal(data, &configData); err != nil {
		return fmt.Errorf("failed to parse lefthook config: %w", err)
	}

	if configData["pre-commit"] == nil {
		configData["pre-commit"] = map[string]interface{}{
			"commands": map[string]interface{}{},
		}
	}

	preCommit, ok := configData["pre-commit"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid lefthook configuration structure")
	}

	if preCommit["commands"] == nil {
		preCommit["commands"] = map[string]interface{}{}
	}

	commands, ok := preCommit["commands"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid lefthook commands structure")
	}

	if _, exists := commands["ai-rulez"]; exists {
		return nil
	}

	commands["ai-rulez"] = map[string]interface{}{
		"glob":      "**/*.{ai-rulez,ai_rulez}.{yaml,yml}",
		"run":       "ai-rulez validate",
		"fail_text": "AI rules validation failed",
	}

	updatedData, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal lefthook config: %w", err)
	}

	if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write lefthook config: %w", err)
	}

	return nil
}

func setupPreCommit() error {
	configFile := ".pre-commit-config.yaml"

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read pre-commit config: %w", err)
	}

	var config struct {
		Repos []map[string]interface{} `yaml:"repos"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse pre-commit config: %w", err)
	}

	for _, repo := range config.Repos {
		if repo["repo"] == "local" {
			if hooks, ok := repo["hooks"].([]interface{}); ok {
				for _, hook := range hooks {
					if h, ok := hook.(map[string]interface{}); ok {
						if h["id"] == "ai-rulez" {
							return nil
						}
					}
				}
			}
		}
	}

	// Find or create local repo
	var localRepo map[string]interface{}
	for _, repo := range config.Repos {
		if repo["repo"] == "local" {
			localRepo = repo
			break
		}
	}

	if localRepo == nil {
		localRepo = map[string]interface{}{
			"repo":  "local",
			"hooks": []interface{}{},
		}
		config.Repos = append(config.Repos, localRepo)
	}

	hooks, ok := localRepo["hooks"].([]interface{})
	if !ok {
		hooks = []interface{}{}
		localRepo["hooks"] = hooks
	}
	aiRulezHook := map[string]interface{}{
		"id":             "ai-rulez",
		"name":           "Validate AI rules",
		"entry":          "ai-rulez validate",
		"language":       "system",
		"files":          `\.(ai-rulez|ai_rulez)\.(yaml|yml)$`,
		"pass_filenames": false,
	}
	localRepo["hooks"] = append(hooks, aiRulezHook)

	updatedData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal pre-commit config: %w", err)
	}

	if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write pre-commit config: %w", err)
	}

	return nil
}

func setupHusky() error {
	if _, err := os.Stat(".husky"); os.IsNotExist(err) {
		return fmt.Errorf(".husky directory not found")
	}

	preCommitPath := ".husky/pre-commit"
	var hookContent string

	if data, err := os.ReadFile(preCommitPath); err == nil {
		hookContent = string(data)

		if strings.Contains(hookContent, "ai-rulez") {
			return nil
		}

		if !strings.HasSuffix(hookContent, "\n") {
			hookContent += "\n"
		}
		hookContent += "\n# Validate AI rules configuration\n"
		hookContent += "echo 'Validating AI rules...'\n"
		hookContent += "npx ai-rulez validate || exit 1\n"
	} else {
		hookContent = `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

# Validate AI rules configuration
echo 'Validating AI rules...'
npx ai-rulez validate || exit 1
`
	}

	if err := os.WriteFile(preCommitPath, []byte(hookContent), 0o755); err != nil {
		return fmt.Errorf("failed to write husky pre-commit hook: %w", err)
	}

	return nil
}

// GetHookSystemName returns a user-friendly name for the detected hook system
func GetHookSystemName(system string) string {
	switch system {
	case "lefthook":
		return "Lefthook"
	case "pre-commit":
		return "Pre-commit"
	case "husky":
		return "Husky"
	default:
		return "Unknown"
	}
}
