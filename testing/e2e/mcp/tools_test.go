package mcp

import (
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type MCPToolsTestSuite struct {
	suite.Suite
	workingDir string
	client     *testutil.MCPClient
}

func TestMCPToolsSuite(t *testing.T) {
	suite.Run(t, new(MCPToolsTestSuite))
}

func (s *MCPToolsTestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)
	s.client = testutil.StartMCPServer(s.T(), s.workingDir)
}

func (s *MCPToolsTestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
}

func (s *MCPToolsTestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

// ========== Version and Info Tools ==========

func (s *MCPToolsTestSuite) TestGetVersion() {
	response := s.client.CallTool(s.T(), "get_version", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	version := response.GetResultString(s.T(), "version")
	s.NotEmpty(version)
}

// ========== Rules Tools ==========

func (s *MCPToolsTestSuite) TestGetRules() {
	response := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	rules, ok := result["rules"].([]interface{})
	s.True(ok, "Rules should be an array")
	s.Len(rules, 2, "Should have 2 rules from basic config")

	firstRule, ok := rules[0].(map[string]interface{})
	s.True(ok, "Rule should be an object")
	s.Contains(firstRule, "Name")
	s.Contains(firstRule, "Content")
	s.Contains(firstRule, "Priority")
}

func (s *MCPToolsTestSuite) TestGetRulesWithFilter() {
	response := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{
		"name_filter": "high priority",
	})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	rules, ok := result["rules"].([]interface{})
	s.True(ok)
	s.Len(rules, 1, "Should filter to only matching rules")
}

func (s *MCPToolsTestSuite) TestAddRule() {
	response := s.client.CallTool(s.T(), "add_rule", map[string]interface{}{
		"name":     "New MCP Rule",
		"content":  "New MCP rule content",
		"priority": 8,
	})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	s.Contains(result, "message")

	getRulesResponse := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	getRulesResponse.AssertToolSuccess(s.T())

	getRulesResult := getRulesResponse.GetParsedResult(s.T())
	rules, _ := getRulesResult["rules"].([]interface{})
	s.Len(rules, 3, "Should now have 3 rules")
}

func (s *MCPToolsTestSuite) TestAddRuleWithAllParameters() {
	response := s.client.CallTool(s.T(), "add_rule", map[string]interface{}{
		"name":     "Complete Rule",
		"content":  "Complete rule content",
		"section":  "Test Section",
		"priority": 7,
	})
	response.AssertToolSuccess(s.T())

	getRulesResponse := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	getRulesResponse.AssertToolSuccess(s.T())

	getRulesResult := getRulesResponse.GetParsedResult(s.T())
	rules, _ := getRulesResult["rules"].([]interface{})

	// Find the new rule
	var newRule map[string]interface{}
	for _, r := range rules {
		rule, _ := r.(map[string]interface{})
		if rule["Content"] == "Complete rule content" {
			newRule = rule
			break
		}
	}

	s.NotNil(newRule, "New rule should be found")
	s.Equal("Complete Rule", newRule["Name"])
	s.Equal(float64(7), newRule["Priority"])
}

func (s *MCPToolsTestSuite) TestUpdateRule() {
	response := s.client.CallTool(s.T(), "update_rule", map[string]interface{}{
		"name":     "Basic Rule",
		"content":  "Updated rule content",
		"priority": 9,
	})
	response.AssertToolSuccess(s.T())

	getRulesResponse := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	getRulesResponse.AssertToolSuccess(s.T())

	getRulesResult := getRulesResponse.GetParsedResult(s.T())
	rules, _ := getRulesResult["rules"].([]interface{})
	firstRule, _ := rules[0].(map[string]interface{})
	s.Equal("Updated rule content", firstRule["Content"])
	s.Equal(float64(9), firstRule["Priority"])
}

func (s *MCPToolsTestSuite) TestDeleteRule() {
	response := s.client.CallTool(s.T(), "delete_rule", map[string]interface{}{
		"name": "Basic Rule",
	})
	response.AssertToolSuccess(s.T())

	getRulesResponse := s.client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	getRulesResponse.AssertToolSuccess(s.T())

	getRulesResult := getRulesResponse.GetParsedResult(s.T())
	rules, _ := getRulesResult["rules"].([]interface{})
	s.Len(rules, 1, "Should now have 1 rule")
}

// ========== Sections Tools ==========

func (s *MCPToolsTestSuite) TestGetSections() {
	response := s.client.CallTool(s.T(), "get_sections", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	sections, ok := result["sections"].([]interface{})
	s.True(ok, "Sections should be an array")
	s.Len(sections, 1, "Should have 1 section from basic config")
}

func (s *MCPToolsTestSuite) TestAddSection() {
	response := s.client.CallTool(s.T(), "add_section", map[string]interface{}{
		"name":     "New MCP Section",
		"content":  "New section content via MCP",
		"priority": 6,
	})
	response.AssertToolSuccess(s.T())

	getSectionsResponse := s.client.CallTool(s.T(), "get_sections", map[string]interface{}{})
	getSectionsResponse.AssertToolSuccess(s.T())

	getSectionsResult := getSectionsResponse.GetParsedResult(s.T())
	sections, _ := getSectionsResult["sections"].([]interface{})
	s.Len(sections, 2, "Should now have 2 sections")
}

func (s *MCPToolsTestSuite) TestUpdateSection() {
	response := s.client.CallTool(s.T(), "update_section", map[string]interface{}{
		"name":     "Development Guidelines",
		"new_name": "Updated Section Title",
		"content":  "Updated section content",
	})
	response.AssertToolSuccess(s.T())

	getSectionsResponse := s.client.CallTool(s.T(), "get_sections", map[string]interface{}{})
	getSectionsResponse.AssertToolSuccess(s.T())

	getSectionsResult := getSectionsResponse.GetParsedResult(s.T())
	sections, _ := getSectionsResult["sections"].([]interface{})
	firstSection, _ := sections[0].(map[string]interface{})
	s.Equal("Updated Section Title", firstSection["Name"])
}

func (s *MCPToolsTestSuite) TestDeleteSection() {
	response := s.client.CallTool(s.T(), "delete_section", map[string]interface{}{
		"name": "Development Guidelines",
	})
	response.AssertToolSuccess(s.T())

	getSectionsResponse := s.client.CallTool(s.T(), "get_sections", map[string]interface{}{})
	getSectionsResponse.AssertToolSuccess(s.T())

	getSectionsResult := getSectionsResponse.GetParsedResult(s.T())
	sections, _ := getSectionsResult["sections"].([]interface{})
	s.Len(sections, 0, "Should now have 0 sections")
}

// ========== Agents Tools ==========

func (s *MCPToolsTestSuite) TestGetAgents() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.ConfigWithAgents)

	response := s.client.CallTool(s.T(), "get_agents", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	agents, ok := result["agents"].([]interface{})
	s.True(ok, "Agents should be an array")
	s.Len(agents, 1, "Should have 1 agent")

	firstAgent, _ := agents[0].(map[string]interface{})
	s.Equal("code-reviewer", firstAgent["Name"])
}

func (s *MCPToolsTestSuite) TestAddAgent() {
	response := s.client.CallTool(s.T(), "add_agent", map[string]interface{}{
		"name":        "test-agent",
		"description": "Test agent via MCP",
		"priority":    7,
		"tools":       []string{"Read", "Edit"},
	})
	response.AssertToolSuccess(s.T())

	getAgentsResponse := s.client.CallTool(s.T(), "get_agents", map[string]interface{}{})
	getAgentsResponse.AssertToolSuccess(s.T())

	getAgentsResult := getAgentsResponse.GetParsedResult(s.T())
	agents, _ := getAgentsResult["agents"].([]interface{})
	s.Len(agents, 1, "Should now have 1 agent")
}

// ========== Outputs Tools ==========

func (s *MCPToolsTestSuite) TestGetOutputs() {
	response := s.client.CallTool(s.T(), "get_outputs", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	outputs, ok := result["outputs"].([]interface{})
	s.True(ok, "Outputs should be an array")
	s.Len(outputs, 1, "Should have 1 output from basic config")
}

func (s *MCPToolsTestSuite) TestAddOutput() {
	response := s.client.CallTool(s.T(), "add_output", map[string]interface{}{
		"path": "NEW_OUTPUT.md",
	})
	response.AssertToolSuccess(s.T())

	getOutputsResponse := s.client.CallTool(s.T(), "get_outputs", map[string]interface{}{})
	getOutputsResponse.AssertToolSuccess(s.T())

	getOutputsResult := getOutputsResponse.GetParsedResult(s.T())
	outputs, _ := getOutputsResult["outputs"].([]interface{})
	s.Len(outputs, 2, "Should now have 2 outputs")
}

// ========== Utility Tools ==========

func (s *MCPToolsTestSuite) TestValidateConfig() {
	response := s.client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	isValid, ok := result["valid"].(bool)
	s.True(ok, "Should return valid flag")
	s.True(isValid, "Basic config should be valid")
}

func (s *MCPToolsTestSuite) TestValidateInvalidConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidSchemaConfig)

	response := s.client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	isValid, ok := result["valid"].(bool)
	s.True(ok, "Should return valid flag")
	s.False(isValid, "Invalid config should return false")

	if errorMsg, exists := result["error"]; exists {
		s.NotEmpty(errorMsg, "Should have error message for invalid config")
	} else {
		s.False(isValid, "Invalid config should return false")
	}
}

func (s *MCPToolsTestSuite) TestGenerateOutput() {
	response := s.client.CallTool(s.T(), "generate_output", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	result := response.GetParsedResult(s.T())
	results, ok := result["results"].([]interface{})
	s.True(ok, "Should return results list")
	s.Greater(len(results), 0, "Should generate at least one file")

	s.True(testutil.FileExists(s.T(), s.workingDir+"/CLAUDE.md"))
}

func (s *MCPToolsTestSuite) TestInitProject() {
	emptyDir := testutil.CreateTempDir(s.T())

	emptyClient := testutil.StartMCPServer(s.T(), emptyDir)
	defer emptyClient.Close()

	response := emptyClient.CallTool(s.T(), "init_project", map[string]interface{}{
		"project_name": "MCP Test Project",
		"providers":    []string{"claude", "cursor"},
	})
	response.AssertToolSuccess(s.T())

	s.True(testutil.FileExists(s.T(), emptyDir+"/ai_rulez.yaml"))

	content := testutil.ReadFile(s.T(), emptyDir+"/ai_rulez.yaml")
	s.Contains(content, "MCP Test Project")
}

// ========== Error Cases ==========

func (s *MCPToolsTestSuite) TestInvalidToolParameters() {
	response := s.client.CallTool(s.T(), "update_rule", map[string]interface{}{
		"name":    "nonexistent_rule",
		"content": "test",
	})
	response.AssertToolError(s.T(), "not found")

	response = s.client.CallTool(s.T(), "delete_rule", map[string]interface{}{
		"name": "nonexistent_rule",
	})
	response.AssertToolError(s.T(), "not found")
}
