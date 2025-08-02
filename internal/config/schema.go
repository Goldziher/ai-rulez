package config

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// Embed the schema file at compile time
//
//go:embed ai-rules-v1.schema.json
var schemaJSON string

// ValidateWithSchema validates a configuration against the JSON Schema.
func ValidateWithSchema(configData []byte) error {
	// Convert YAML to JSON for schema validation
	var yamlData any
	if err := yaml.Unmarshal(configData, &yamlData); err != nil {
		return errors.New(errors.ErrorTypeConfigInvalid, "parse YAML for validation", err).
			WithSuggestion("Check the YAML syntax - ensure proper indentation").
			WithSuggestion("Common issues: tabs instead of spaces, missing colons")
	}

	jsonData, err := json.Marshal(ConvertYAMLToJSON(yamlData))
	if err != nil {
		return errors.New(errors.ErrorTypeConfigInvalid, "convert YAML to JSON", err).
			WithSuggestion("This is an internal error - the YAML structure may be invalid")
	}

	// Load schema and document
	schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	// Validate
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return errors.New(errors.ErrorTypeConfigSchema, "schema validation", err).
			WithSuggestion("This is an internal schema validation error")
	}

	if !result.Valid() {
		var validationErrors []string
		for _, desc := range result.Errors() {
			validationErrors = append(validationErrors, desc.String())
		}
		return errors.SchemaValidation("", validationErrors)
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

// ValidateConfigWithSchema validates a Config struct against the schema.
func ValidateConfigWithSchema(cfg *Config) error {
	// Marshal config to YAML first
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.New(errors.ErrorTypeConfig, "marshal config for validation", err).
			WithSuggestion("Check if the config structure is valid")
	}

	return ValidateWithSchema(yamlData)
}
