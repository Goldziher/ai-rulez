package config

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

//go:embed ai-rules-v1.schema.json
var schemaJSON string

func ValidateWithSchema(configData []byte) error {
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

	schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return errors.New(errors.ErrorTypeConfigSchema, "schema validation", err).
			WithSuggestion("This is an internal schema validation error")
	}

	if !result.Valid() {
		var validationErrors []string
		for _, desc := range result.Errors() {
			errMsg := fmt.Sprintf("- %s: %s", desc.Field(), desc.Description())
			if desc.Value() != nil {
				errMsg = fmt.Sprintf("%s (got: %v)", errMsg, desc.Value())
			}
			validationErrors = append(validationErrors, errMsg)
		}
		return errors.SchemaValidation("", validationErrors)
	}

	return nil
}

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

func ValidateConfigWithSchema(cfg *Config) error {
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.New(errors.ErrorTypeConfig, "marshal config for validation", err).
			WithSuggestion("Check if the config structure is valid")
	}

	return ValidateWithSchema(yamlData)
}
