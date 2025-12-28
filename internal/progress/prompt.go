package progress

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// PromptYesNo displays a yes/no prompt and returns true if user selects yes
// If input is not interactive (e.g., in CI), returns defaultValue
func PromptYesNo(prompt string, defaultValue bool) bool {
	// Check if we're in interactive mode
	if !isInteractive() {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	var defaultStr string
	if defaultValue {
		defaultStr = "Y/n"
	} else {
		defaultStr = "y/N"
	}

	fullPrompt := fmt.Sprintf("%s [%s] ", prompt, defaultStr)
	fmt.Print(fullPrompt)

	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}

	response := strings.ToLower(strings.TrimSpace(input))

	switch response {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	case "":
		// Empty response uses default
		return defaultValue
	default:
		// Invalid response, show error and retry
		fmt.Println("Invalid response. Please enter 'y' or 'n'.")
		return PromptYesNo(prompt, defaultValue)
	}
}

// isInteractive returns true if stdin is a terminal and we're not in CI mode
func isInteractive() bool {
	// Check CI environment variables
	ciEnvs := []string{"CI", "CONTINUOUS_INTEGRATION", "BUILD_ID"}
	for _, env := range ciEnvs {
		if os.Getenv(env) != "" {
			return false
		}
	}

	// Check if stdin is a terminal
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	// If mode has CharDevice bit set, it's a terminal
	return (stat.Mode() & os.ModeCharDevice) != 0
}
