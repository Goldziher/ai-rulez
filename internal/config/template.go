package config

import (
	"fmt"
	"strings"
)

// TemplateType represents the type of template
type TemplateType string

const (
	TemplateBuiltin TemplateType = "builtin"
	TemplateFile    TemplateType = "file"
	TemplateInline  TemplateType = "inline"
)

// Template represents a template configuration
type Template struct {
	Type  TemplateType `yaml:"type"`
	Value string       `yaml:"value"`
}

// TemplateConfig can be either a string (legacy) or structured Template object
type TemplateConfig interface{}

// ParseTemplate converts various template formats to a unified Template struct
func ParseTemplate(templateConfig TemplateConfig) (*Template, error) {
	if templateConfig == nil {
		return nil, nil
	}

	switch t := templateConfig.(type) {
	case string:
		return parseStringTemplate(t)
	case map[string]interface{}:
		return parseObjectTemplate(t)
	case Template:
		return &t, nil
	default:
		return nil, fmt.Errorf("unsupported template format: %T", templateConfig)
	}
}

// parseStringTemplate handles legacy string format
func parseStringTemplate(template string) (*Template, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil, nil
	}

	// File reference (@file)
	if strings.HasPrefix(template, "@") {
		return &Template{
			Type:  TemplateFile,
			Value: strings.TrimPrefix(template, "@"),
		}, nil
	}

	// Inline template (contains newlines)
	if strings.Contains(template, "\n") {
		return &Template{
			Type:  TemplateInline,
			Value: template,
		}, nil
	}

	// Built-in template name
	return &Template{
		Type:  TemplateBuiltin,
		Value: template,
	}, nil
}

// parseObjectTemplate handles new structured format
func parseObjectTemplate(obj map[string]interface{}) (*Template, error) {
	typeStr, hasType := obj["type"].(string)
	value, hasValue := obj["value"].(string)

	if !hasType || !hasValue {
		return nil, fmt.Errorf("template object must have 'type' and 'value' fields")
	}

	var templateType TemplateType
	switch typeStr {
	case "builtin":
		templateType = TemplateBuiltin
	case "file":
		templateType = TemplateFile
	case "inline":
		templateType = TemplateInline
	default:
		return nil, fmt.Errorf("invalid template type: %s", typeStr)
	}

	return &Template{
		Type:  templateType,
		Value: value,
	}, nil
}

// String returns the string representation for backwards compatibility
func (t *Template) String() string {
	if t == nil {
		return ""
	}

	switch t.Type {
	case TemplateFile:
		return "@" + t.Value
	case TemplateBuiltin, TemplateInline:
		return t.Value
	default:
		return t.Value
	}
}

// IsBuiltin returns true if this is a built-in template
func (t *Template) IsBuiltin() bool {
	return t != nil && t.Type == TemplateBuiltin
}

// IsFile returns true if this is a file template
func (t *Template) IsFile() bool {
	return t != nil && t.Type == TemplateFile
}

// IsInline returns true if this is an inline template
func (t *Template) IsInline() bool {
	return t != nil && t.Type == TemplateInline
}
