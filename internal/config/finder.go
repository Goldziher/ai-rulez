package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FindConfigFile searches for config files starting from the current directory
// and traversing up to the root. Returns the path to the first config file found.
// Supports: airules.yaml, .airules.yaml, ai_rules.yaml, .ai_rules.yaml (and .yml variants)
func FindConfigFile(startDir string) (string, error) {
	// Config file names to search for (in priority order)
	configNames := []string{
		".airules.yaml", ".airules.yml",
		"airules.yaml", "airules.yml",
		".ai_rules.yaml", ".ai_rules.yml",
		"ai_rules.yaml", "ai_rules.yml",
	}

	// Start from the given directory
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Keep track of visited directories to avoid infinite loops
	visited := make(map[string]bool)

	for !visited[dir] {
		visited[dir] = true

		// Check for each config file name
		for _, name := range configNames {
			configPath := filepath.Join(dir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath, nil
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the root
			break
		}
		dir = parent
	}

	return "", errors.New("no configuration file found. Create an 'airules.yaml', '.airules.yaml', 'ai_rules.yaml', or '.ai_rules.yaml' file in your project")
}

// FindAllConfigFiles recursively finds all config files
// starting from the given directory.
// Supports: airules.yaml, .airules.yaml, ai_rules.yaml, .ai_rules.yaml (and .yml variants)
func FindAllConfigFiles(rootDir string) ([]string, error) {
	var configs []string
	configNames := map[string]bool{
		".airules.yaml": true, ".airules.yml": true,
		"airules.yaml": true, "airules.yml": true,
		".ai_rules.yaml": true, ".ai_rules.yml": true,
		"ai_rules.yaml": true, "ai_rules.yml": true,
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (except .airules.yaml itself)
		if info.IsDir() && filepath.Base(path) != "." && filepath.Base(path)[0] == '.' {
			return filepath.SkipDir
		}

		// Check if this is a config file
		if !info.IsDir() && configNames[filepath.Base(path)] {
			configs = append(configs, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory tree: %w", err)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no configuration files found in %s", rootDir)
	}

	return configs, nil
}
