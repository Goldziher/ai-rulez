package hooks

import "os"

// DetectGitHooks detects which git hook system is in use
func DetectGitHooks() string {
	if detectLefthook() {
		return "lefthook"
	}
	if detectPreCommit() {
		return "pre-commit"
	}
	if detectHusky() {
		return "husky"
	}
	return ""
}

func detectLefthook() bool {
	if _, err := os.Stat("lefthook.yml"); err == nil {
		return true
	}
	if _, err := os.Stat(".lefthook.yml"); err == nil {
		return true
	}
	if _, err := os.Stat("lefthook.yaml"); err == nil {
		return true
	}
	if _, err := os.Stat(".lefthook.yaml"); err == nil {
		return true
	}
	return false
}

func detectPreCommit() bool {
	_, err := os.Stat(".pre-commit-config.yaml")
	return err == nil
}

func detectHusky() bool {
	_, err := os.Stat(".husky")
	return err == nil
}
