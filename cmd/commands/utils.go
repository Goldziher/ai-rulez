package commands

import (
	"fmt"
	"os"
	"strings"
)

// confirmRemoval prompts the user to confirm a removal operation
// Returns true if the user confirms, false otherwise
// If resourceType is empty, uses resourceName as the full description
func confirmRemoval(resourceType, resourceName string) bool {
	// Check for non-interactive terminal
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Non-interactive terminal
		return false
	}

	// Build confirmation message
	var prompt string
	if resourceType == "" {
		prompt = fmt.Sprintf("Are you sure you want to remove %s? (y/N): ", resourceName)
	} else {
		prompt = fmt.Sprintf("Are you sure you want to remove %s '%s'? (y/N): ", resourceType, resourceName)
	}

	fmt.Print(prompt)

	var response string
	_, err = fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
