package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateValidConfigWithCustomPath() {
	configPath := filepath.Join(s.workingDir, "custom.yaml")
	testutil.WriteFile(s.T(), s.workingDir, "custom.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--config", configPath)

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateMinimalConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.MinimalConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithAgents() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithAgents)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithTargets() {
	config := `metadata:
  name: "Test Project"

outputs:
  - path: "test.md"

rules:
  - name: "Test Rule"
    content: "Test content"
    targets: ["test.md"]
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateInvalidYAML() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidYAMLConfig)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "Error")
	result.AssertStderrContains(s.T(), "invalid")
}

func (s *ValidateCLITestSuite) TestValidateInvalidSchema() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidSchemaConfig)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "validation failed")
}

func (s *ValidateCLITestSuite) TestValidateMissingConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "configuration file")
}

func (s *ValidateCLITestSuite) TestValidateNonExistentConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate", "--config", "nonexistent.yaml")

	// Windows returns "The system cannot find the file specified" instead of "no such file"
	stderr := result.Stderr
	if !strings.Contains(stderr, "no such file") && !strings.Contains(stderr, "cannot find the file") {
		s.T().Errorf("Expected error message about missing file, got: %s", stderr)
	}
}

func (s *ValidateCLITestSuite) TestValidateEmptyConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", "")

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "validation failed")
}

func (s *ValidateCLITestSuite) TestValidateMissingRequiredFields() {
	config := `metadata:
  version: "1.0.0"
# Missing name and outputs
rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "validation failed")
}

func (s *ValidateCLITestSuite) TestValidateInvalidTargetReference() {
	config := `metadata:
  name: "Test Project"

targets:
  frontend: ["*.ts"]

outputs:
  - path: "test.md"

rules:
  - name: "Test Rule"
    content: "Test content"
    targets: ["@nonexistent"]  # Invalid target reference
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")

	result.AssertStderrContains(s.T(), "validation")
}

func (s *ValidateCLITestSuite) TestValidateVerboseOutput() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--verbose")

	result.AssertOutputContains(s.T(), "valid")
}

func (s *ValidateCLITestSuite) TestValidateQuietOutput() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--quiet")
}

func (s *ValidateCLITestSuite) TestValidateConfigWithWarnings() {
	config := `metadata:
  name: "Test Project"

outputs:
  - path: "test.md"

rules:
  - name: "Rule with Zero Priority"
    priority: minimal
    content: "This rule has low priority"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	result.AssertOutputContains(s.T(), "valid")
}
