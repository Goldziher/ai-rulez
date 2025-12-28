package progress

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptYesNoNonInteractive(t *testing.T) {
	// Test non-interactive mode (CI environment)
	oldCI := os.Getenv("CI")
	defer func() {
		if oldCI != "" {
			os.Setenv("CI", oldCI)
		} else {
			os.Unsetenv("CI")
		}
	}()

	os.Setenv("CI", "true")

	// Should return default value without prompting
	result := PromptYesNo("Test prompt", true)
	assert.True(t, result, "Should return default value (true) in non-interactive mode")

	result = PromptYesNo("Test prompt", false)
	assert.False(t, result, "Should return default value (false) in non-interactive mode")
}

func TestPromptYesNoDetectsCI(t *testing.T) {
	// Test that CI environment variables are properly detected
	oldCI := os.Getenv("CI")
	oldContInt := os.Getenv("CONTINUOUS_INTEGRATION")
	oldBuildID := os.Getenv("BUILD_ID")

	defer func() {
		if oldCI != "" {
			os.Setenv("CI", oldCI)
		} else {
			os.Unsetenv("CI")
		}
		if oldContInt != "" {
			os.Setenv("CONTINUOUS_INTEGRATION", oldContInt)
		} else {
			os.Unsetenv("CONTINUOUS_INTEGRATION")
		}
		if oldBuildID != "" {
			os.Setenv("BUILD_ID", oldBuildID)
		} else {
			os.Unsetenv("BUILD_ID")
		}
	}()

	// Test CONTINUOUS_INTEGRATION variable
	os.Unsetenv("CI")
	os.Setenv("CONTINUOUS_INTEGRATION", "true")
	os.Unsetenv("BUILD_ID")
	result := PromptYesNo("Test", true)
	assert.True(t, result, "Should detect CONTINUOUS_INTEGRATION env var")

	// Test BUILD_ID variable
	os.Unsetenv("CI")
	os.Unsetenv("CONTINUOUS_INTEGRATION")
	os.Setenv("BUILD_ID", "12345")
	result = PromptYesNo("Test", false)
	assert.False(t, result, "Should detect BUILD_ID env var")
}

func TestIsInteractiveDetectsCI(t *testing.T) {
	// Test that isInteractive properly detects CI environments
	oldCI := os.Getenv("CI")
	defer func() {
		if oldCI != "" {
			os.Setenv("CI", oldCI)
		} else {
			os.Unsetenv("CI")
		}
	}()

	os.Setenv("CI", "true")

	// When CI is set, it should not be interactive
	// This is verified indirectly through PromptYesNo behavior
	result := PromptYesNo("Test", true)
	assert.True(t, result)
}
