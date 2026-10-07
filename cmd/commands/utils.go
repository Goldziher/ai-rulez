package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// newContentOperator opens the CRUD operator for the current directory. With
// local set, content operations work on the machine-local tree
// (.ai-rulez/local/), which mirrors the shared layout.
func newContentOperator(local bool) (*crud.OperatorImpl, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return nil, err
	}
	if local {
		op = op.Local()
	}
	return op, nil
}

// confirmRemoval prompts the user to confirm a removal operation
// Returns true if the user confirms, false otherwise
// If resourceType is empty, uses resourceName as the full description
func confirmRemoval(resourceType, resourceName string) bool {
	// Build confirmation message
	var prompt string
	if resourceType == "" {
		prompt = fmt.Sprintf("Are you sure you want to remove %s? (y/N): ", resourceName)
	} else {
		prompt = fmt.Sprintf("Are you sure you want to remove %s '%s'? (y/N): ", resourceType, resourceName)
	}
	return askYesNo(prompt)
}

// exitDeclined reports that a destructive operation was not confirmed and exits
// 1: nothing was done, which a script must be able to tell from success.
func exitDeclined(what string) {
	logger.Error(what+": not confirmed, nothing was changed", "hint", "pass --yes to skip the confirmation prompt (required in non-interactive shells)")
	os.Exit(1)
}

// askYesNo prints prompt and reads a yes/no answer from an interactive terminal;
// a pipe, CI or any other non-interactive input answers no.
func askYesNo(prompt string) bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Non-interactive terminal
		return false
	}

	fmt.Print(prompt)

	var response string
	_, err = fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == answerYes
}

// workingDir is the process working directory, "" when it cannot be read. The
// CLI resolves it once here and passes it to library code that shows paths
// relative to it, so no library package reads the working directory itself.
func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
