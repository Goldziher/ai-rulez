package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type GenerateCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestGenerateCLISuite(t *testing.T) {
	suite.Run(t, new(GenerateCLITestSuite))
}

func (s *GenerateCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *GenerateCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *GenerateCLITestSuite) TestGenerateBasicConfig() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateWithCustomConfig() {
	// Create a custom V3 config directory structure
	aiRulesDir := filepath.Join(s.workingDir, "custom", ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "custom-project"
description: "Custom configuration"
presets:
  - claude
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules directory with a rule
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "test-rule.md", `---
priority: high
---

# Test Rule

Test content for custom config
`)

	result := testutil.RunCLIExpectSuccess(s.T(), filepath.Join(s.workingDir, "custom"), "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateWithAgents() {
	// Create a V3 config with agents
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "agent-test-project"
description: "Project with agents"
presets:
  - claude
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "test-rule.md", `---
priority: high
---

# Test Rule

Test content
`)

	// Create agents directory
	agentsDir := filepath.Join(aiRulesDir, "agents")
	s.NoError(os.MkdirAll(agentsDir, 0o755))
	testutil.WriteFile(s.T(), agentsDir, "code-reviewer.md", `---
priority: high
description: "Reviews code for quality"
---

# Code Reviewer Agent

This agent reviews code for quality and best practices.
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateWithTargets() {
	// Create a V3 config with multiple presets
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "multi-preset-project"
description: "Project with multiple presets"
presets:
  - claude
  - cursor
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "test-rule.md", `---
priority: high
---

# Test Rule

Test content
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateWithoutConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), ".ai-rulez directory not found")
}

func (s *GenerateCLITestSuite) TestGenerateWithInvalidConfig() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *GenerateCLITestSuite) TestGenerateWithSchemaInvalidConfig() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *GenerateCLITestSuite) TestGenerateVerboseOutput() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--verbose")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateQuietOutput() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--quiet")

	s.Empty(result.Stdout)
}

func (s *GenerateCLITestSuite) TestGenerateIdempotent() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result1 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result1.AssertOutputContains(s.T(), "Generation complete")

	// Running generate again should also succeed (idempotent)
	result2 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result2.AssertOutputContains(s.T(), "Generation complete")
}

func (s *GenerateCLITestSuite) TestGenerateDirectoryOutputs() {
	// Create a V3 config with cursor preset
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "directory-test-project"
description: "Test directory outputs"
presets:
  - cursor
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "test-rule.md", `---
priority: high
---

# Test Rule

Test content
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}
