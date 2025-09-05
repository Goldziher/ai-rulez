package config

import (
	"fmt"
)

type TemplateType string

const (
	TemplateBuiltin TemplateType = "builtin"
	TemplateFile    TemplateType = "file"
	TemplateInline  TemplateType = "inline"
)

type Template struct {
	Type  TemplateType `yaml:"type"`
	Value string       `yaml:"value"`
}

type TemplateConfig interface{}

func ParseTemplate(templateConfig TemplateConfig) (*Template, error) {
	if templateConfig == nil {
		return nil, nil
	}

	switch t := templateConfig.(type) {
	case map[string]interface{}:
		return parseObjectTemplate(t)
	case Template:
		return &t, nil
	default:
		return nil, fmt.Errorf("unsupported template format: %T - templates must be objects with 'type' and 'value' fields", templateConfig)
	}
}

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

func (t *Template) String() string {
	if t == nil {
		return ""
	}
	return t.Value
}

func (t *Template) IsBuiltin() bool {
	return t != nil && t.Type == TemplateBuiltin
}

func (t *Template) IsFile() bool {
	return t != nil && t.Type == TemplateFile
}

func (t *Template) IsInline() bool {
	return t != nil && t.Type == TemplateInline
}
