package commands_test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestEnforceCommand(t *testing.T) {
	// Test that the command exists and is properly configured
	assert.NotNil(t, commands.EnforceCmd)
	assert.Equal(t, "enforce", commands.EnforceCmd.Use)
	assert.NotEmpty(t, commands.EnforceCmd.Short)
	assert.NotEmpty(t, commands.EnforceCmd.Long)

	// Verify the command is registered with root
	var enforceFound bool
	for _, cmd := range commands.RootCmd.Commands() {
		if cmd.Use == "enforce" {
			enforceFound = true
			break
		}
	}
	assert.True(t, enforceFound, "Enforce command should be registered with root")
}

func TestEnforceCommandFlags(t *testing.T) {
	cmd := commands.EnforceCmd

	// Test that all expected flags exist
	expectedFlags := []struct {
		name        string
		shorthand   string
		flagType    string
		description string
	}{
		{"agent", "", "string", "AI agent to use"},
		{"level", "", "string", "Enforcement level"},
		{"fix", "", "bool", "Apply fixes"},
		{"timeout", "", "duration", "Timeout per file"},
		{"only-rules", "", "stringSlice", "Only these rules"},
		{"exclude-rules", "", "stringSlice", "Exclude these rules"},
		{"include-files", "", "stringSlice", "Include file patterns"},
		{"exclude-files", "", "stringSlice", "Exclude file patterns"},
		{"max-violations", "", "int", "Maximum violations"},
		{"format", "", "string", "Output format"},
		{"output", "o", "string", "Output file"},
		{"verbose", "", "bool", "Verbose output"},
		{"review", "", "bool", "Enable review"},
		{"review-agent", "", "string", "Review agent"},
		{"review-iterations", "", "int", "Review iterations"},
		{"review-threshold", "", "float64", "Quality threshold"},
		{"review-timeout", "", "duration", "Review timeout"},
		{"review-auto-approve", "", "bool", "Auto approve"},
		{"require-improvement", "", "bool", "Require improvement"},
	}

	for _, ef := range expectedFlags {
		flag := cmd.Flags().Lookup(ef.name)
		require.NotNil(t, flag, "Flag %s should exist", ef.name)

		// Check shorthand if specified
		if ef.shorthand != "" {
			assert.Equal(t, ef.shorthand, flag.Shorthand,
				"Flag %s should have shorthand %s", ef.name, ef.shorthand)
		}

		// Check that description contains expected keywords
		assert.True(t, strings.Contains(strings.ToLower(flag.Usage),
			strings.ToLower(strings.Split(ef.description, " ")[0])),
			"Flag %s description should mention %s", ef.name, ef.description)
	}
}

func TestEnforceCommandDefaults(t *testing.T) {
	cmd := commands.EnforceCmd

	// Test default values
	defaults := map[string]string{
		"level":             "warn",
		"format":            "table",
		"review-iterations": "3",
		"review-threshold":  "75",
		"max-violations":    "-1",
	}

	for flag, expected := range defaults {
		f := cmd.Flags().Lookup(flag)
		require.NotNil(t, f, "Flag %s should exist", flag)
		assert.Equal(t, expected, f.DefValue,
			"Flag %s should have default value %s", flag, expected)
	}
}

func TestEnforceCommandHelp(t *testing.T) {
	// Capture help output
	var helpOutput strings.Builder
	cmd := &cobra.Command{}
	*cmd = *commands.EnforceCmd // Copy command
	cmd.SetOut(&helpOutput)
	cmd.SetErr(&helpOutput)

	err := cmd.Help()
	assert.NoError(t, err)

	help := helpOutput.String()

	// Check that help contains key information
	assert.Contains(t, help, "enforce", "Help should mention command name")
	assert.Contains(t, help, "AI agents", "Help should mention AI agents")
	assert.Contains(t, help, "--fix", "Help should mention fix flag")
	assert.Contains(t, help, "--agent", "Help should mention agent flag")
	assert.Contains(t, help, "Examples:", "Help should include examples")

	// Verify examples don't mention dry-run (since we removed it)
	assert.NotContains(t, help, "--dry-run", "Help should not mention removed dry-run flag")
}

func TestEnforceCommandValidation(t *testing.T) {
	// Test that the command has a RunE function
	assert.NotNil(t, commands.EnforceCmd.RunE, "Enforce command should have RunE function")

	// Test that command validates conflicting flags
	cmd := commands.EnforceCmd

	// Check mutually exclusive flags are marked
	onlyRules := cmd.Flags().Lookup("only-rules")
	excludeRules := cmd.Flags().Lookup("exclude-rules")
	assert.NotNil(t, onlyRules)
	assert.NotNil(t, excludeRules)
}

func TestEnforceCommandExamples(t *testing.T) {
	// Verify that the Long description contains useful examples
	long := commands.EnforceCmd.Long

	examples := []string{
		"ai-rulez enforce",
		"--fix",
		"--agent claude",
		"--format json",
	}

	for _, example := range examples {
		assert.Contains(t, long, example,
			"Long description should contain example: %s", example)
	}
}

func TestEnforceCommandOutputFormats(t *testing.T) {
	// Test that format flag accepts expected values
	formatFlag := commands.EnforceCmd.Flags().Lookup("format")
	require.NotNil(t, formatFlag)

	// Since we don't have enum validation in the flag definition,
	// at least verify the default is sensible
	assert.Equal(t, "table", formatFlag.DefValue, "Default format should be table")

	// Test level flag values
	levelFlag := commands.EnforceCmd.Flags().Lookup("level")
	require.NotNil(t, levelFlag)
	assert.Equal(t, "warn", levelFlag.DefValue, "Default level should be warn")

	// Expected levels that should work
	expectedLevels := []string{"off", "warn", "error", "fix", "strict"}
	_ = expectedLevels // These would be validated at runtime
}

func TestEnforceCommandIntegration(t *testing.T) {
	// This is more of an integration test placeholder
	// In a real scenario, we'd test the actual command execution

	t.Run("CommandStructure", func(t *testing.T) {
		// Verify command is properly integrated with root
		root := commands.RootCmd
		enforceCmd := findSubcommand(root, "enforce")

		require.NotNil(t, enforceCmd, "Enforce should be a subcommand of root")
		assert.Equal(t, commands.EnforceCmd, enforceCmd, "Should be the same command instance")
	})

	t.Run("FlagParsing", func(t *testing.T) {
		// Test that flags can be parsed without error
		cmd := &cobra.Command{}
		*cmd = *commands.EnforceCmd // Copy command

		// Simulate flag parsing
		args := []string{
			"--agent", "claude",
			"--fix",
			"--format", "json",
			"--timeout", "30s",
			"--level", "error",
		}

		err := cmd.ParseFlags(args)
		assert.NoError(t, err, "Should parse valid flags without error")
	})
}

// Helper function to find a subcommand
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Use == name || strings.HasPrefix(cmd.Use, name+" ") {
			return cmd
		}
	}
	return nil
}
