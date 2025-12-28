package integration

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type TemplatesTestSuite struct {
	suite.Suite
	workingDir string
}

func TestTemplatesSuite(t *testing.T) {
	suite.Run(t, new(TemplatesTestSuite))
}

func (s *TemplatesTestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *TemplatesTestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *TemplatesTestSuite) TestDefaultTemplate() {
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// After migration to V3, verify that .ai-rulez/ directory structure was created
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory to be created")

	// Verify rules directory exists
	rulesPath := filepath.Join(aiRulezPath, "rules")
	s.DirExists(rulesPath, "Expected .ai-rulez/rules directory")

	// Verify skills directory exists (from sections)
	skillsPath := filepath.Join(aiRulezPath, "skills")
	s.DirExists(skillsPath, "Expected .ai-rulez/skills directory")
}

func (s *TemplatesTestSuite) TestCustomTemplate() {
	config := `metadata:
  name: "Custom Template Test"

presets:
  - claude

outputs:
  - path: "custom.md"
    template:
      type: inline
      value: |
        # Custom Template for {{.ProjectName}}

        Rules Count: {{.RuleCount}}

        {{range .Rules}}
        - {{.Name}}: {{.Content}}
        {{end}}

rules:
  - name: "First Rule"
    content: "First content"
  - name: "Second Rule"
    content: "Second content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Verify migration to V3 created the expected structure
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory")

	// Verify rules were migrated
	rulesPath := filepath.Join(aiRulezPath, "rules")
	s.DirExists(rulesPath, "Expected .ai-rulez/rules directory")
}

func (s *TemplatesTestSuite) TestTemplateWithSections() {
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// After migration to V3, sections are converted to skills/
	// Check that the migration was successful by verifying .ai-rulez/skills/ directory exists
	skillPath := filepath.Join(s.workingDir, ".ai-rulez", "skills", "development-guidelines", "SKILL.md")
	skillContent := testutil.ReadFile(s.T(), skillPath)

	s.Contains(skillContent, "Development Guidelines")
	s.Contains(skillContent, "Follow these guidelines for development")
}

func (s *TemplatesTestSuite) TestTemplateWithAgents() {
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", testutil.ConfigWithAgents)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// After migration to V3, files are in .ai-rulez/ directory
	// Check that the migration was successful by verifying .ai-rulez/agents/ directory exists
	agentPath := filepath.Join(s.workingDir, ".ai-rulez", "agents", "code-reviewer.md")
	agentContent := testutil.ReadFile(s.T(), agentPath)

	s.Contains(agentContent, "name: code-reviewer")
	s.Contains(agentContent, "Reviews code for quality and best practices")
	s.Contains(agentContent, "You are a code reviewer focused on quality")
}

func (s *TemplatesTestSuite) TestTemplateVariables() {
	config := `metadata:
  name: "Variable Test"
  version: "2.1.0"
  description: "Testing template variables"

presets:
  - claude

outputs:
  - path: "variables.md"
    template:
      type: inline
      value: |
        Project: {{.ProjectName}}
        Version: {{.Version}}
        Description: {{.Description}}
        Rules: {{.RuleCount}}
        Sections: {{.SectionCount}}
        Agents: {{.AgentCount}}

rules:
  - name: "Test Rule"
    content: "Test content"

sections:
  - name: "Test Section"
    content: "Test section content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Verify migration to V3 was successful
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory")

	// Verify both rules and sections (skills) were migrated
	rulesPath := filepath.Join(aiRulezPath, "rules")
	s.DirExists(rulesPath, "Expected .ai-rulez/rules directory")

	skillsPath := filepath.Join(aiRulezPath, "skills")
	s.DirExists(skillsPath, "Expected .ai-rulez/skills directory")
}

func (s *TemplatesTestSuite) TestTemplateConditionals() {
	config := `metadata:
  name: "Conditionals Test"
  version: "1.0.0"

presets:
  - claude

outputs:
  - path: "conditionals.md"
    template:
      type: inline
      value: |
        # {{.ProjectName}}
        {{- if .Version}}
        Version: {{.Version}}
        {{- end}}
        {{- if .Rules}}

        ## Rules
        {{range .Rules}}
        - {{.Name}}
        {{end}}
        {{- end}}

rules:
  - name: "Rule 1"
    content: "Content 1"
  - name: "Rule 2"
    content: "Content 2"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Verify migration to V3 was successful
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory")

	// Verify config.yaml was created with correct metadata
	configPath := filepath.Join(aiRulezPath, "config.yaml")
	configContent := testutil.ReadFile(s.T(), configPath)
	s.Contains(configContent, "Conditionals Test")
}

func (s *TemplatesTestSuite) TestInvalidTemplate() {
	config := `metadata:
  name: "Invalid Template Test"

presets:
  - claude

outputs:
  - path: "invalid.md"
    template: |
      {{.NonExistentField}}
      {{range .Rules}}
      {{.InvalidField}}
      {{end}}

rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "template")
}

func (s *TemplatesTestSuite) TestMalformedTemplate() {
	config := `metadata:
  name: "Malformed Template Test"

presets:
  - claude

outputs:
  - path: "malformed.md"
    template: |
      {{.ProjectName}
      {{range .Rules}}
      {{.Name}}

rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "template")
}

func (s *TemplatesTestSuite) TestDirectoryTemplates() {
	config := `metadata:
  name: "Directory Templates Test"

presets:
  - claude

outputs:
  - path: ".test-rules/"
    type: "rule"
    naming_scheme: "{priority}-{name}.md"
    template:
      type: inline
      value: |
        # Rule: {{.Name}}
        Priority: {{.Priority}}

        {{.Content}}

rules:
  - name: "High Priority"
    priority: critical
    content: "High priority content"
  - name: "Low Priority"
    priority: low
    content: "Low priority content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// After V2->V3 migration, verify the rules were migrated to .ai-rulez/rules/
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory")

	rulesPath := filepath.Join(aiRulezPath, "rules")
	s.DirExists(rulesPath, "Expected .ai-rulez/rules directory")

	// Verify both rules were migrated (names are sanitized: spaces -> hyphens, lowercase)
	highPriorityPath := filepath.Join(rulesPath, "high-priority.md")
	s.True(testutil.FileExists(s.T(), highPriorityPath))

	lowPriorityPath := filepath.Join(rulesPath, "low-priority.md")
	s.True(testutil.FileExists(s.T(), lowPriorityPath))
}

func (s *TemplatesTestSuite) TestAgentTemplates() {
	config := `metadata:
  name: "Agent Templates Test"

presets:
  - claude

outputs:
  - path: ".custom-agents/"
    type: "agent"
    naming_scheme: "{name}-agent.md"
    template:
      type: inline
      value: |
        # Agent: {{.Name}}

        **Description:** {{.Description}}
        **Priority:** {{.Priority}}

agents:
  - name: "reviewer"
    description: "Code review agent"
    priority: high
    tools: ["Read", "Edit", "Grep"]
    system_prompt: "You are a code reviewer"
  - name: "documenter"
    description: "Documentation agent"
    priority: medium
    tools: ["Read", "Write"]
    system_prompt: "You write documentation"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai-rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// After V2->V3 migration, verify agents were migrated to .ai-rulez/agents/
	aiRulezPath := filepath.Join(s.workingDir, ".ai-rulez")
	s.DirExists(aiRulezPath, "Expected .ai-rulez directory")

	agentsPath := filepath.Join(aiRulezPath, "agents")
	s.DirExists(agentsPath, "Expected .ai-rulez/agents directory")

	// Verify both agents were migrated
	reviewerPath := filepath.Join(agentsPath, "reviewer.md")
	s.True(testutil.FileExists(s.T(), reviewerPath))

	documenterPath := filepath.Join(agentsPath, "documenter.md")
	s.True(testutil.FileExists(s.T(), documenterPath))
}
