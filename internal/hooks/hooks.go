package hooks

import "os"

func DetectGitHooks() string {
	hooks := []struct {
		name  string
		files []string
	}{
		{"lefthook", []string{"lefthook.yml", ".lefthook.yml", "lefthook.yaml", ".lefthook.yaml"}},
		{"pre-commit", []string{".pre-commit-config.yaml", "pre-commit-config.yaml"}},
		{"husky", []string{".husky"}},
	}

	for _, hook := range hooks {
		if checkAnyFileExists(hook.files) {
			return hook.name
		}
	}
	return ""
}

func checkAnyFileExists(files []string) bool {
	for _, file := range files {
		if fileExists(file) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
