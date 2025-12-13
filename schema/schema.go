package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaptinlin/jsonschema"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

//go:embed ai-rules-v2.schema.json
var schemaV2JSON []byte

//go:embed ai-rules-v3.schema.json
var schemaV3JSON []byte

var compiler = jsonschema.NewCompiler()

const propertiesField = "properties"

// ValidateWithSchema validates configuration data against the V2 schema (default)
func ValidateWithSchema(configData []byte) error {
	return validateWithSchemaBytes(configData, schemaV2JSON, "v2")
}

// ValidateWithSchemaV3 validates configuration data against the V3 schema
func ValidateWithSchemaV3(configData []byte) error {
	return validateWithSchemaBytes(configData, schemaV3JSON, "v3")
}

func validateWithSchemaBytes(configData []byte, schemaBytes []byte, version string) error {
	var yamlData any
	if err := yaml.Unmarshal(configData, &yamlData); err != nil {
		return oops.
			Hint("Check the YAML syntax - ensure proper indentation\nCommon issues: tabs instead of spaces, missing colons").
			Wrapf(err, "parse YAML for validation")
	}

	jsonData, err := json.Marshal(convertYAMLToJSON(yamlData))
	if err != nil {
		return oops.
			Hint("This is an internal error - the YAML structure may be invalid").
			Wrapf(err, "convert YAML to JSON")
	}

	schema, err := compiler.Compile(schemaBytes)
	if err != nil {
		return oops.
			Hint("This is an internal schema compilation error").
			Wrapf(err, "compile schema")
	}

	result := schema.Validate(jsonData)
	if !result.IsValid() {
		validationErrors := extractDetailedErrors(result)
		hint := "Check the YAML syntax using a YAML validator\nEnsure all required fields are present\nRun 'ai-rulez validate' for detailed validation output"
		switch version {
		case "v2":
			hint = "Check the YAML syntax using a YAML validator\nEnsure all required fields are present (metadata.name, outputs)\nVerify the structure matches the schema\nRun 'ai-rulez validate' for detailed validation output"
		case "v3":
			hint = "Check the YAML/JSON syntax using a validator\nEnsure required fields are present (version: \"3.0\", name)\nVerify the structure matches the V3 schema\nRun 'ai-rulez validate' for detailed validation output"
		}
		return oops.
			With("errors", validationErrors).
			With("error_count", len(validationErrors)).
			With("version", version).
			Hint(hint).
			Errorf("configuration validation failed: %d errors", len(validationErrors))
	}

	return nil
}

func extractDetailedErrors(result *jsonschema.EvaluationResult) []string {
	var errors []string

	for field, validationErr := range result.Errors {
		msg := formatError(field, validationErr, "")
		errors = append(errors, msg)
	}

	errors = append(errors, extractNestedErrors(result.ToList(), "")...)

	return deduplicateErrors(errors)
}

func formatError(field string, err *jsonschema.EvaluationError, path string) string {
	if err.Params != nil {
		if property, ok := err.Params["property"]; ok {
			propertyName := fmt.Sprintf("%v", property)

			switch err.Code {
			case "missing_required_property":
				if path != "" {
					return fmt.Sprintf("- %s.%s: required field is missing", path, propertyName)
				}
				return fmt.Sprintf("- %s: required field is missing", propertyName)
			case "property_mismatch":
				return ""
			}
		}
	}

	switch err.Code {
	case "minItems":
		return fmt.Sprintf("- %s: must have at least 1 item", field)
	case "additionalProperties":
		if err.Params != nil {
			if property, ok := err.Params["property"]; ok {
				return fmt.Sprintf("- %s: additional property '%v' is not allowed", field, property)
			}
		}
		return fmt.Sprintf("- %s: additional properties are not allowed", field)
	case "type_mismatch":
		if err.Params != nil {
			if expected, ok := err.Params["expected"]; ok {
				if actual, ok := err.Params["actual"]; ok {
					return fmt.Sprintf("- %s: expected %v but got %v", field, expected, actual)
				}
			}
		}
	}

	return fmt.Sprintf("- %s: %s", field, err.Message)
}

func extractNestedErrors(list *jsonschema.List, parentPath string) []string {
	var errors []string

	if list == nil {
		return errors
	}

	for _, detail := range list.Details {
		path := parentPath
		if detail.InstanceLocation != "" {
			if path != "" {
				path += detail.InstanceLocation
			} else {
				path = strings.TrimPrefix(detail.InstanceLocation, "/")
			}
		}

		for field, errObj := range detail.Errors {
			msg := formatNestedError(field, errObj, path)
			if msg != "" {
				errors = append(errors, msg)
			}
		}

		if len(detail.Details) > 0 {
			nested := &jsonschema.List{Details: detail.Details}
			errors = append(errors, extractNestedErrors(nested, path)...)
		}
	}

	return errors
}

//nolint:gocyclo // Complex validation function with many cases
func formatNestedError(field string, errMsg string, path string) string {
	cleanPath := strings.ReplaceAll(path, "/", ".")
	cleanPath = strings.Trim(cleanPath, ".")

	switch field {
	case "required":
		if strings.Contains(errMsg, "Required property") {
			start := strings.Index(errMsg, "'")
			end := strings.LastIndex(errMsg, "'")
			if start != -1 && end != -1 && end > start {
				property := errMsg[start+1 : end]
				if cleanPath != "" {
					return fmt.Sprintf("- %s.%s: required field is missing", cleanPath, property)
				}
				return fmt.Sprintf("- %s: required field is missing", property)
			}
		}
	case "type":
		if strings.Contains(errMsg, "null but should be") {
			expectedType := strings.TrimPrefix(errMsg, "Value is null but should be ")
			if cleanPath != "" {
				return fmt.Sprintf("- %s: required field is missing (expected %s)", cleanPath, expectedType)
			}
		}
	case "minItems":
		if cleanPath != "" {
			return fmt.Sprintf("- %s: must have at least 1 item", cleanPath)
		}
	case "additionalProperties":
		if strings.Contains(errMsg, "Additional propert") {
			var properties []string
			inQuote := false
			currentProp := ""
			for _, ch := range errMsg {
				if ch == '\'' {
					if inQuote && currentProp != "" {
						properties = append(properties, currentProp)
						currentProp = ""
					}
					inQuote = !inQuote
				} else if inQuote {
					currentProp += string(ch)
				}
			}

			if len(properties) > 0 {
				propList := strings.Join(properties, "', '")
				if cleanPath != "" {
					return fmt.Sprintf("- %s.additionalProperties: Additional properties '%s' do not match the schema", cleanPath, propList)
				}
				return fmt.Sprintf("- additionalProperties: Additional properties '%s' do not match the schema", propList)
			}
		}
	case propertiesField:
		return ""
	case "schema":
		if strings.Contains(errMsg, "No values are allowed") {
			return ""
		}
	}

	if field == propertiesField && strings.Contains(errMsg, "does not match the schema") {
		return ""
	}

	if cleanPath != "" && field != propertiesField {
		return fmt.Sprintf("- %s.%s: %s", cleanPath, field, errMsg)
	}
	return ""
}

func deduplicateErrors(errors []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, err := range errors {
		if err != "" && !seen[err] {
			seen[err] = true
			result = append(result, err)
		}
	}

	return result
}

func convertYAMLToJSON(i any) any {
	switch x := i.(type) {
	case map[any]any:
		m2 := map[string]any{}
		for k, v := range x {
			m2[fmt.Sprint(k)] = convertYAMLToJSON(v)
		}
		return m2
	case []any:
		for i, v := range x {
			x[i] = convertYAMLToJSON(v)
		}
	}
	return i
}
