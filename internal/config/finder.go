package config

import (
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/errors"
)

func FindConfigFile(startDir string) (string, error) {
	configNames := []string{
		".ai-rulez.yaml", ".ai-rulez.yml",
		"ai-rulez.yaml", "ai-rulez.yml",
		".ai_rulez.yaml", ".ai_rulez.yml",
		"ai_rulez.yaml", "ai_rulez.yml",
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", errors.FileRead(startDir, err).
			WithContext("operation", "resolve absolute path").
			WithSuggestion("Check if the directory exists and is accessible")
	}

	visited := make(map[string]bool)

	for !visited[dir] {
		visited[dir] = true

		for _, name := range configNames {
			configPath := filepath.Join(dir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.ConfigNotFound(startDir)
}

func FindAllConfigFiles(rootDir string) ([]string, error) {
	var configs []string
	configNames := map[string]bool{
		".ai-rulez.yaml": true, ".ai-rulez.yml": true,
		"ai-rulez.yaml": true, "ai-rulez.yml": true,
		".ai_rulez.yaml": true, ".ai_rulez.yml": true,
		"ai_rulez.yaml": true, "ai_rulez.yml": true,
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.FileRead(path, err).
				WithContext("operation", "walk directory")
		}

		if info.IsDir() && filepath.Base(path) != "." && filepath.Base(path)[0] == '.' {
			return filepath.SkipDir
		}

		if !info.IsDir() && configNames[filepath.Base(path)] {
			configs = append(configs, path)
		}

		return nil
	})

	if err != nil {
		return nil, errors.FileRead(rootDir, err).
			WithContext("operation", "walk directory tree").
			WithSuggestion("Check if the directory exists and is accessible")
	}

	if len(configs) == 0 {
		return nil, errors.ConfigNotFound(rootDir).
			WithContext("search_type", "recursive").
			WithSuggestion("Run 'ai-rulez init' in the directory to create a config file")
	}

	return configs, nil
}
