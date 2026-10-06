package adapter

import (
	"embed"
)

//go:embed templates/shell.sh templates/research.md templates/repair-workflow.yml
var templateFS embed.FS

// Template returns the text of a bundled template (shell, research, repair-workflow); ok is false for any other name.
func Template(name string) (text string, ok bool) {
	file := map[string]string{Shell: "templates/shell.sh", Research: "templates/research.md", RepairWorkflow: "templates/repair-workflow.yml"}[name]
	if file == "" {
		return "", false
	}
	data, err := templateFS.ReadFile(file)
	if err != nil {
		return "", false
	}
	return string(data), true
}
