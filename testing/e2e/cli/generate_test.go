package cli

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated")
	result.AssertOutputContains(s.T(), "CLAUDE.md")

	outputPath := filepath.Join(s.workingDir, "CLAUDE.md")
	s.True(testutil.FileExists(s.T(), outputPath), "Output file should be created")

	content := testutil.ReadFile(s.T(), outputPath)
	s.Contains(content, "Test Project")
	s.Contains(content, "Basic Rule")
	s.Contains(content, "This is a basic rule for testing")
	s.Contains(content, "High Priority Rule")
}

func (s *GenerateCLITestSuite) TestGenerateWithCustomConfig() {
	configPath := filepath.Join(s.workingDir, "custom-config.yaml")
	testutil.WriteFile(s.T(), s.workingDir, "custom-config.yaml", testutil.MinimalConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--config", configPath)

	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "output.md")
	s.True(testutil.FileExists(s.T(), outputPath))

	content := testutil.ReadFile(s.T(), outputPath)
	s.Contains(content, "Minimal Project")
	s.Contains(content, "Only Rule")
}

func (s *GenerateCLITestSuite) TestGenerateWithAgents() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithAgents)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated")

	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	s.True(testutil.FileExists(s.T(), claudePath))

	agentsDir := filepath.Join(s.workingDir, ".claude", "agents")
	s.True(testutil.FileExists(s.T(), agentsDir))

	agentPath := filepath.Join(agentsDir, "code-reviewer.md")
	s.True(testutil.FileExists(s.T(), agentPath))

	agentContent := testutil.ReadFile(s.T(), agentPath)
	s.Contains(agentContent, "code-reviewer")
	s.Contains(agentContent, "Reviews code for quality")
}

func (s *GenerateCLITestSuite) TestGenerateWithTargets() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithTargets)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated")

	frontendPath := filepath.Join(s.workingDir, "frontend.md")
	s.True(testutil.FileExists(s.T(), frontendPath))
	frontendContent := testutil.ReadFile(s.T(), frontendPath)
	s.Contains(frontendContent, "Frontend Rule")
	s.Contains(frontendContent, "Universal Rule")
	s.NotContains(frontendContent, "Backend Rule")

	backendPath := filepath.Join(s.workingDir, "backend.md")
	s.True(testutil.FileExists(s.T(), backendPath))
	backendContent := testutil.ReadFile(s.T(), backendPath)
	s.Contains(backendContent, "Backend Rule")
	s.Contains(backendContent, "Universal Rule")
	s.NotContains(backendContent, "Frontend Rule")
}

func (s *GenerateCLITestSuite) TestGenerateWithoutConfig() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "configuration file")
}

func (s *GenerateCLITestSuite) TestGenerateWithInvalidConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidYAMLConfig)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "Error")
}

func (s *GenerateCLITestSuite) TestGenerateWithSchemaInvalidConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidSchemaConfig)

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")

	result.AssertStderrContains(s.T(), "validation failed")
}

func (s *GenerateCLITestSuite) TestGenerateVerboseOutput() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--verbose")

	result.AssertOutputContains(s.T(), "Generated")
}

func (s *GenerateCLITestSuite) TestGenerateQuietOutput() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--quiet")

	s.Empty(result.Stdout)
}

func (s *GenerateCLITestSuite) TestGenerateIdempotent() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result1 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result1.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, "CLAUDE.md")
	_ = testutil.ReadFile(s.T(), outputPath)

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	content2 := testutil.ReadFile(s.T(), outputPath)

	s.Contains(content2, "Test Project")
	s.Contains(content2, "Basic Rule")
}

func (s *GenerateCLITestSuite) TestGenerateDirectoryOutputs() {
	config := `metadata:
  name: "Directory Test"

outputs:
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "rules.mdc"

rules:
  - name: "Test Rule"
    content: "Test content"
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	result.AssertOutputContains(s.T(), "Generated")

	outputPath := filepath.Join(s.workingDir, ".cursor", "rules", "rules.mdc")
	s.True(testutil.FileExists(s.T(), outputPath))

	content := testutil.ReadFile(s.T(), outputPath)
	s.Contains(content, "Test Rule")
}
