package config

import (
	"fmt"
	"strings"
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

func parseStringTemplate(template string) (*Template, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil, nil
	}

	if strings.HasPrefix(template, "@") {
		return &Template{
			Type:  TemplateFile,
			Value: strings.TrimPrefix(template, "@"),
		}, nil
	}

	if strings.Contains(template, "\n") {
		return &Template{
			Type:  TemplateInline,
			Value: template,
		}, nil
	}

	return &Template{
		Type:  TemplateBuiltin,
		Value: template,
	}, nil
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

	switch t.Type {
	case TemplateFile:
		return "@" + t.Value
	case TemplateBuiltin, TemplateInline:
		return t.Value
	default:
		return t.Value
	}
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
