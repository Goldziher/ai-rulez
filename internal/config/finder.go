package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FindConfigFile searches for .airules.yaml or airules.yaml starting from the current directory
// and traversing up to the root. Returns the path to the first config file found.
func FindConfigFile(startDir string) (string, error) {
	// Config file names to search for
	configNames := []string{".airules.yaml", "airules.yaml"}

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

	return "", errors.New("no airules configuration file found. Create an '.airules.yaml' or 'airules.yaml' file in your project")
}

// FindAllConfigFiles recursively finds all .airules.yaml and airules.yaml files
// starting from the given directory.
func FindAllConfigFiles(rootDir string) ([]string, error) {
	var configs []string
	configNames := map[string]bool{
		".airules.yaml": true,
		"airules.yaml":  true,
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
		return nil, fmt.Errorf("no airules configuration files found in %s", rootDir)
	}

	return configs, nil
}
