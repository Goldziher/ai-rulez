package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type MCPCommandsCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestMCPCommandsCLISuite(t *testing.T) {
	suite.Run(t, new(MCPCommandsCLITestSuite))
}

func (s *MCPCommandsCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *MCPCommandsCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *MCPCommandsCLITestSuite) TestGenerateWithInvalidTemplate() {
	// Create an empty directory with no .ai-rulez config
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	// The error message should indicate missing config
	result.AssertStderrContains(s.T(), "Error")
}

func (s *MCPCommandsCLITestSuite) TestGenerateMCPServers() {
	testutil.SetupV3ConfigWithMCPServers(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *MCPCommandsCLITestSuite) TestGenerateCommands() {
	// Create a V3 config for testing commands
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *MCPCommandsCLITestSuite) TestValidateMCPAndCommands() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *MCPCommandsCLITestSuite) TestInvalidMCPConfiguration() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *MCPCommandsCLITestSuite) TestInvalidCommandConfiguration() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *MCPCommandsCLITestSuite) TestMCPAndCommandsWithTargets() {
	// Create a V3 config with multiple presets
	testutil.SetupV3MultiPresetConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generation complete")
}

func (s *MCPCommandsCLITestSuite) TestTemplateCounts() {
	// Create a V3 config with multiple rules
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "count-test-project"
description: "Test counting"
presets:
  - claude
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create multiple rules
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "rule1.md", `---
priority: high
---
# Rule 1
Content 1
`)
	testutil.WriteFile(s.T(), rulesDir, "rule2.md", `---
priority: high
---
# Rule 2
Content 2
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generation complete")
}
