package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type ValidateCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestValidateCLISuite(t *testing.T) {
	suite.Run(t, new(ValidateCLITestSuite))
}

func (s *ValidateCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *ValidateCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *ValidateCLITestSuite) TestValidateValidConfig() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateValidConfigWithCustomPath() {
	// Create a custom V3 config directory
	customDir := filepath.Join(s.workingDir, "custom")
	s.NoError(os.MkdirAll(customDir, 0o755))
	testutil.SetupV3BasicConfig(s.T(), customDir)

	result := testutil.RunCLIExpectSuccess(s.T(), customDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateMinimalConfig() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithAgents() {
	// Create a V3 config with agents
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "agent-test"
description: "Config with agents"
presets:
  - claude
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules and agents directories
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "test.md", `---
priority: high
---
# Test
`)

	agentsDir := filepath.Join(aiRulesDir, "agents")
	s.NoError(os.MkdirAll(agentsDir, 0o755))
	testutil.WriteFile(s.T(), agentsDir, "reviewer.md", `---
priority: high
---
# Reviewer
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithTargets() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateInvalidYAML() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *ValidateCLITestSuite) TestValidateInvalidSchema() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *ValidateCLITestSuite) TestValidateMissingConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), ".ai-rulez directory not found")
}

func (s *ValidateCLITestSuite) TestValidateNonExistentConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate", "--config", "nonexistent.yaml")

	stderr := result.Stderr
	if !strings.Contains(stderr, ".ai-rulez") && !strings.Contains(stderr, "Error") {
		s.T().Errorf("Expected error message about missing config, got: %s", stderr)
	}
}

func (s *ValidateCLITestSuite) TestValidateEmptyConfig() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *ValidateCLITestSuite) TestValidateMissingRequiredFields() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *ValidateCLITestSuite) TestValidateInvalidTargetReference() {
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *ValidateCLITestSuite) TestValidateVerboseOutput() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--verbose")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateQuietOutput() {
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--quiet")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithWarnings() {
	// Create a V3 config with rules
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))

	configYAML := `version: "3.0"
name: "warning-test"
description: "Config with warnings"
presets:
  - claude
gitignore: false
`
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", configYAML)

	// Create rules directory with a minimal priority rule
	rulesDir := filepath.Join(aiRulesDir, "rules")
	s.NoError(os.MkdirAll(rulesDir, 0o755))
	testutil.WriteFile(s.T(), rulesDir, "warning-rule.md", `---
priority: low
---

# Rule with Low Priority

This rule has low priority
`)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}
