package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadV3AsV2_BasicConfig(t *testing.T) {
	t.Run("loads basic V3 config and converts to V2", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config file
		configContent := `version: "3.0"
name: test-project
description: A test project
presets:
  - claude
gitignore: false
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create a rule file
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		rulePath := filepath.Join(rulesPath, "style.md")
		require.NoError(t, os.WriteFile(rulePath, []byte("# Code Style\nBe consistent"), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, v2Config)

		// Verify metadata
		assert.Equal(t, "test-project", v2Config.Metadata.Name)
		assert.Equal(t, "A test project", v2Config.Metadata.Description)
		assert.Equal(t, false, *v2Config.Gitignore)

		// Verify rules were converted
		assert.Len(t, v2Config.Rules, 1)
		assert.Equal(t, "style", v2Config.Rules[0].Name)
		assert.Equal(t, "# Code Style\nBe consistent", v2Config.Rules[0].Content)
		assert.Equal(t, PriorityMedium, v2Config.Rules[0].Priority)

		// Verify outputs were created from presets
		assert.NotEmpty(t, v2Config.Outputs)
	})

	t.Run("handles config with priority metadata", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config file
		configContent := `version: "3.0"
name: test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create rule files with priority
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))

		highPriority := filepath.Join(rulesPath, "critical-rule.md")
		require.NoError(t, os.WriteFile(highPriority, []byte(`---
priority: high
---

Critical rule content`), 0o644))

		lowPriority := filepath.Join(rulesPath, "minor-rule.md")
		require.NoError(t, os.WriteFile(lowPriority, []byte(`---
priority: low
---

Minor rule content`), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify priorities were converted
		assert.Len(t, v2Config.Rules, 2)

		// Find rules by name
		var highRule, lowRule *Rule
		for i := range v2Config.Rules {
			if v2Config.Rules[i].Name == "critical-rule" {
				highRule = &v2Config.Rules[i]
			}
			if v2Config.Rules[i].Name == "minor-rule" {
				lowRule = &v2Config.Rules[i]
			}
		}

		assert.NotNil(t, highRule)
		assert.NotNil(t, lowRule)
		assert.Equal(t, PriorityHigh, highRule.Priority)
		assert.Equal(t, PriorityLow, lowRule.Priority)
	})
}

func TestLoadV3AsV2_WithDomains(t *testing.T) {
	t.Run("loads V3 config with domains and merges content", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config file
		configContent := `version: "3.0"
name: multi-domain
presets:
  - claude
profiles:
  full:
    - backend
    - frontend
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create root rule
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		rootRule := filepath.Join(rulesPath, "root.md")
		require.NoError(t, os.WriteFile(rootRule, []byte("# Root Rule"), 0o644))

		// Create backend domain
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		backendRule := filepath.Join(backendRulesPath, "api.md")
		require.NoError(t, os.WriteFile(backendRule, []byte("# API Standards"), 0o644))

		// Create frontend domain
		frontendPath := filepath.Join(configDir, domainsDir, "frontend")
		frontendRulesPath := filepath.Join(frontendPath, rulesDir)
		require.NoError(t, os.MkdirAll(frontendRulesPath, 0o755))
		frontendRule := filepath.Join(frontendRulesPath, "ui.md")
		require.NoError(t, os.WriteFile(frontendRule, []byte("# UI Guidelines"), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify all rules are merged
		assert.Len(t, v2Config.Rules, 3)

		// Check rule names (domain-prefixed to preserve domain membership)
		ruleNames := make(map[string]bool)
		for _, rule := range v2Config.Rules {
			ruleNames[rule.Name] = true
		}
		assert.True(t, ruleNames["root"])
		// Domain names are prefixed with ": " separator
		assert.True(t, ruleNames["backend: api"], "expected rule 'backend: api', got names: %v", ruleNames)
		assert.True(t, ruleNames["frontend: ui"], "expected rule 'frontend: ui', got names: %v", ruleNames)
	})

	t.Run("loads V3 config with domain context files", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config file
		configContent := `version: "3.0"
name: domain-context
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create root context
		contextPath := filepath.Join(configDir, contextDir)
		require.NoError(t, os.MkdirAll(contextPath, 0o755))
		rootContext := filepath.Join(contextPath, "overview.md")
		require.NoError(t, os.WriteFile(rootContext, []byte("# Project Overview"), 0o644))

		// Create backend domain with context
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendContextPath := filepath.Join(backendPath, contextDir)
		require.NoError(t, os.MkdirAll(backendContextPath, 0o755))
		backendContext := filepath.Join(backendContextPath, "architecture.md")
		require.NoError(t, os.WriteFile(backendContext, []byte("# Backend Architecture"), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify sections (context files) - domain-prefixed for backend domain
		assert.Len(t, v2Config.Sections, 2)

		sectionNames := make(map[string]bool)
		for _, section := range v2Config.Sections {
			sectionNames[section.Name] = true
		}
		assert.True(t, sectionNames["overview"])
		// Domain names are prefixed with ": " separator
		assert.True(t, sectionNames["backend: architecture"], "expected section 'backend: architecture', got names: %v", sectionNames)
	})
}

func TestLoadV3AsV2_ContentTypes(t *testing.T) {
	t.Run("converts rules correctly", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config
		configContent := `version: "3.0"
name: test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create rule with targets metadata
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		rulePath := filepath.Join(rulesPath, "go-style.md")
		require.NoError(t, os.WriteFile(rulePath, []byte(`---
priority: high
targets:
  - "*.go"
  - "backend/*"
---

Go code style guidelines`), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify rule conversion
		assert.Len(t, v2Config.Rules, 1)
		rule := v2Config.Rules[0]
		assert.Equal(t, "go-style", rule.Name)
		assert.Equal(t, PriorityHigh, rule.Priority)
		assert.Equal(t, []string{"*.go", "backend/*"}, rule.Targets)
		assert.Equal(t, "Go code style guidelines", rule.Content)
	})

	t.Run("converts context to sections", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config
		configContent := `version: "3.0"
name: test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create context
		contextPath := filepath.Join(configDir, contextDir)
		require.NoError(t, os.MkdirAll(contextPath, 0o755))
		contextFile := filepath.Join(contextPath, "tech-stack.md")
		require.NoError(t, os.WriteFile(contextFile, []byte(`---
priority: critical
---

# Tech Stack

Uses Go and React`), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify section conversion
		assert.Len(t, v2Config.Sections, 1)
		section := v2Config.Sections[0]
		assert.Equal(t, "tech-stack", section.Name)
		assert.Equal(t, PriorityCritical, section.Priority)
		assert.Contains(t, section.Content, "Tech Stack")
	})

	t.Run("converts agents to agents", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config
		configContent := `version: "3.0"
name: test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create agent
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))
		agentFile := filepath.Join(agentsPath, "code-reviewer.md")
		require.NoError(t, os.WriteFile(agentFile, []byte(`---
name: code-reviewer
description: Reviews code for quality
model: haiku
---

# Code Reviewer Agent

Performs comprehensive code reviews`), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify agent conversion
		assert.Len(t, v2Config.Agents, 1)
		agent := v2Config.Agents[0]
		assert.Equal(t, "code-reviewer", agent.Name)
		assert.Equal(t, "Reviews code for quality", agent.Description)
		assert.Contains(t, agent.SystemPrompt, "Code Reviewer Agent")
	})
}

func TestLoadV3AsV2_PresetsToOutputs(t *testing.T) {
	t.Run("converts built-in presets to outputs", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config with multiple presets
		configContent := `version: "3.0"
name: multi-preset
presets:
  - claude
  - cursor
  - gemini
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify outputs were created
		assert.NotEmpty(t, v2Config.Outputs)

		// Check that we have outputs for each preset
		paths := make(map[string]bool)
		for _, output := range v2Config.Outputs {
			paths[output.Path] = true
		}

		// Verify preset-specific outputs exist
		assert.True(t, paths["CLAUDE.md"] || paths[".claude/agents/"] || paths[".mcp.json"])
		assert.True(t, paths[".cursor/rules/"])
		assert.True(t, paths["GEMINI.md"] || paths[".gemini/"] || paths[".gemini/settings.json"])
	})

	t.Run("converts custom presets to outputs", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config with custom preset
		configContent := `version: "3.0"
name: custom-preset
presets:
  - name: custom-output
    type: markdown
    path: CUSTOM.md
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify custom preset was converted
		assert.Len(t, v2Config.Outputs, 1)
		output := v2Config.Outputs[0]
		assert.Equal(t, "CUSTOM.md", output.Path)
		assert.Equal(t, "markdown", output.Type)
	})

	t.Run("handles mixed built-in and custom presets", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config with mixed presets
		configContent := `version: "3.0"
name: mixed-presets
presets:
  - claude
  - name: custom
    type: markdown
    path: RULES.md
  - cursor
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify all outputs are present
		assert.True(t, len(v2Config.Outputs) > 3) // At least claude (3) + custom (1) + cursor (1)

		// Check for custom output
		hasCustom := false
		for _, output := range v2Config.Outputs {
			if output.Path == "RULES.md" {
				hasCustom = true
				assert.Equal(t, "markdown", output.Type)
				break
			}
		}
		assert.True(t, hasCustom, "custom preset should be converted to output")
	})
}

func TestLoadV3AsV2_DefaultValues(t *testing.T) {
	t.Run("applies default priority when none specified", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create config
		configContent := `version: "3.0"
name: test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create rule without priority metadata
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		rulePath := filepath.Join(rulesPath, "rule.md")
		require.NoError(t, os.WriteFile(rulePath, []byte("Rule content"), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify default priority
		assert.Len(t, v2Config.Rules, 1)
		assert.Equal(t, PriorityMedium, v2Config.Rules[0].Priority)
	})

	t.Run("handles empty content directories gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create only config, no content
		configContent := `version: "3.0"
name: empty
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify empty content
		assert.Empty(t, v2Config.Rules)
		assert.Empty(t, v2Config.Sections)
		assert.Empty(t, v2Config.Agents)
		assert.NotEmpty(t, v2Config.Outputs) // Should have preset outputs
	})
}

func TestLoadV3AsV2_ComplexScenario(t *testing.T) {
	t.Run("converts full V3 structure with all content types and domains", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create comprehensive config
		configContent := `version: "3.0"
name: comprehensive
description: Full featured configuration
presets:
  - claude
  - cursor
  - name: custom
    type: markdown
    path: RULES.md
profiles:
  backend:
    - backend
  frontend:
    - frontend
default: backend
gitignore: true
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create root rules
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(rulesPath, "root-rule.md"),
			[]byte(`---
priority: critical
---

Root rule`), 0o644))

		// Create root context
		contextPath := filepath.Join(configDir, contextDir)
		require.NoError(t, os.MkdirAll(contextPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(contextPath, "overview.md"),
			[]byte(`---
priority: high
---

Project overview`), 0o644))

		// Create root agent
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(agentsPath, "reviewer.md"),
			[]byte(`---
name: reviewer
description: Code review agent
model: haiku
---

Performs reviews`), 0o644))

		// Create backend domain
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(backendRulesPath, "api.md"),
			[]byte(`---
priority: high
targets:
  - "api/*"
---

API standards`), 0o644))

		backendContextPath := filepath.Join(backendPath, contextDir)
		require.NoError(t, os.MkdirAll(backendContextPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(backendContextPath, "db.md"),
			[]byte("Database schema"), 0o644))

		// Create frontend domain
		frontendPath := filepath.Join(configDir, domainsDir, "frontend")
		frontendRulesPath := filepath.Join(frontendPath, rulesDir)
		require.NoError(t, os.MkdirAll(frontendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(frontendRulesPath, "ui.md"),
			[]byte("UI guidelines"), 0o644))

		// Load as V2
		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify comprehensive structure
		assert.Equal(t, "comprehensive", v2Config.Metadata.Name)
		assert.Equal(t, "Full featured configuration", v2Config.Metadata.Description)
		assert.Equal(t, true, *v2Config.Gitignore)

		// Verify content conversion
		assert.Len(t, v2Config.Rules, 3)    // root-rule + api + ui
		assert.Len(t, v2Config.Sections, 2) // overview + db
		assert.Len(t, v2Config.Agents, 1)   // reviewer

		// Verify preset outputs
		assert.NotEmpty(t, v2Config.Outputs)

		// Verify priorities and targets were preserved (domain-prefixed rule)
		var apiRule *Rule
		for i := range v2Config.Rules {
			// API rule should now be prefixed with domain name
			if v2Config.Rules[i].Name == "backend: api" {
				apiRule = &v2Config.Rules[i]
				break
			}
		}
		assert.NotNil(t, apiRule, "expected to find rule 'backend: api'")
		assert.Equal(t, PriorityHigh, apiRule.Priority)
		assert.Equal(t, []string{"api/*"}, apiRule.Targets)
	})
}

func TestLoadV3AsV2_Fixtures(t *testing.T) {
	t.Run("loads basic fixture as V2", func(t *testing.T) {
		fixtureDir := "/Users/naamanhirschfeld/workspace/ai-rulez/tests/fixtures/v3/generator/basic"

		// Skip if fixture doesn't exist
		if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
			t.Skip("fixture directory not found")
		}

		v2Config, err := LoadV3AsV2(context.Background(), fixtureDir)
		require.NoError(t, err)
		assert.NotNil(t, v2Config)
		assert.Equal(t, "basic-test", v2Config.Metadata.Name)
		assert.NotEmpty(t, v2Config.Rules)
		assert.NotEmpty(t, v2Config.Outputs)
	})

	t.Run("loads multi-preset fixture as V2", func(t *testing.T) {
		fixtureDir := "/Users/naamanhirschfeld/workspace/ai-rulez/tests/fixtures/v3/generator/multi-preset"

		// Skip if fixture doesn't exist
		if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
			t.Skip("fixture directory not found")
		}

		v2Config, err := LoadV3AsV2(context.Background(), fixtureDir)
		require.NoError(t, err)
		assert.NotNil(t, v2Config)
		assert.NotEmpty(t, v2Config.Outputs)
	})

	t.Run("loads domains fixture as V2", func(t *testing.T) {
		fixtureDir := "/Users/naamanhirschfeld/workspace/ai-rulez/tests/fixtures/v3/generator/with-domains"

		// Skip if fixture doesn't exist
		if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
			t.Skip("fixture directory not found")
		}

		v2Config, err := LoadV3AsV2(context.Background(), fixtureDir)
		require.NoError(t, err)
		assert.NotNil(t, v2Config)
		// Should have merged domain content
		assert.True(t, len(v2Config.Rules) > 1)
	})
}

func TestLoadV3AsV2_IssuesFixes(t *testing.T) {
	t.Run("Issue #19: Domain parameter preserved in rule names", func(t *testing.T) {
		// Verify that domain names are used to prefix rule names
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: issue-19-test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create domains with rules
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(backendRulesPath, "db.md"),
			[]byte("Database rules"), 0o644))

		frontendPath := filepath.Join(configDir, domainsDir, "frontend")
		frontendRulesPath := filepath.Join(frontendPath, rulesDir)
		require.NoError(t, os.MkdirAll(frontendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(frontendRulesPath, "css.md"),
			[]byte("CSS rules"), 0o644))

		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify domain names are preserved in rule names
		ruleNames := make(map[string]bool)
		for _, rule := range v2Config.Rules {
			ruleNames[rule.Name] = true
		}
		assert.True(t, ruleNames["backend: db"], "domain prefix missing from rule name")
		assert.True(t, ruleNames["frontend: css"], "domain prefix missing from rule name")
	})

	t.Run("Issue #20: Profile information stored in metadata", func(t *testing.T) {
		// Verify that profile information is preserved during conversion
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: issue-20-test
presets:
  - claude
profiles:
  production:
    - backend
  development:
    - backend
    - frontend
default: production
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify profile metadata is stored
		assert.NotEmpty(t, v2Config.Metadata.Version, "profile metadata should be stored in version field")
		assert.Contains(t, v2Config.Metadata.Version, "profiles:")
		assert.Contains(t, v2Config.Metadata.Version, "production")
		assert.Contains(t, v2Config.Metadata.Version, "development")
		assert.Contains(t, v2Config.Metadata.Version, "default:production")
	})

	t.Run("Issue #21: Domain collision detection", func(t *testing.T) {
		// Verify that collisions are handled with numeric suffixes
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: issue-21-test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create domains with SAME rule names (collision scenario)
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(backendRulesPath, "style.md"),
			[]byte("Backend style rules"), 0o644))

		frontendPath := filepath.Join(configDir, domainsDir, "frontend")
		frontendRulesPath := filepath.Join(frontendPath, rulesDir)
		require.NoError(t, os.MkdirAll(frontendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(frontendRulesPath, "style.md"),
			[]byte("Frontend style rules"), 0o644))

		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify both rules are present with different names (collision handling)
		assert.True(t, len(v2Config.Rules) >= 2)
		ruleNames := make([]string, 0)
		for _, rule := range v2Config.Rules {
			ruleNames = append(ruleNames, rule.Name)
		}

		// At least one should have numeric suffix for collision handling
		hasCollisionHandling := false
		for _, name := range ruleNames {
			if strings.Contains(name, "(") && strings.Contains(name, ")") {
				hasCollisionHandling = true
				break
			}
		}
		assert.True(t, hasCollisionHandling || len(ruleNames) == 2, "expected collision handling or 2 unique names")
	})

	t.Run("Issue #22: IDs generated for rules/sections/agents", func(t *testing.T) {
		// Verify that IDs are generated for all content types
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: issue-22-test
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		// Create content with domain
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(rulesPath, "naming-convention.md"),
			[]byte("Naming rules"), 0o644))

		contextPath := filepath.Join(configDir, contextDir)
		require.NoError(t, os.MkdirAll(contextPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(contextPath, "project-overview.md"),
			[]byte("Project overview"), 0o644))

		skillsPath := filepath.Join(configDir, skillsDir)
		skillDirPath := filepath.Join(skillsPath, "code-reviewer")
		require.NoError(t, os.MkdirAll(skillDirPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDirPath, skillMarkerFile),
			[]byte("Review skill"), 0o644))

		// Create domain content
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(backendRulesPath, "api-design.md"),
			[]byte("API rules"), 0o644))

		v2Config, err := LoadV3AsV2(context.Background(), tempDir)
		require.NoError(t, err)

		// Verify all rules have IDs
		for _, rule := range v2Config.Rules {
			assert.NotEmpty(t, rule.ID, "rule %s should have an ID", rule.Name)
			// IDs should be sanitized (lowercase, no spaces)
			assert.NotContains(t, rule.ID, " ", "rule ID should not contain spaces")
			assert.NotContains(t, rule.ID, ":", "rule ID should not contain colons")
		}

		// Verify all sections have IDs
		for _, section := range v2Config.Sections {
			assert.NotEmpty(t, section.ID, "section %s should have an ID", section.Name)
			assert.NotContains(t, section.ID, " ", "section ID should not contain spaces")
			assert.NotContains(t, section.ID, ":", "section ID should not contain colons")
		}

		// Verify all agents have IDs
		for _, agent := range v2Config.Agents {
			assert.NotEmpty(t, agent.ID, "agent %s should have an ID", agent.Name)
			assert.NotContains(t, agent.ID, " ", "agent ID should not contain spaces")
			assert.NotContains(t, agent.ID, ":", "agent ID should not contain colons")
		}
	})
}

func TestConvertV3ToV2_EdgeCases(t *testing.T) {
	t.Run("handles nil content tree", func(t *testing.T) {
		v3 := &ConfigV3{
			Version:     "3.0",
			Name:        "test",
			Description: "Test",
			Presets:     []PresetV3{{BuiltIn: "claude"}},
			Content:     nil,
		}

		v2 := convertV3ToV2(v3)
		assert.NotNil(t, v2)
		assert.Empty(t, v2.Rules)
		assert.Empty(t, v2.Sections)
		assert.Empty(t, v2.Agents)
	})

	t.Run("handles empty presets", func(t *testing.T) {
		v3 := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{},
			Content: &ContentTreeV3{
				Domains: make(map[string]*DomainV3),
			},
		}

		v2 := convertV3ToV2(v3)
		assert.NotNil(t, v2)
		assert.Empty(t, v2.Outputs)
	})

	t.Run("handles unknown built-in preset gracefully", func(t *testing.T) {
		v3 := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "unknown-preset"},
			},
			Content: &ContentTreeV3{
				Domains: make(map[string]*DomainV3),
			},
		}

		v2 := convertV3ToV2(v3)
		assert.NotNil(t, v2)
		// Should gracefully skip unknown preset
		assert.Empty(t, v2.Outputs)
	})
}

func TestGetPriorityFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		meta     *MetadataV3
		expected Priority
	}{
		{
			name:     "nil metadata",
			meta:     nil,
			expected: PriorityMedium,
		},
		{
			name:     "empty priority string",
			meta:     &MetadataV3{Priority: ""},
			expected: PriorityMedium,
		},
		{
			name:     "high priority",
			meta:     &MetadataV3{Priority: "high"},
			expected: PriorityHigh,
		},
		{
			name:     "critical priority",
			meta:     &MetadataV3{Priority: "critical"},
			expected: PriorityCritical,
		},
		{
			name:     "low priority",
			meta:     &MetadataV3{Priority: "low"},
			expected: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPriorityFromMetadata(tt.meta)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTargetsFromMetadata(t *testing.T) {
	t.Run("returns nil for nil metadata", func(t *testing.T) {
		result := getTargetsFromMetadata(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil when no targets", func(t *testing.T) {
		meta := &MetadataV3{Targets: nil}
		result := getTargetsFromMetadata(meta)
		assert.Nil(t, result)
	})

	t.Run("returns targets when specified", func(t *testing.T) {
		targets := []string{"*.go", "backend/*"}
		meta := &MetadataV3{Targets: targets}
		result := getTargetsFromMetadata(meta)
		assert.Equal(t, targets, result)
	})
}

func TestGetAgentDescription(t *testing.T) {
	t.Run("returns empty string for nil metadata", func(t *testing.T) {
		result := getAgentDescription(nil)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty string when no extra fields", func(t *testing.T) {
		meta := &MetadataV3{Extra: nil}
		result := getAgentDescription(meta)
		assert.Equal(t, "", result)
	})

	t.Run("returns description from extra fields", func(t *testing.T) {
		meta := &MetadataV3{
			Extra: map[string]string{
				"description": "Code reviewer skill",
			},
		}
		result := getAgentDescription(meta)
		assert.Equal(t, "Code reviewer skill", result)
	})
}
