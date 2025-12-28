package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type InitCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestInitCLISuite(t *testing.T) {
	suite.Run(t, new(InitCLITestSuite))
}

func (s *InitCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *InitCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *InitCLITestSuite) TestInitSetupHooks() {
	lefthookContent := `pre-commit:
  commands:
    lint:
      run: npm run lint
`
	testutil.WriteFile(s.T(), s.workingDir, "lefthook.yml", lefthookContent)

	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "HookProject", "--setup-hooks", "--yes")

	result.AssertStderrContains(s.T(), "Successfully configured Lefthook")

	modifiedContent := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "lefthook.yml"))
	s.Contains(modifiedContent, "ai-rulez validate", "The validate command should be added to lefthook.yml")
}

func (s *InitCLITestSuite) TestInitWithFormat() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "FormatProject", "--format", "yaml", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")

	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "FormatProject")
}

func (s *InitCLITestSuite) TestBasicInit() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "TestProject", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")
	result.AssertStderrContains(s.T(), "TestProject")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")

	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "TestProject")
	s.Contains(content, "version: \"3.0\"")
}

func (s *InitCLITestSuite) TestInitWithoutProjectName() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "name:")
	s.Contains(content, "version: \"3.0\"")
}

func (s *InitCLITestSuite) TestInitWithDomains() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "DomainProject", "--domains", "frontend,backend", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")

	// Check that domains directories were created
	frontendPath := filepath.Join(s.workingDir, ".ai-rulez", "domains", "frontend")
	backendPath := filepath.Join(s.workingDir, ".ai-rulez", "domains", "backend")
	s.True(testutil.FileExists(s.T(), frontendPath), "Frontend domain directory should exist")
	s.True(testutil.FileExists(s.T(), backendPath), "Backend domain directory should exist")
}

func (s *InitCLITestSuite) TestInitSkipContent() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "SkipContentProject", "--skip-content", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")
}

func (s *InitCLITestSuite) TestInitSkipMCP() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "SkipMCPProject", "--skip-mcp", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")
}

func (s *InitCLITestSuite) TestInitExistingConfig() {
	// V3 uses .ai-rulez/ directory instead of ai-rulez.yaml file
	aiRulesDir := filepath.Join(s.workingDir, ".ai-rulez")
	os.MkdirAll(aiRulesDir, 0o755)

	// Don't use NO_INTERACTIVE because it allows overwriting existing config
	// Instead, we rely on test environment's non-interactive nature to trigger failure
	result := testutil.RunCLI(s.T(), s.workingDir, "init", "TestProject")

	// In non-interactive mode without explicit approval, init should fail
	s.NotEqual(0, result.ExitCode, "init should fail when .ai-rulez/ already exists in non-interactive mode")
}

func (s *InitCLITestSuite) TestInitWithJsonFormat() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "JSONProject", "--format", "json", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.json")
	s.True(testutil.FileExists(s.T(), configPath), "JSON config file should be created")
}

func (s *InitCLITestSuite) TestInitMultipleDomains() {
	result := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{
		"NO_INTERACTIVE": "1",
	}, "init", "MultiDomainProject", "--domains", "api,web,mobile", "--yes")

	result.AssertStderrContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath))

	// Check that all domain directories were created
	apiPath := filepath.Join(s.workingDir, ".ai-rulez", "domains", "api")
	webPath := filepath.Join(s.workingDir, ".ai-rulez", "domains", "web")
	mobilePath := filepath.Join(s.workingDir, ".ai-rulez", "domains", "mobile")
	s.True(testutil.FileExists(s.T(), apiPath), "API domain directory should exist")
	s.True(testutil.FileExists(s.T(), webPath), "Web domain directory should exist")
	s.True(testutil.FileExists(s.T(), mobilePath), "Mobile domain directory should exist")
}
