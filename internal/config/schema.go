package config

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// Embed the schema file at compile time
//
//go:embed schema/ai-rules-v1.schema.json
var schemaJSON string

// ValidateWithSchema validates a configuration against the JSON Schema.
func ValidateWithSchema(configData []byte) error {
	// Convert YAML to JSON for schema validation
	var yamlData any
	if err := yaml.Unmarshal(configData, &yamlData); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	jsonData, err := json.Marshal(ConvertYAMLToJSON(yamlData))
	if err != nil {
		return fmt.Errorf("failed to convert to JSON: %w", err)
	}

	// Load schema and document
	schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	// Validate
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if !result.Valid() {
		var errors []string
		for _, desc := range result.Errors() {
			errors = append(errors, fmt.Sprintf("- %s", desc))
		}
		return fmt.Errorf("configuration validation failed:\n%s",
			stringSliceToString(errors, "\n"))
	}

	return nil
}

// ConvertYAMLToJSON converts YAML data to JSON-compatible format.
// This is needed because YAML uses map[any]any while JSON needs map[string]any.
func ConvertYAMLToJSON(i any) any {
	switch x := i.(type) {
	case map[any]any:
		m2 := map[string]any{}
		for k, v := range x {
			m2[fmt.Sprint(k)] = ConvertYAMLToJSON(v)
		}
		return m2
	case []any:
		for i, v := range x {
			x[i] = ConvertYAMLToJSON(v)
		}
	}
	return i
}

// stringSliceToString joins a slice of strings.
func stringSliceToString(slice []string, sep string) string {
	result := ""
	for i, s := range slice {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// ValidateConfigWithSchema validates a Config struct against the schema.
func ValidateConfigWithSchema(cfg *Config) error {
	// Marshal config to YAML first
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return ValidateWithSchema(yamlData)
}
