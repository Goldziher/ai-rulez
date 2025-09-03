package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/schema"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// LoadConfigWithLocal loads main config and merges with local override if it exists
func LoadConfigWithLocal(filename string) (*Config, error) {
	mainConfig, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}

	localConfigPath, err := FindLocalConfigFile(filename)
	if err != nil {
		return nil, err
	}

	if localConfigPath == "" {
		// No local config, return main config
		return mainConfig, nil
	}

	localConfig, err := LoadPartialConfig(localConfigPath)
	if err != nil {
		return nil, oops.
			With("local_config_path", localConfigPath).
			Wrapf(err, "load local config")
	}

	return MergeConfigs(mainConfig, localConfig), nil
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, oops.
			With("path", filename).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions for the file\nEnsure the path is correct and accessible", filename)).
			Wrapf(err, "read file")
	}

	if err := schema.ValidateWithSchema(data); err != nil {
		if oopsErr, ok := oops.AsOops(err); ok {
			if validationErrors, ok := oopsErr.Context()["errors"].([]string); ok && len(validationErrors) > 0 {
				return nil, oops.
					With("path", filename).
					With("filename", filename).
					With("errors", validationErrors).
					With("error_count", len(validationErrors)).
					Hint("Check the YAML syntax using a YAML validator\nEnsure all required fields are present (metadata.name, outputs)\nVerify the structure matches the schema at: https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v2.schema.json\nRun 'ai-rulez validate' for detailed validation output").
					Errorf("configuration validation failed: %d errors", len(validationErrors))
			}
		}
		return nil, oops.
			With("path", filename).
			With("filename", filename).
			With("errors", []string{err.Error()}).
			Hint("Check the YAML syntax using a YAML validator\nEnsure all required fields are present (metadata.name, outputs)\nRun 'ai-rulez validate' for detailed validation output").
			Errorf("configuration validation failed: %s", err.Error())
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		parseErr := oops.
			With("path", filename).
			With("filename", filename).
			Hint("Check the YAML syntax - ensure proper indentation\nValidate your YAML at: https://www.yamllint.com/\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse config")

		if strings.Contains(err.Error(), "line") {
			if oopsErr, ok := oops.AsOops(parseErr); ok {
				parseErr = oops.
					With("path", filename).
					With("filename", filename).
					With("parse_error", err.Error()).
					Hint(oopsErr.Hint()).
					Wrapf(err, "parse config")
			}
		}

		return nil, parseErr
	}

	setDefaultPriorities(&config)

	return &config, nil
}

// LoadPartialConfig loads a configuration fragment without full schema validation
// This is used for included files that may contain only partial configuration
func LoadPartialConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, oops.
			With("path", filename).
			Hint(fmt.Sprintf("Check if the file exists: %s\nVerify you have read permissions for the file\nEnsure the path is correct and accessible", filename)).
			Wrapf(err, "read file")
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		parseErr := oops.
			With("path", filename).
			With("filename", filename).
			Hint("Check the YAML syntax - ensure proper indentation\nValidate your YAML at: https://www.yamllint.com/\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse config")

		if strings.Contains(err.Error(), "line") {
			if oopsErr, ok := oops.AsOops(parseErr); ok {
				parseErr = oops.
					With("path", filename).
					With("filename", filename).
					With("parse_error", err.Error()).
					Hint(oopsErr.Hint()).
					Wrapf(err, "parse config")
			}
		}

		return nil, parseErr
	}

	setDefaultPriorities(&config)

	return &config, nil
}

func SaveConfig(config *Config, filename string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return oops.
			With("path", filename).
			Hint("Check if the config structure is valid").
			Wrapf(err, "marshal config")
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("path", dir).
			With("operation", "create directory").
			Hint("Check if you have write permissions for the parent directory").
			Wrapf(err, "create directory")
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return oops.
			With("path", filename).
			Hint(fmt.Sprintf("Check if you have write permissions for: %s\nEnsure the parent directory exists\nCheck available disk space", filename)).
			Wrapf(err, "write file")
	}

	return nil
}
