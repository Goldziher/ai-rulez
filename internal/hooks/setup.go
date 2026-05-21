package hooks

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	lefthookSystem        = "lefthook"
	preCommitSystem       = "pre-commit"
	huskySystem           = "husky"
	officialPreCommitRepo = "https://github.com/Goldziher/ai-rulez"
	officialPreCommitRev  = "v4.2.1"
	keyRepo               = "repo"
	keyHooks              = "hooks"
	binaryAIRulez         = "ai-rulez"
	unknownLabel          = "Unknown"
)

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

	if _, exists := commands[binaryAIRulez]; exists {
		return nil
	}

	commands[binaryAIRulez] = map[string]interface{}{
		"glob":      ".ai-rulez/**",
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
	configFile, err := findPreCommitConfig()
	if err != nil {
		return err
	}

	config, err := readPreCommitConfig(configFile)
	if err != nil {
		return err
	}

	repos := extractRepoList(config)

	repos, officialRepo := ensureOfficialPreCommitRepo(repos)
	ensureOfficialHooks(officialRepo)
	repos = pruneLegacyLocalHooks(repos)

	config["repos"] = repos

	return writePreCommitConfig(configFile, config)
}

func findPreCommitConfig() (string, error) {
	candidates := []string{".pre-commit-config.yaml", "pre-commit-config.yaml"}
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}
	return "", fmt.Errorf("pre-commit configuration file not found")
}

func readPreCommitConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pre-commit config: %w", err)
	}

	config := map[string]interface{}{}
	if len(data) == 0 {
		return config, nil
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse pre-commit config: %w", err)
	}
	return config, nil
}

func extractRepoList(config map[string]interface{}) []interface{} {
	repos, ok := config["repos"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return repos
}

func ensureOfficialPreCommitRepo(repos []interface{}) (updated []interface{}, repo map[string]interface{}) {
	updated = repos
	for i, candidate := range updated {
		repoMap, ok := candidate.(map[string]interface{})
		if !ok {
			continue
		}
		if repoMap[keyRepo] == officialPreCommitRepo {
			repoMap["rev"] = officialPreCommitRev
			if _, ok := repoMap[keyHooks]; !ok {
				repoMap[keyHooks] = []interface{}{}
			}
			updated[i] = repoMap
			return updated, repoMap
		}
	}

	repo = map[string]interface{}{
		keyRepo:  officialPreCommitRepo,
		"rev":    officialPreCommitRev,
		keyHooks: []interface{}{},
	}
	updated = append(updated, repo)
	return updated, repo
}

func ensureOfficialHooks(repo map[string]interface{}) {
	hooks, ok := repo[keyHooks].([]interface{})
	if !ok {
		hooks = []interface{}{}
	}

	hookIDs := make(map[string]struct{}, len(hooks))
	for _, hook := range hooks {
		hookMap, ok := hook.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := hookMap["id"].(string); ok {
			hookIDs[id] = struct{}{}
		}
	}

	for _, id := range []string{"ai-rulez-validate", "ai-rulez-generate"} {
		if _, exists := hookIDs[id]; exists {
			continue
		}
		hooks = append(hooks, map[string]interface{}{"id": id})
	}

	repo[keyHooks] = hooks
}

func pruneLegacyLocalHooks(repos []interface{}) []interface{} {
	for i, repo := range repos {
		repoMap, ok := repo.(map[string]interface{})
		if !ok {
			continue
		}
		if repoMap[keyRepo] != "local" {
			continue
		}
		hooks, ok := repoMap[keyHooks].([]interface{})
		if !ok || len(hooks) == 0 {
			continue
		}
		filtered := make([]interface{}, 0, len(hooks))
		for _, hook := range hooks {
			hookMap, ok := hook.(map[string]interface{})
			if !ok {
				filtered = append(filtered, hook)
				continue
			}
			id, ok := hookMap["id"].(string)
			if !ok {
				filtered = append(filtered, hook)
				continue
			}
			if id == binaryAIRulez {
				continue
			}
			filtered = append(filtered, hook)
		}
		repoMap[keyHooks] = filtered
		repos[i] = repoMap //nolint:gosec // i is bounded by range over repos
	}
	return repos
}

func writePreCommitConfig(path string, config map[string]interface{}) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal pre-commit config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write pre-commit config: %w", err)
	}

	return nil
}

func setupHusky() error {
	if _, err := os.Stat(".husky"); os.IsNotExist(err) {
		return fmt.Errorf(".husky directory not found")
	}

	var hookContent string

	if data, err := os.ReadFile(".husky/pre-commit"); err == nil {
		hookContent = string(data)

		if strings.Contains(hookContent, binaryAIRulez) {
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

	huskyRoot, err := os.OpenRoot(".husky")
	if err != nil {
		return fmt.Errorf("failed to open .husky directory: %w", err)
	}
	defer func() { _ = huskyRoot.Close() }()

	if err := huskyRoot.WriteFile("pre-commit", []byte(hookContent), 0o755); err != nil {
		return fmt.Errorf("failed to write husky pre-commit hook: %w", err)
	}

	return nil
}

func GetHookSystemName(system string) string {
	switch system {
	case lefthookSystem:
		return "Lefthook"
	case preCommitSystem:
		return "Pre-commit"
	case huskySystem:
		return "Husky"
	default:
		return unknownLabel
	}
}
