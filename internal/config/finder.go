package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func FindConfigFile(startDir string) (string, error) {
	return FindConfigFileInDirName(startDir, aiRulezDirName)
}

func FindConfigFileInDirName(startDir, configDirName string) (string, error) {
	if configDirName == "" {
		configDirName = aiRulezDirName
	}
	configNames := []string{
		// V4 directory-based config (TOML preferred)
		filepath.Join(configDirName, "config.toml"),
		// Directory-based config
		filepath.Join(configDirName, "config.yaml"), filepath.Join(configDirName, "config.yml"),
		// Directory-based config (JSON)
		filepath.Join(configDirName, "config.json"),
		// V2 flat file configs
		".ai-rulez.yaml", ".ai-rulez.yml",
		configFilenameYAMLV2, configFilenameYMLV2,
		".ai_rulez.yaml", ".ai_rulez.yml",
		"ai_rulez.yaml", "ai_rulez.yml",
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", oops.
			With("path", startDir).
			With("operation", "resolve absolute path").
			Hint("Check if the directory exists and is accessible").
			Wrapf(err, "resolve absolute path")
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

	return "", oops.
		With("search_dir", startDir).
		With("supported_names", []string{
			filepath.Join(configDirName, "config.toml"),
			filepath.Join(configDirName, "config.yaml"), filepath.Join(configDirName, "config.yml"),
			filepath.Join(configDirName, "config.json"),
			configFilenameYAMLV2, configFilenameYMLV2,
			".ai-rulez.yaml", ".ai-rulez.yml",
			"ai_rulez.yaml", "ai_rulez.yml",
			".ai_rulez.yaml", ".ai_rulez.yml",
		}).
		Hint(fmt.Sprintf("Run 'ai-rulez init' to create a new configuration file\nCreate one of the supported config files: ai-rulez.yaml, .ai-rulez.yaml, or %s/config.yaml\nCheck if you're in the correct directory\nUse --config flag to specify the config file path explicitly", configDirName)).
		Errorf("no configuration file found")
}

func FindLocalConfigFile(mainConfigPath string) (string, error) {
	dir := filepath.Dir(mainConfigPath)
	base := filepath.Base(mainConfigPath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	localConfigName := nameWithoutExt + ".local" + ext
	localConfigPath := filepath.Join(dir, localConfigName)

	if _, err := os.Stat(localConfigPath); err == nil {
		return localConfigPath, nil
	}

	return "", nil
}
