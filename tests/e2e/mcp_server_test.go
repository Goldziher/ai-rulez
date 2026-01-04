package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type MCPServerE2ETestSuite struct {
	suite.Suite
	workingDir string
	client     *testutil.MCPClient
}

func TestMCPServerE2ESuite(t *testing.T) {
	suite.Run(t, new(MCPServerE2ETestSuite))
}

func (s *MCPServerE2ETestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)
	s.client = testutil.StartMCPServer(s.T(), s.workingDir)
}

func (s *MCPServerE2ETestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
	testutil.CleanupTestBinary()
}

func (s *MCPServerE2ETestSuite) TestGetVersion() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	response := s.client.CallTool(s.T(), "get_version", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
	s.NotEmpty(response.Result.Content, "Should have content")
	s.Contains(response.Result.Content[0].Text, "version")
}

func (s *MCPServerE2ETestSuite) TestRuleCRUD_FullCycle() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Create rule
	createParams := map[string]interface{}{
		"name":    "test-rule",
		"content": "# Test Rule\n\nThis is a test rule.",
	}
	createResponse := s.client.CallTool(s.T(), "create_rule", createParams)
	createResponse.AssertToolSuccess(s.T())
	s.NotEmpty(createResponse.Result.Content)
	s.Contains(createResponse.Result.Content[0].Text, "created")

	// List rules
	listResponse := s.client.CallTool(s.T(), "list_rules", map[string]interface{}{})
	listResponse.AssertToolSuccess(s.T())
	s.NotEmpty(listResponse.Result.Content)
	s.Contains(listResponse.Result.Content[0].Text, "test-rule")

	// Update rule
	updateParams := map[string]interface{}{
		"name":    "test-rule",
		"content": "# Test Rule Updated\n\nThis is an updated test rule.",
	}
	updateResponse := s.client.CallTool(s.T(), "update_rule", updateParams)
	updateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(updateResponse.Result.Content)

	// Delete rule
	deleteResponse := s.client.CallTool(s.T(), "delete_rule", map[string]interface{}{"name": "test-rule"})
	deleteResponse.AssertToolSuccess(s.T())
	s.NotEmpty(deleteResponse.Result.Content)
}

func (s *MCPServerE2ETestSuite) TestContextCRUD_FullCycle() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Create context
	createParams := map[string]interface{}{
		"name":    "test-context",
		"content": "# Test Context\n\nThis is a test context file.",
	}
	createResponse := s.client.CallTool(s.T(), "create_context", createParams)
	createResponse.AssertToolSuccess(s.T())
	s.NotEmpty(createResponse.Result.Content)
	s.Contains(createResponse.Result.Content[0].Text, "created")

	// List context
	listResponse := s.client.CallTool(s.T(), "list_context", map[string]interface{}{})
	listResponse.AssertToolSuccess(s.T())
	s.NotEmpty(listResponse.Result.Content)
	s.Contains(listResponse.Result.Content[0].Text, "test-context")

	// Update context
	updateParams := map[string]interface{}{
		"name":    "test-context",
		"content": "# Test Context Updated\n\nThis is an updated test context file.",
	}
	updateResponse := s.client.CallTool(s.T(), "update_context", updateParams)
	updateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(updateResponse.Result.Content)

	// Delete context
	deleteResponse := s.client.CallTool(s.T(), "delete_context", map[string]interface{}{"name": "test-context"})
	deleteResponse.AssertToolSuccess(s.T())
	s.NotEmpty(deleteResponse.Result.Content)
}

func (s *MCPServerE2ETestSuite) TestProfileCRUD_FullCycle() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Create a domain first
	createDomainParams := map[string]interface{}{
		"name": "test-domain",
	}
	createDomainResponse := s.client.CallTool(s.T(), "create_domain", createDomainParams)
	createDomainResponse.AssertToolSuccess(s.T())
	s.NotEmpty(createDomainResponse.Result.Content)

	// Create profile
	createProfileParams := map[string]interface{}{
		"name":    "test-profile",
		"domains": []string{"test-domain"},
	}
	createProfileResponse := s.client.CallTool(s.T(), "add_profile", createProfileParams)
	createProfileResponse.AssertToolSuccess(s.T())
	s.NotEmpty(createProfileResponse.Result.Content)

	// List profiles
	listProfilesResponse := s.client.CallTool(s.T(), "list_profiles", map[string]interface{}{})
	listProfilesResponse.AssertToolSuccess(s.T())
	s.NotEmpty(listProfilesResponse.Result.Content)
	s.Contains(listProfilesResponse.Result.Content[0].Text, "test-profile")

	// Set default profile
	setDefaultParams := map[string]interface{}{
		"name": "test-profile",
	}
	setDefaultResponse := s.client.CallTool(s.T(), "set_default_profile", setDefaultParams)
	setDefaultResponse.AssertToolSuccess(s.T())
	s.NotEmpty(setDefaultResponse.Result.Content)

	// Remove profile
	removeProfileParams := map[string]interface{}{
		"name": "test-profile",
	}
	removeProfileResponse := s.client.CallTool(s.T(), "remove_profile", removeProfileParams)
	removeProfileResponse.AssertToolSuccess(s.T())
	s.NotEmpty(removeProfileResponse.Result.Content)
}

func (s *MCPServerE2ETestSuite) TestSkillCRUD_FullCycle() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Create skill
	createParams := map[string]interface{}{
		"name":    "test-skill",
		"content": "# Test Skill\n\nThis is a test skill.",
	}
	createResponse := s.client.CallTool(s.T(), "create_skill", createParams)
	createResponse.AssertToolSuccess(s.T())
	s.NotEmpty(createResponse.Result.Content)
	s.Contains(createResponse.Result.Content[0].Text, "created")

	// List skills
	listResponse := s.client.CallTool(s.T(), "list_skills", map[string]interface{}{})
	listResponse.AssertToolSuccess(s.T())
	s.NotEmpty(listResponse.Result.Content)
	s.Contains(listResponse.Result.Content[0].Text, "test-skill")

	// Update skill
	updateParams := map[string]interface{}{
		"name":    "test-skill",
		"content": "# Test Skill Updated\n\nThis is an updated test skill.",
	}
	updateResponse := s.client.CallTool(s.T(), "update_skill", updateParams)
	updateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(updateResponse.Result.Content)

	// Delete skill
	deleteResponse := s.client.CallTool(s.T(), "delete_skill", map[string]interface{}{"name": "test-skill"})
	deleteResponse.AssertToolSuccess(s.T())
	s.NotEmpty(deleteResponse.Result.Content)
}

func (s *MCPServerE2ETestSuite) TestInitProject() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Remove existing V3 config
	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	os.RemoveAll(filepath.Join(s.workingDir, ".ai-rulez"))

	// Initialize project with V3 configuration
	params := map[string]interface{}{
		"project_name": "MCP-Initialized-Project",
		"providers":    []string{"claude"},
	}
	response := s.client.CallTool(s.T(), "init_project", params)
	response.AssertToolSuccess(s.T())
	s.NotEmpty(response.Result.Content)
	s.Contains(response.Result.Content[0].Text, "initialized")

	// Verify V3 config file exists at the correct location
	s.True(testutil.FileExists(s.T(), configPath), "Config file should exist at %s", configPath)

	// Verify config content
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "MCP-Initialized-Project")
	s.Contains(content, "version:")
	s.Contains(content, "presets:")
}

func (s *MCPServerE2ETestSuite) TestGenerateAndValidate() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Validate config
	validateResponse := s.client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	validateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(validateResponse.Result.Content)
	s.Contains(validateResponse.Result.Content[0].Text, "valid")

	// Generate outputs
	generateResponse := s.client.CallTool(s.T(), "generate_outputs", map[string]interface{}{})
	generateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(generateResponse.Result.Content)
	s.Contains(generateResponse.Result.Content[0].Text, "generated")
}
