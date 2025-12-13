package presets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestCustomPresetGenerator_GenerateMarkdown(t *testing.T) {
	preset := config.PresetV3{
		Name: "custom-md",
		Type: config.PresetTypeMarkdown,
		Path: "CUSTOM.md",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "rule1",
				Content: "Rule content",
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "context1",
				Content: "Context content",
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "Test config",
	}

	g := NewCustomPresetGenerator(&preset)
	outputs, err := g.Generate(content, "/test", cfg)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(outputs) != 1 {
		t.Errorf("Generate() got %d outputs, want 1", len(outputs))
	}

	if outputs[0].IsDir {
		t.Error("Expected file output, got directory")
	}

	if !strings.Contains(outputs[0].Content, "# test") {
		t.Error("Expected project name in output")
	}
}

func TestCustomPresetGenerator_GenerateMarkdownWithTemplate(t *testing.T) {
	preset := config.PresetV3{
		Name:     "custom-md",
		Type:     config.PresetTypeMarkdown,
		Path:     "CUSTOM.md",
		Template: "# {{.Name}}\n\n{{range .Rules}}## {{.Name}}\n{{.Content}}\n{{end}}",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "rule1",
				Content: "Rule content",
			},
		},
	}

	cfg := &config.ConfigV3{
		Name: "test-project",
	}

	g := NewCustomPresetGenerator(&preset)
	outputs, err := g.Generate(content, "/test", cfg)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(outputs) != 1 {
		t.Errorf("Generate() got %d outputs, want 1", len(outputs))
	}

	outputContent := outputs[0].Content

	if !strings.Contains(outputContent, "# test-project") {
		t.Error("Expected template to render project name")
	}
	if !strings.Contains(outputContent, "## rule1") {
		t.Error("Expected template to render rule name")
	}
	if !strings.Contains(outputContent, "Rule content") {
		t.Error("Expected template to render rule content")
	}
}

func TestCustomPresetGenerator_GenerateDirectory(t *testing.T) {
	preset := config.PresetV3{
		Name: "custom-dir",
		Type: config.PresetTypeDirectory,
		Path: ".custom-rules",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "rule1",
				Content: "Rule 1 content",
			},
			{
				Name:    "rule2",
				Content: "Rule 2 content",
			},
		},
	}

	cfg := &config.ConfigV3{
		Name: "test",
	}

	g := NewCustomPresetGenerator(&preset)
	outputs, err := g.Generate(content, "/test", cfg)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have 1 directory + 2 rule files
	if len(outputs) < 3 {
		t.Errorf("Generate() got %d outputs, want at least 3", len(outputs))
	}

	// First output should be directory
	if !outputs[0].IsDir {
		t.Error("Expected first output to be directory")
	}

	// Check that rule files are created
	fileCount := 0
	for _, output := range outputs {
		if !output.IsDir {
			fileCount++
			if !strings.HasSuffix(output.Path, ".md") {
				t.Errorf("Expected .md file, got %s", output.Path)
			}
		}
	}

	if fileCount != 2 {
		t.Errorf("Expected 2 rule files, got %d", fileCount)
	}
}

func TestCustomPresetGenerator_GenerateJSON(t *testing.T) {
	preset := config.PresetV3{
		Name: "custom-json",
		Type: config.PresetTypeJSON,
		Path: "custom.json",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "rule1",
				Content: "Rule content",
				Metadata: &config.MetadataV3{
					Priority: "high",
				},
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "Test config",
		Version:     "3.0",
	}

	g := NewCustomPresetGenerator(&preset)
	outputs, err := g.Generate(content, "/test", cfg)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(outputs) != 1 {
		t.Errorf("Generate() got %d outputs, want 1", len(outputs))
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(outputs[0].Content), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Check structure
	if result["Name"] != "test" {
		t.Errorf("Expected Name=test, got %v", result["Name"])
	}

	if result["Version"] != "3.0" {
		t.Errorf("Expected Version=3.0, got %v", result["Version"])
	}

	rules, ok := result["Rules"].([]interface{})
	if !ok {
		t.Error("Expected Rules to be an array")
	} else if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
}

func TestCustomPresetGenerator_GenerateJSONWithTemplate(t *testing.T) {
	preset := config.PresetV3{
		Name:     "custom-json",
		Type:     config.PresetTypeJSON,
		Path:     "custom.json",
		Template: `{"project": "{{.Name}}", "ruleCount": {{len .Rules}}}`,
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1"},
			{Name: "rule2"},
		},
	}

	cfg := &config.ConfigV3{
		Name: "test-project",
	}

	g := NewCustomPresetGenerator(&preset)
	outputs, err := g.Generate(content, "/test", cfg)

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(outputs[0].Content), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if result["project"] != "test-project" {
		t.Errorf("Expected project=test-project, got %v", result["project"])
	}

	ruleCount, ok := result["ruleCount"].(float64)
	if !ok || ruleCount != 2 {
		t.Errorf("Expected ruleCount=2, got %v", result["ruleCount"])
	}
}

func TestCustomPresetGenerator_PrepareTemplateData(t *testing.T) {
	preset := config.PresetV3{
		Name: "test",
		Type: config.PresetTypeMarkdown,
		Path: "test.md",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "rule1",
				Content: "Rule content",
				Metadata: &config.MetadataV3{
					Priority: "high",
					Targets:  []string{"claude"},
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "context1",
				Content: "Context content",
			},
		},
		Skills: []config.ContentFile{
			{
				Name:    "skill1",
				Content: "Skill content",
			},
		},
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Rules: []config.ContentFile{
					{Name: "backend-rule"},
				},
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "Test config",
		Version:     "3.0",
	}

	g := NewCustomPresetGenerator(&preset)
	data := g.prepareTemplateData(content, cfg)

	if data["Name"] != "test" {
		t.Errorf("Expected Name=test, got %v", data["Name"])
	}

	rules, ok := data["Rules"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected Rules to be []map[string]interface{}")
	}

	// Should have root rule + domain rule
	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}

	// Check that metadata is included
	if rules[0]["Priority"] != "high" {
		t.Errorf("Expected Priority=high, got %v", rules[0]["Priority"])
	}

	domains, ok := data["Domains"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected Domains to be map")
	}

	if _, exists := domains["backend"]; !exists {
		t.Error("Expected backend domain in data")
	}
}

func TestCustomPresetGenerator_GetName(t *testing.T) {
	preset := config.PresetV3{
		Name: "my-custom-preset",
		Type: config.PresetTypeMarkdown,
		Path: "test.md",
	}

	g := NewCustomPresetGenerator(&preset)
	if g.GetName() != "my-custom-preset" {
		t.Errorf("GetName() = %v, want %v", g.GetName(), "my-custom-preset")
	}
}
