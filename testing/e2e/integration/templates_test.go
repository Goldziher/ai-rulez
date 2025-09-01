package integration

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "CLAUDE.md")
	content := testutil.ReadFile(s.T(), outputPath)

	// Check default template structure
	s.Contains(content, "# Test Project")
	s.Contains(content, "Generated on")
	s.Contains(content, "## Basic Rule")
	s.Contains(content, "**Priority:** 5")
	s.Contains(content, "This is a basic rule for testing")
	s.Contains(content, "## High Priority Rule")
	s.Contains(content, "**Priority:** 9")

	// Higher priority should come first
	basicPos := s.findPosition(content, "Basic Rule")
	highPos := s.findPosition(content, "High Priority Rule")
	s.Less(highPos, basicPos, "High priority rule should appear before basic rule")
}

func (s *TemplatesTestSuite) TestCustomTemplate() {
	config := `metadata:
  name: "Custom Template Test"

outputs:
  - file: "custom.md"
    template: |
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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "custom.md")
	content := testutil.ReadFile(s.T(), outputPath)

	s.Contains(content, "# Custom Template for Custom Template Test")
	s.Contains(content, "Rules Count: 2")
	s.Contains(content, "- First Rule: First content")
	s.Contains(content, "- Second Rule: Second content")
}

func (s *TemplatesTestSuite) TestTemplateWithSections() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "CLAUDE.md")
	content := testutil.ReadFile(s.T(), outputPath)

	// Check sections are included
	s.Contains(content, "Development Guidelines")
	s.Contains(content, "Follow these guidelines for development")
}

func (s *TemplatesTestSuite) TestTemplateWithAgents() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithAgents)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Check main output
	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	claudeContent := testutil.ReadFile(s.T(), claudePath)
	s.Contains(claudeContent, "Project with Agents")

	// Check agent-specific output
	agentPath := filepath.Join(s.workingDir, ".claude", "agents", "code-reviewer.md")
	agentContent := testutil.ReadFile(s.T(), agentPath)

	s.Contains(agentContent, "code-reviewer")
	s.Contains(agentContent, "Reviews code for quality and best practices")
	s.Contains(agentContent, "Read")
	s.Contains(agentContent, "Edit")
	s.Contains(agentContent, "Grep")
	s.Contains(agentContent, "You are a code reviewer focused on quality")
}

func (s *TemplatesTestSuite) TestTemplateVariables() {
	config := `metadata:
  name: "Variable Test"
  version: "2.1.0"
  description: "Testing template variables"

outputs:
  - file: "variables.md"
    template: |
      Project: {{.ProjectName}}
      Version: {{.Version}}
      Description: {{.Description}}
      Timestamp: {{.Timestamp.Format "2006-01-02"}}
      Rules: {{.RuleCount}}
      Sections: {{.SectionCount}}
      Agents: {{.AgentCount}}

rules:
  - name: "Test Rule"
    content: "Test content"

sections:
  - title: "Test Section"
    content: "Test section content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "variables.md")
	content := testutil.ReadFile(s.T(), outputPath)

	s.Contains(content, "Project: Variable Test")
	s.Contains(content, "Version: 2.1.0")
	s.Contains(content, "Description: Testing template variables")
	s.Contains(content, "Rules: 1")
	s.Contains(content, "Sections: 1")
	s.Contains(content, "Agents: 0")

	// Timestamp should be today's date
	s.Regexp(`Timestamp: \d{4}-\d{2}-\d{2}`, content)
}

func (s *TemplatesTestSuite) TestTemplateConditionals() {
	config := `metadata:
  name: "Conditionals Test"
  version: "1.0.0"

outputs:
  - file: "conditionals.md"
    template: |
      # {{.ProjectName}}
      {{- if .Version}}
      Version: {{.Version}}
      {{- end}}
      {{- if .Description}}
      Description: {{.Description}}
      {{- end}}
      {{- if .Rules}}
      
      ## Rules
      {{range .Rules}}
      - {{.Name}}
      {{end}}
      {{- end}}
      {{- if not .Sections}}
      
      No sections defined.
      {{- end}}

rules:
  - name: "Rule 1"
    content: "Content 1"
  - name: "Rule 2"
    content: "Content 2"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "conditionals.md")
	content := testutil.ReadFile(s.T(), outputPath)

	s.Contains(content, "# Conditionals Test")
	s.Contains(content, "Version: 1.0.0")
	s.NotContains(content, "Description:") // No description provided
	s.Contains(content, "## Rules")
	s.Contains(content, "- Rule 1")
	s.Contains(content, "- Rule 2")
	s.Contains(content, "No sections defined.")
}

func (s *TemplatesTestSuite) TestInvalidTemplate() {
	config := `metadata:
  name: "Invalid Template Test"

outputs:
  - file: "invalid.md"
    template: |
      {{.NonExistentField}}
      {{range .Rules}}
      {{.InvalidField}}
      {{end}}

rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "template")
}

func (s *TemplatesTestSuite) TestMalformedTemplate() {
	config := `metadata:
  name: "Malformed Template Test"

outputs:
  - file: "malformed.md"
    template: |
      {{.ProjectName}
      {{range .Rules}}
      {{.Name}}

rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "template")
}

func (s *TemplatesTestSuite) TestDirectoryTemplates() {
	config := `metadata:
  name: "Directory Templates Test"

outputs:
  - path: ".test-rules/"
    type: "rule"  
    naming_scheme: "{priority}-{name}.md"
    template: |
      # Rule: {{.Name}}
      Priority: {{.Priority}}
      
      {{.Content}}

rules:
  - name: "High Priority"
    priority: 9
    content: "High priority content"
  - name: "Low Priority"
    priority: 3
    content: "Low priority content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Check individual rule files were created
	highPriorityPath := filepath.Join(s.workingDir, ".test-rules", "9-High Priority.md")
	s.True(testutil.FileExists(s.T(), highPriorityPath))

	highContent := testutil.ReadFile(s.T(), highPriorityPath)
	s.Contains(highContent, "# Rule: High Priority")
	s.Contains(highContent, "Priority: 9")
	s.Contains(highContent, "High priority content")

	lowPriorityPath := filepath.Join(s.workingDir, ".test-rules", "3-Low Priority.md")
	s.True(testutil.FileExists(s.T(), lowPriorityPath))

	lowContent := testutil.ReadFile(s.T(), lowPriorityPath)
	s.Contains(lowContent, "# Rule: Low Priority")
	s.Contains(lowContent, "Priority: 3")
	s.Contains(lowContent, "Low priority content")
}

func (s *TemplatesTestSuite) TestAgentTemplates() {
	config := `metadata:
  name: "Agent Templates Test"

outputs:
  - path: ".custom-agents/"
    type: "agent"
    naming_scheme: "{name}-agent.md"
    template: |
      # Agent: {{.Name}}
      
      **Description:** {{.Description}}
      **Priority:** {{.Priority}}
      
      {{if .Tools}}
      ## Available Tools
      {{range .Tools}}
      - {{.}}
      {{end}}
      {{end}}
      
      {{if .SystemPrompt}}
      ## System Prompt
      {{.SystemPrompt}}
      {{end}}

agents:
  - name: "reviewer"
    description: "Code review agent"
    priority: 8
    tools: ["Read", "Edit", "Grep"]
    system_prompt: "You are a code reviewer"
  - name: "documenter"
    description: "Documentation agent"
    priority: 6
    tools: ["Read", "Write"]
    system_prompt: "You write documentation"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Check reviewer agent file
	reviewerPath := filepath.Join(s.workingDir, ".custom-agents", "reviewer-agent.md")
	s.True(testutil.FileExists(s.T(), reviewerPath))

	reviewerContent := testutil.ReadFile(s.T(), reviewerPath)
	s.Contains(reviewerContent, "name: reviewer")
	s.Contains(reviewerContent, "description: Code review agent")
	s.Contains(reviewerContent, "- Read")
	s.Contains(reviewerContent, "- Edit")
	s.Contains(reviewerContent, "- Grep")
	s.Contains(reviewerContent, "You are a code reviewer")
}

// Helper method to find the position of text in content
func (s *TemplatesTestSuite) findPosition(content, text string) int {
	pos := 0
	for i := range content {
		if i < len(content) && content[i:] != "" {
			if len(content[i:]) >= len(text) && content[i:i+len(text)] == text {
				return pos
			}
		}
		pos++
	}
	return -1
}
