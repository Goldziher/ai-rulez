package crud

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// loadConfiguration loads the configuration file
func loadConfiguration() (string, *config.Config) {
	configPath, err := config.FindConfigFile(".")
	if err != nil {
		FmtError(oops.
			With("search_dir", ".").
			Hint("Run 'ai-rulez init' to create a new configuration file\nCreate one of the supported config files: ai-rulez.yaml, .ai-rulez.yaml").
			Errorf("no configuration file found"))
		os.Exit(1)
	}

	cfg, err := config.LoadConfigWithIncludes(context.Background(), configPath)
	if err != nil {
		FmtError(err)
		os.Exit(1)
	}

	return configPath, cfg
}

// saveConfiguration saves the configuration to a file
func saveConfiguration(cfg *config.Config, configPath string) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		FmtError(oops.Wrapf(err, "marshal configuration"))
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		FmtError(oops.
			With("path", configPath).
			Hint(fmt.Sprintf("Check if you have write permissions for: %s", configPath)).
			Wrapf(err, "write file"))
		os.Exit(1)
	}
}

// FmtError formats an error for display
func FmtError(err error) {
	if err == nil {
		return
	}

	// Simple error formatting for oops errors
	fmt.Fprintf(os.Stderr, "\033[31mError:\033[0m %v\n", err)

	// Check if it's an oops error to show hints
	if oopsErr, ok := oops.AsOops(err); ok {
		ctx := oopsErr.Context()

		// Show validation errors if present
		if validationErrors, ok := ctx["errors"].([]string); ok && len(validationErrors) > 0 {
			fmt.Fprintf(os.Stderr, "\n\033[31mValidation Errors:\033[0m\n")
			for _, errMsg := range validationErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", errMsg)
			}
		}

		// Show hint if available
		if hint := oopsErr.Hint(); hint != "" {
			fmt.Fprintf(os.Stderr, "\n\033[36mHint:\033[0m %s\n", hint)
		}
	}
}

// readFromStdin reads content from stdin
func readFromStdin() (string, error) {
	var content []byte
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		content = append(content, scanner.Bytes()...)
		content = append(content, '\n')
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return string(content), nil
}
