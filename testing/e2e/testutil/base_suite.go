package testutil

import (
	"github.com/stretchr/testify/suite"
)

type BaseE2ESuite struct {
	suite.Suite
	WorkingDir string
}

func (s *BaseE2ESuite) SetupTest() {
	s.WorkingDir = CreateTempDir(s.T())
	WriteFile(s.T(), s.WorkingDir, "ai_rulez.yaml", BasicConfig)
}

func (s *BaseE2ESuite) TearDownSuite() {
	CleanupTestBinary()
}

func (s *BaseE2ESuite) GetWorkingDir() string {
	return s.WorkingDir
}

func (s *BaseE2ESuite) WriteConfigFile(filename string, content string) {
	WriteFile(s.T(), s.WorkingDir, filename, content)
}

func (s *BaseE2ESuite) RunCLI(args ...string) *CLIResult {
	return RunCLI(s.T(), s.WorkingDir, args...)
}

func (s *BaseE2ESuite) RunCLIExpectSuccess(args ...string) *CLIResult {
	return RunCLIExpectSuccess(s.T(), s.WorkingDir, args...)
}

func (s *BaseE2ESuite) RunCLIExpectError(args ...string) *CLIResult {
	return RunCLIExpectError(s.T(), s.WorkingDir, args...)
}

func (s *BaseE2ESuite) StartMCP() *MCPClient {
	return StartMCPServer(s.T(), s.WorkingDir)
}
