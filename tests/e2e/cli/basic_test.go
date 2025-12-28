package cli

import (
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type BasicCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestBasicCLISuite(t *testing.T) {
	suite.Run(t, new(BasicCLITestSuite))
}

func (s *BasicCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *BasicCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *BasicCLITestSuite) TestRootHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "--help")
	result.AssertStdoutContains(s.T(), "ai-rulez is a lightning-fast CLI tool for managing AI assistant rules")
	result.AssertStdoutContains(s.T(), "Available Commands:")
	result.AssertStdoutContains(s.T(), "generate")
	result.AssertStdoutContains(s.T(), "init")
	result.AssertStdoutContains(s.T(), "validate")
}

func (s *BasicCLITestSuite) TestRootHelpWithoutArgs() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir)
	result.AssertStdoutContains(s.T(), "ai-rulez is a lightning-fast CLI tool for managing AI assistant rules")
}

func (s *BasicCLITestSuite) TestVersion() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "version")
	result.AssertStderrContains(s.T(), "ai-rulez version")
}

func (s *BasicCLITestSuite) TestVersionShortFlag() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "version")
	result.AssertStderrContains(s.T(), "ai-rulez version")
}

func (s *BasicCLITestSuite) TestGenerateHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--help")
	result.AssertStdoutContains(s.T(), "Generate AI assistant rule files")
}

func (s *BasicCLITestSuite) TestValidateHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate", "--help")
	result.AssertStdoutContains(s.T(), "Validate")
}

func (s *BasicCLITestSuite) TestInitHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "--help")
	result.AssertStdoutContains(s.T(), "Initialize")
}

func (s *BasicCLITestSuite) TestMCPHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "mcp", "--help")
	result.AssertStdoutContains(s.T(), "Model Context Protocol")
}

func (s *BasicCLITestSuite) TestAddHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "--help")
	result.AssertStdoutContains(s.T(), "Add rules, context, or skills")
}

func (s *BasicCLITestSuite) TestRemoveHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "remove", "--help")
	result.AssertStdoutContains(s.T(), "Remove rules, context, or skills")
}

func (s *BasicCLITestSuite) TestListHelp() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "--help")
	result.AssertStdoutContains(s.T(), "List rules, context, or skills")
}

func (s *BasicCLITestSuite) TestInvalidCommand() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "invalid-command")
	result.AssertStderrContains(s.T(), "unknown command")
}

func (s *BasicCLITestSuite) TestInvalidFlag() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "--invalid-flag")
	result.AssertStderrContains(s.T(), "unknown flag")
}
