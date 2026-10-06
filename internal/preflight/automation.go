package preflight

import (
	"path"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// automationFiles are files a tool or CI system executes. A custom preset may
// write them (they are not control data like .git, so they are not refused), but
// a person who cloned the repository must see that generate will.
var automationFiles = map[string]bool{
	".gitlab-ci.yml": true, ".gitlab-ci.yaml": true, ".pre-commit-config.yaml": true, ".envrc": true,
	"makefile": true, "gnumakefile": true, "justfile": true, "taskfile.yml": true, "taskfile.yaml": true,
	"package.json": true, "jenkinsfile": true, "dockerfile": true, "azure-pipelines.yml": true,
	"bitbucket-pipelines.yml": true, ".travis.yml": true, "setup.py": true, "conftest.py": true,
	".devcontainer.json": true, ".vscode/tasks.json": true, ".vscode/launch.json": true,
	".vscode/settings.json": true,
}

// automationDirs are directories whose files a tool or CI system executes.
var automationDirs = []string{
	".github/workflows/", ".github/actions/", ".circleci/", ".buildkite/", ".husky/", ".githooks/",
	".devcontainer/", ".gitlab/",
}

// IsAutomationPath reports whether a project-relative path names a file that CI
// or a developer tool runs.
func IsAutomationPath(p string) bool {
	clean := strings.ToLower(path.Clean(strings.ReplaceAll(p, "\\", "/")))
	if automationFiles[clean] {
		return true
	}
	for _, dir := range automationDirs {
		if strings.HasPrefix(clean+"/", dir) {
			return true
		}
	}
	return false
}

// automationItems lists the custom presets that write an executable
// configuration file, so the summary announces them before generate does.
func automationItems(cfg *config.Config) []Item {
	var items []Item
	for i := range cfg.Presets {
		preset := &cfg.Presets[i]
		if preset.Path != "" && IsAutomationPath(preset.Path) {
			items = append(items, Item{Kind: "exec-file", Command: "write " + preset.Path, Where: []string{preset.GetName()}})
		}
	}
	return items
}
