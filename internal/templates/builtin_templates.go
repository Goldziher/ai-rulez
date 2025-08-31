package templates

// Built-in template definitions
const (
	defaultTemplate = `# {{.ProjectName}}
{{- if .Description}}

{{.Description}}
{{- end}}
{{- if .Version}}

Version: {{.Version}}
{{- end}}

Generated on {{.Timestamp.Format "2006-01-02 15:04:05"}}
{{- if or .RuleCount .SectionCount}}

Total content: {{.RuleCount}} rules, {{.SectionCount}} sections
{{- end}}
{{- range .AllContent}}
{{- if .IsRule}}

## {{.Title}}

**Priority:** {{.Priority}}

{{.Content}}
{{- else}}

## {{.Title}}

{{.Content}}
{{- end}}
{{- end}}
`

	minimalTemplate = `{{range .Rules}}{{.Content}}

{{end}}`

	documentationTemplate = `# {{.ProjectName}} Documentation
{{- if .Description}}

{{.Description}}
{{- end}}

Generated on {{.Timestamp.Format "2006-01-02 15:04:05"}}

{{- range .Sections}}
## {{.Title}}

{{.Content}}

{{- end}}
{{- range .Rules}}
## {{.Name}}

**Priority:** {{.Priority}}

{{.Content}}

{{- end}}
`
)

// getBuiltinTemplates returns all built-in templates
func getBuiltinTemplates() map[string]string {
	return map[string]string{
		"default":       defaultTemplate,
		"minimal":       minimalTemplate,
		"documentation": documentationTemplate,
	}
}
