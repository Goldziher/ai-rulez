package integration_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIIntegration(t *testing.T) {
	
	tempDir := t.TempDir()
	
	t.Run("Basic Commands", func(t *testing.T) {
		suite := loadTestSuite(t, "suites/basic_commands.json")
		runTestSuite(t, suite, tempDir)
	})
	
	t.Run("Config Discovery", func(t *testing.T) {
		suite := loadTestSuite(t, "suites/config_discovery.json")
		runTestSuite(t, suite, tempDir)
	})
	
	t.Run("Generation", func(t *testing.T) {
		suite := loadTestSuite(t, "suites/generation.json")
		runTestSuite(t, suite, tempDir)
	})
	
	t.Run("Validation", func(t *testing.T) {
		suite := loadTestSuite(t, "suites/validation.json")
		runTestSuite(t, suite, tempDir)
	})
	
	t.Run("Error Handling", func(t *testing.T) {
		suite := loadTestSuite(t, "suites/error_handling.json")
		runTestSuite(t, suite, tempDir)
	})
}

func copyTestScenarios(t *testing.T, destDir string) error {
	t.Helper()
	
	
	testingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	
	scenarios := []string{
		"scenarios/basic",
		"scenarios/minimal", 
		"scenarios/with-includes",
		"scenarios/nested-includes",
		"scenarios/invalid",
		"includes",
		"templates",
	}
	
	for _, scenario := range scenarios {
		srcPath := filepath.Join(testingDir, scenario)
		destPath := filepath.Join(destDir, scenario)
		
		if err := copyDir(srcPath, destPath); err != nil {
			return err
		}
	}
	
	return nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	
	return os.WriteFile(dst, data, 0644)
}