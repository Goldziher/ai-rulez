package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
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
	result.AssertStdoutContains(s.T(), "ai-rulez version")
}

func (s *BasicCLITestSuite) TestVersionShortFlag() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "version")
	result.AssertStdoutContains(s.T(), "ai-rulez version")
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

// TestLegacyConfigIsRefused covers a project that has only a V2/V3 config: every
// command that loads a config exits 1 and names the file and ai-rulez migrate v5.
func (s *BasicCLITestSuite) TestLegacyConfigIsRefused() {
	tests := []struct {
		name string
		file string
	}{
		{"V3 directory config", ".ai-rulez/config.yaml"},
		{"V2 flat file", "ai-rulez.yaml"},
	}
	for _, tt := range tests {
		for _, command := range []string{"generate", "validate", "doctor"} {
			s.Run(tt.name+" "+command, func() {
				// Arrange
				dir := testutil.CreateTempDir(s.T())
				s.Require().NoError(os.MkdirAll(filepath.Dir(filepath.Join(dir, tt.file)), 0o755))
				testutil.WriteFile(s.T(), dir, tt.file, "version: \"3.0\"\nname: legacy\npresets:\n  - claude\n")

				// Act
				result := testutil.RunCLI(s.T(), dir, command)

				// Assert
				s.Equal(1, result.ExitCode)
				result.AssertOutputContains(s.T(), filepath.FromSlash(tt.file))
				result.AssertOutputContains(s.T(), "ai-rulez migrate v5")
			})
		}
	}
}
