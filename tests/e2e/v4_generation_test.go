package e2e_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	_ "github.com/Goldziher/ai-rulez/internal/generator/presets" // register all presets
	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// V4GenerationSuite tests V4 config loading and generation for all presets.
// All tests are expected to FAIL initially (TDD red phase).
type V4GenerationSuite struct {
	suite.Suite
	workingDir string
	cfg        *config.ConfigV3
	content    *config.ContentTreeV3
	outputs    map[string][]config.OutputFileV3
}

func TestV4GenerationSuite(t *testing.T) {
	suite.Run(t, new(V4GenerationSuite))
}

func (s *V4GenerationSuite) SetupSuite() {
	t := s.T()
	s.workingDir = testutil.CreateTempDir(t)
	testutil.SetupV4FullConfig(t, s.workingDir)

	// Load V4 config (TOML support is the first thing that needs implementing)
	cfg, err := config.LoadConfigV3(context.Background(), s.workingDir)
	require.NoError(t, err, "LoadConfigV3 should support TOML config in V4")

	s.cfg = cfg

	// Verify V4-specific fields were loaded
	require.Equal(t, "4.0", cfg.Version, "Config version should be 4.0")
	require.Equal(t, "v4-test-project", cfg.Name)
	require.Len(t, cfg.Plugins, 3, "Should have 3 plugins configured")
	require.Len(t, cfg.Marketplaces, 2, "Should have 2 marketplaces configured")

	// Load content for default profile
	content, err := cfg.GetContentForProfile("full")
	require.NoError(t, err)
	s.content = content

	// Verify content was loaded
	require.NotEmpty(t, content.Rules, "Should have root rules")
	require.NotEmpty(t, content.Context, "Should have root context")
	require.NotEmpty(t, content.Skills, "Should have root skills")
	require.NotEmpty(t, content.Agents, "Should have root agents")
	require.NotEmpty(t, content.Commands, "Should have root commands")
	require.Contains(t, content.Domains, "backend", "Should have backend domain")
	require.Contains(t, content.Domains, "frontend", "Should have frontend domain")

	// Verify MCP servers were loaded from TOML
	require.NotEmpty(t, cfg.MCPServers, "Should have MCP servers from mcp.toml")
	require.Contains(t, cfg.MCPServers, "test-mcp-server")
	require.Contains(t, cfg.MCPServers, "http-mcp-server")

	// Generate all presets
	results, err := config.GeneratePresetsV3(cfg)
	require.NoError(t, err, "Should generate all presets without error")
	s.outputs = results
}

// --- Helper methods ---

func (s *V4GenerationSuite) getOutputs(preset string) []config.OutputFileV3 {
	outputs, ok := s.outputs[preset]
	s.Require().True(ok, "Expected outputs for preset %q", preset)
	return outputs
}

func (s *V4GenerationSuite) findFile(outputs []config.OutputFileV3, pathSuffix string) *config.OutputFileV3 {
	for i := range outputs {
		if strings.HasSuffix(outputs[i].Path, pathSuffix) && !outputs[i].IsDir {
			return &outputs[i]
		}
	}
	return nil
}

func (s *V4GenerationSuite) findDir(outputs []config.OutputFileV3, pathSuffix string) *config.OutputFileV3 {
	for i := range outputs {
		if strings.HasSuffix(outputs[i].Path, pathSuffix) && outputs[i].IsDir {
			return &outputs[i]
		}
	}
	return nil
}

func (s *V4GenerationSuite) requireFile(outputs []config.OutputFileV3, pathSuffix string) config.OutputFileV3 {
	f := s.findFile(outputs, pathSuffix)
	s.Require().NotNilf(f, "Expected file with suffix %q in outputs", pathSuffix)
	return *f
}

func (s *V4GenerationSuite) requireDir(outputs []config.OutputFileV3, pathSuffix string) {
	d := s.findDir(outputs, pathSuffix)
	s.Require().NotNilf(d, "Expected directory with suffix %q in outputs", pathSuffix)
}

func (s *V4GenerationSuite) assertContentContains(output config.OutputFileV3, expected string) {
	s.Assert().Contains(output.Content, expected, "File %s should contain %q", output.Path, expected)
}

// ==========================================
// CLAUDE PRESET
// ==========================================

func (s *V4GenerationSuite) TestClaude_FileStructure() {
	outputs := s.getOutputs("claude")

	// Directories
	s.requireDir(outputs, ".claude")
	s.requireDir(outputs, filepath.Join(".claude", "skills"))
	s.requireDir(outputs, filepath.Join(".claude", "agents"))

	// Main instructions file
	s.Require().NotNil(s.findFile(outputs, "CLAUDE.md"), "Should generate CLAUDE.md")

	// Skills (root + domain + commands-as-skills)
	s.Require().NotNil(s.findFile(outputs, filepath.Join("deployment-workflow", "SKILL.md")),
		"Should generate root skill file")
	s.Require().NotNil(s.findFile(outputs, filepath.Join("api-design", "SKILL.md")),
		"Should generate domain skill file")
	s.Require().NotNil(s.findFile(outputs, filepath.Join("run-tests", "SKILL.md")),
		"Should generate command as skill file")

	// Agents
	s.Require().NotNil(s.findFile(outputs, filepath.Join("agents", "security-reviewer.md")),
		"Should generate root agent file")
	s.Require().NotNil(s.findFile(outputs, filepath.Join("agents", "backend-architect.md")),
		"Should generate domain agent file")

	// MCP servers in settings.json (NEW)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".claude", "settings.json")),
		"Should generate .claude/settings.json with MCP servers")

	// Plugins (NEW)
	claudePluginFile := s.findFile(outputs, filepath.Join(".claude", "plugins.json"))
	s.Require().NotNil(claudePluginFile, "Should generate .claude/plugins.json with plugin declarations")
}

func (s *V4GenerationSuite) TestClaude_Content() {
	outputs := s.getOutputs("claude")

	// CLAUDE.md content
	claudeMD := s.requireFile(outputs, "CLAUDE.md")
	s.assertContentContains(claudeMD, "code-review-standards")
	s.assertContentContains(claudeMD, "documentation-standards")
	s.assertContentContains(claudeMD, "project-architecture")
	s.assertContentContains(claudeMD, "api-standards")
	s.assertContentContains(claudeMD, "backend-architecture")
	s.assertContentContains(claudeMD, "component-standards")
	s.assertContentContains(claudeMD, "AI-RULEZ :: GENERATED FILE")

	// Skill frontmatter
	skillFile := s.requireFile(outputs, filepath.Join("deployment-workflow", "SKILL.md"))
	s.assertContentContains(skillFile, "name: deployment-workflow")
	s.assertContentContains(skillFile, "description:")
	s.assertContentContains(skillFile, "Deployment Workflow")

	// Agent frontmatter
	agentFile := s.requireFile(outputs, filepath.Join("agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "name: security-reviewer")
	s.assertContentContains(agentFile, "description:")

	// MCP settings.json content (NEW)
	settingsFile := s.requireFile(outputs, filepath.Join(".claude", "settings.json"))
	var settingsJSON map[string]interface{}
	err := json.Unmarshal([]byte(settingsFile.Content), &settingsJSON)
	s.Require().NoError(err, "settings.json should be valid JSON")
	mcpServers, ok := settingsJSON["mcpServers"].(map[string]interface{})
	s.Require().True(ok, "settings.json should have mcpServers key")
	s.Assert().Contains(mcpServers, "test-mcp-server", "Should include test-mcp-server")
	s.Assert().Contains(mcpServers, "http-mcp-server", "Should include http-mcp-server")

	// Plugins content (NEW)
	pluginsFile := s.requireFile(outputs, filepath.Join(".claude", "plugins.json"))
	s.assertContentContains(pluginsFile, "github")
	s.assertContentContains(pluginsFile, "claude-plugins-official")
}

// ==========================================
// CURSOR PRESET
// ==========================================

func (s *V4GenerationSuite) TestCursor_FileStructure() {
	outputs := s.getOutputs("cursor")

	// Rule files (.mdc format)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".cursor", "rules", "code-review-standards.mdc")),
		"Should generate root rule as .mdc")
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".cursor", "rules", "api-standards.mdc")),
		"Should generate domain rule as .mdc")

	// Commands
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".cursor", "commands", "run-tests.md")),
		"Should generate command file")

	// Skills (in .agents/ directory)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (in .agents/ directory)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md")),
		"Should generate agent file")

	// MCP servers (shared .mcp.json)
	s.Require().NotNil(s.findFile(outputs, ".mcp.json"),
		"Should generate .mcp.json with MCP servers")
}

func (s *V4GenerationSuite) TestCursor_Content() {
	outputs := s.getOutputs("cursor")

	// Rule content in .mdc format
	ruleFile := s.requireFile(outputs, filepath.Join(".cursor", "rules", "code-review-standards.mdc"))
	s.assertContentContains(ruleFile, "code-review-standards")
	s.assertContentContains(ruleFile, "reviewed before merging")

	// Context should be rendered (NEW — currently missing for Cursor)
	// Context should appear in rules or a dedicated context file
	hasContext := false
	for _, output := range outputs {
		if strings.Contains(output.Content, "hexagonal architecture") {
			hasContext = true
			break
		}
	}
	s.Assert().True(hasContext, "Cursor output should include context content (project architecture)")

	// Command metadata
	cmdFile := s.requireFile(outputs, filepath.Join(".cursor", "commands", "run-tests.md"))
	s.assertContentContains(cmdFile, "Run the full test suite")

	// Skill frontmatter
	skillFile := s.requireFile(outputs, filepath.Join(".agents", "skills", "deployment-workflow", "SKILL.md"))
	s.assertContentContains(skillFile, "name: deployment-workflow")

	// Agent frontmatter
	agentFile := s.requireFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// WINDSURF PRESET
// ==========================================

func (s *V4GenerationSuite) TestWindsurf_FileStructure() {
	outputs := s.getOutputs("windsurf")

	// Rule files
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".windsurf", "rules", "code-review-standards.md")),
		"Should generate root rule")
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".windsurf", "rules", "api-standards.md")),
		"Should generate domain rule")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".windsurf", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (NEW — currently not rendered for Windsurf)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".windsurf", "agents", "security-reviewer.md")),
		"Should generate agent file (NEW)")
}

func (s *V4GenerationSuite) TestWindsurf_Content() {
	outputs := s.getOutputs("windsurf")

	// Rule content
	ruleFile := s.requireFile(outputs, filepath.Join(".windsurf", "rules", "code-review-standards.md"))
	s.assertContentContains(ruleFile, "code-review-standards")

	// Context should be rendered (NEW — currently missing for Windsurf)
	hasContext := false
	for _, output := range outputs {
		if strings.Contains(output.Content, "hexagonal architecture") {
			hasContext = true
			break
		}
	}
	s.Assert().True(hasContext, "Windsurf output should include context content")

	// Skill frontmatter
	skillFile := s.requireFile(outputs, filepath.Join(".windsurf", "skills", "deployment-workflow", "SKILL.md"))
	s.assertContentContains(skillFile, "name: deployment-workflow")

	// Agent frontmatter (NEW)
	agentFile := s.requireFile(outputs, filepath.Join(".windsurf", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// COPILOT PRESET
// ==========================================

func (s *V4GenerationSuite) TestCopilot_FileStructure() {
	outputs := s.getOutputs("copilot")

	// Main instructions
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".github", "copilot-instructions.md")),
		"Should generate copilot-instructions.md")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".github", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (.agent.md extension)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".github", "agents", "security-reviewer.agent.md")),
		"Should generate agent with .agent.md extension")
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".github", "agents", "backend-architect.agent.md")),
		"Should generate domain agent with .agent.md extension")

	// MCP servers (shared .mcp.json)
	s.Require().NotNil(s.findFile(outputs, ".mcp.json"),
		"Should generate .mcp.json with MCP servers")

	// Commands (NEW — currently not rendered for Copilot)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".github", "commands", "run-tests.md")),
		"Should generate command file (NEW)")
}

func (s *V4GenerationSuite) TestCopilot_Content() {
	outputs := s.getOutputs("copilot")

	// Instructions content
	instructions := s.requireFile(outputs, filepath.Join(".github", "copilot-instructions.md"))
	s.assertContentContains(instructions, "code-review-standards")
	s.assertContentContains(instructions, "project-architecture")
	s.assertContentContains(instructions, "api-standards")

	// Agent frontmatter with Copilot-specific fields
	agentFile := s.requireFile(outputs, filepath.Join(".github", "agents", "security-reviewer.agent.md"))
	s.assertContentContains(agentFile, "description:")
	s.assertContentContains(agentFile, "tools:")
}

// ==========================================
// GEMINI PRESET
// ==========================================

func (s *V4GenerationSuite) TestGemini_FileStructure() {
	outputs := s.getOutputs("gemini")

	// Main instructions
	s.Require().NotNil(s.findFile(outputs, "GEMINI.md"), "Should generate GEMINI.md")

	// MCP settings from user config (NEW — currently hardcoded)
	settingsFile := s.findFile(outputs, filepath.Join(".gemini", "settings.json"))
	s.Require().NotNil(settingsFile, "Should generate .gemini/settings.json")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md")),
		"Should generate agent file")
}

func (s *V4GenerationSuite) TestGemini_Content() {
	outputs := s.getOutputs("gemini")

	// GEMINI.md content
	geminiMD := s.requireFile(outputs, "GEMINI.md")
	s.assertContentContains(geminiMD, "code-review-standards")
	s.assertContentContains(geminiMD, "project-architecture")

	// Settings.json should include user-configured MCP servers, not just ai-rulez (NEW)
	settingsFile := s.requireFile(outputs, filepath.Join(".gemini", "settings.json"))
	var settingsJSON map[string]interface{}
	err := json.Unmarshal([]byte(settingsFile.Content), &settingsJSON)
	s.Require().NoError(err, "settings.json should be valid JSON")
	mcpServers, ok := settingsJSON["mcpServers"].(map[string]interface{})
	s.Require().True(ok, "settings.json should have mcpServers key")
	s.Assert().Contains(mcpServers, "test-mcp-server",
		"Should include user-configured MCP servers, not just hardcoded ai-rulez")
}

// ==========================================
// CLINE PRESET
// ==========================================

func (s *V4GenerationSuite) TestCline_FileStructure() {
	outputs := s.getOutputs("cline")

	// Rule files
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".clinerules", "code-review-standards.md")),
		"Should generate root rule")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".cline", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (NEW — currently not rendered for Cline)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".cline", "agents", "security-reviewer.md")),
		"Should generate agent file (NEW)")
}

func (s *V4GenerationSuite) TestCline_Content() {
	outputs := s.getOutputs("cline")

	// Rule content
	ruleFile := s.requireFile(outputs, filepath.Join(".clinerules", "code-review-standards.md"))
	s.assertContentContains(ruleFile, "code-review-standards")

	// Context should be rendered (NEW — currently missing for Cline)
	hasContext := false
	for _, output := range outputs {
		if strings.Contains(output.Content, "hexagonal architecture") {
			hasContext = true
			break
		}
	}
	s.Assert().True(hasContext, "Cline output should include context content")

	// Agent file (NEW)
	agentFile := s.requireFile(outputs, filepath.Join(".cline", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// JUNIE PRESET
// ==========================================

func (s *V4GenerationSuite) TestJunie_FileStructure() {
	outputs := s.getOutputs("junie")

	// Main guidelines
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".junie", "guidelines.md")),
		"Should generate guidelines.md")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".junie", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".junie", "agents", "security-reviewer.md")),
		"Should generate agent file")
}

func (s *V4GenerationSuite) TestJunie_Content() {
	outputs := s.getOutputs("junie")

	// Guidelines content
	guidelines := s.requireFile(outputs, filepath.Join(".junie", "guidelines.md"))
	s.assertContentContains(guidelines, "code-review-standards")
	s.assertContentContains(guidelines, "project-architecture")

	// Agent frontmatter
	agentFile := s.requireFile(outputs, filepath.Join(".junie", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// CONTINUE.DEV PRESET
// ==========================================

func (s *V4GenerationSuite) TestContinueDev_FileStructure() {
	outputs := s.getOutputs("continue-dev")

	// Rule files
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".continue", "rules", "code-review-standards.md")),
		"Should generate root rule")

	// Prompts YAML (context + skills + commands)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".continue", "prompts", "ai_rulez_prompts.yaml")),
		"Should generate prompts.yaml")

	// Agents (NEW — currently not rendered for Continue.dev)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".continue", "agents", "security-reviewer.md")),
		"Should generate agent file (NEW)")
}

func (s *V4GenerationSuite) TestContinueDev_Content() {
	outputs := s.getOutputs("continue-dev")

	// Rule content
	ruleFile := s.requireFile(outputs, filepath.Join(".continue", "rules", "code-review-standards.md"))
	s.assertContentContains(ruleFile, "code-review-standards")

	// Prompts YAML should include context, skills, and commands
	promptsFile := s.requireFile(outputs, filepath.Join(".continue", "prompts", "ai_rulez_prompts.yaml"))
	s.assertContentContains(promptsFile, "deployment-workflow")
	s.assertContentContains(promptsFile, "run-tests")
}

// ==========================================
// CODEX PRESET
// ==========================================

func (s *V4GenerationSuite) TestCodex_FileStructure() {
	outputs := s.getOutputs("codex")

	// Main AGENTS.md
	s.Require().NotNil(s.findFile(outputs, "AGENTS.md"), "Should generate AGENTS.md")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".codex", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (TOML format)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".codex", "agents", "security-reviewer.toml")),
		"Should generate agent in TOML format")

	// Plugins (NEW — Codex plugin declarations)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".codex", "plugins.json")),
		"Should generate Codex plugin declarations (NEW)")

	// Commands (NEW — currently not rendered for Codex)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".codex", "commands", "run-tests.md")),
		"Should generate command file (NEW)")
}

func (s *V4GenerationSuite) TestCodex_Content() {
	outputs := s.getOutputs("codex")

	// AGENTS.md content
	agentsMD := s.requireFile(outputs, "AGENTS.md")
	s.assertContentContains(agentsMD, "code-review-standards")
	s.assertContentContains(agentsMD, "project-architecture")

	// Agent TOML content
	agentFile := s.requireFile(outputs, filepath.Join(".codex", "agents", "security-reviewer.toml"))
	s.assertContentContains(agentFile, "name")
	s.assertContentContains(agentFile, "description")
	s.assertContentContains(agentFile, "security vulnerabilities")

	// Plugins content (NEW)
	pluginsFile := s.requireFile(outputs, filepath.Join(".codex", "plugins.json"))
	s.assertContentContains(pluginsFile, "gmail")
	s.assertContentContains(pluginsFile, "openai-curated")
}

// ==========================================
// OPENCODE PRESET
// ==========================================

func (s *V4GenerationSuite) TestOpencode_FileStructure() {
	outputs := s.getOutputs("opencode")

	// Main AGENTS.md
	s.Require().NotNil(s.findFile(outputs, "AGENTS.md"), "Should generate AGENTS.md")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".opencode", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".opencode", "agents", "security-reviewer.md")),
		"Should generate agent file")
}

func (s *V4GenerationSuite) TestOpencode_Content() {
	outputs := s.getOutputs("opencode")

	// AGENTS.md content
	agentsMD := s.requireFile(outputs, "AGENTS.md")
	s.assertContentContains(agentsMD, "code-review-standards")
	s.assertContentContains(agentsMD, "project-architecture")

	// Agent frontmatter
	agentFile := s.requireFile(outputs, filepath.Join(".opencode", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// AMP PRESET
// ==========================================

func (s *V4GenerationSuite) TestAmp_FileStructure() {
	outputs := s.getOutputs("amp")

	// Main AGENTS.md
	s.Require().NotNil(s.findFile(outputs, "AGENTS.md"), "Should generate AGENTS.md")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents (NEW — currently not rendered for Amp)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md")),
		"Should generate agent file (NEW)")
}

func (s *V4GenerationSuite) TestAmp_Content() {
	outputs := s.getOutputs("amp")

	// AGENTS.md content
	agentsMD := s.requireFile(outputs, "AGENTS.md")
	s.assertContentContains(agentsMD, "code-review-standards")
	s.assertContentContains(agentsMD, "project-architecture")

	// Agent file content (NEW)
	agentFile := s.requireFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md"))
	s.assertContentContains(agentFile, "description:")
}

// ==========================================
// ANTIGRAVITY PRESET
// ==========================================

func (s *V4GenerationSuite) TestAntigravity_FileStructure() {
	outputs := s.getOutputs("antigravity")

	// Main GEMINI.md
	s.Require().NotNil(s.findFile(outputs, "GEMINI.md"), "Should generate GEMINI.md")

	// MCP settings from user config (NEW — currently hardcoded)
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "settings.json")),
		"Should generate .agents/settings.json")

	// Skills
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "skills", "deployment-workflow", "SKILL.md")),
		"Should generate skill file")

	// Agents
	s.Require().NotNil(s.findFile(outputs, filepath.Join(".agents", "agents", "security-reviewer.md")),
		"Should generate agent file")
}

func (s *V4GenerationSuite) TestAntigravity_Content() {
	outputs := s.getOutputs("antigravity")

	// GEMINI.md content
	geminiMD := s.requireFile(outputs, "GEMINI.md")
	s.assertContentContains(geminiMD, "code-review-standards")

	// Settings.json should include user-configured MCP servers (NEW — currently hardcoded)
	settingsFile := s.requireFile(outputs, filepath.Join(".agents", "settings.json"))
	var settingsJSON map[string]interface{}
	err := json.Unmarshal([]byte(settingsFile.Content), &settingsJSON)
	s.Require().NoError(err, "settings.json should be valid JSON")
	mcpServers, ok := settingsJSON["mcpServers"].(map[string]interface{})
	s.Require().True(ok, "settings.json should have mcpServers key")
	s.Assert().Contains(mcpServers, "test-mcp-server",
		"Should include user-configured MCP servers, not just hardcoded ai-rulez")
}

// ==========================================
// MCP (STANDALONE) PRESET
// ==========================================

func (s *V4GenerationSuite) TestMCP_FileStructure() {
	outputs := s.getOutputs("mcp")

	s.Require().NotNil(s.findFile(outputs, ".mcp.json"), "Should generate .mcp.json")
}

func (s *V4GenerationSuite) TestMCP_Content() {
	outputs := s.getOutputs("mcp")

	mcpFile := s.requireFile(outputs, ".mcp.json")

	var mcpJSON map[string]interface{}
	err := json.Unmarshal([]byte(mcpFile.Content), &mcpJSON)
	s.Require().NoError(err, ".mcp.json should be valid JSON")

	mcpServers, ok := mcpJSON["mcpServers"].(map[string]interface{})
	s.Require().True(ok, ".mcp.json should have mcpServers key")

	// test-mcp-server
	testServer, ok := mcpServers["test-mcp-server"].(map[string]interface{})
	s.Require().True(ok, "Should have test-mcp-server entry")
	s.Assert().Equal("npx", testServer["command"])
	args, ok := testServer["args"].([]interface{})
	s.Require().True(ok)
	s.Assert().Contains(args, "-y")
	s.Assert().Contains(args, "@test/mcp-server")

	// http-mcp-server with transport and url
	httpServer, ok := mcpServers["http-mcp-server"].(map[string]interface{})
	s.Require().True(ok, "Should have http-mcp-server entry")
	s.Assert().Equal("http", httpServer["transport"])
	s.Assert().Equal("http://localhost:8080", httpServer["url"])
}

// ==========================================
// PLUGIN & MARKETPLACE ASSERTIONS
// ==========================================

func (s *V4GenerationSuite) TestPlugins_LoadedCorrectly() {
	require.Len(s.T(), s.cfg.Plugins, 3)

	// Claude plugin
	s.Assert().Equal("claude-plugins-official", s.cfg.Plugins[0].Marketplace)
	s.Assert().Equal("github", s.cfg.Plugins[0].Name)
	s.Assert().Equal("project", s.cfg.Plugins[0].GetScope())
	s.Assert().True(s.cfg.Plugins[0].IsEnabled())

	// Codex plugin
	s.Assert().Equal("openai-curated", s.cfg.Plugins[1].Marketplace)
	s.Assert().Equal("gmail", s.cfg.Plugins[1].Name)
	s.Assert().Equal("user", s.cfg.Plugins[1].GetScope())

	// Custom marketplace plugin
	s.Assert().Equal("my-org/team-plugins", s.cfg.Plugins[2].Marketplace)
	s.Assert().Equal("internal-tool", s.cfg.Plugins[2].Name)
}

func (s *V4GenerationSuite) TestMarketplaces_LoadedCorrectly() {
	require.Len(s.T(), s.cfg.Marketplaces, 2)

	s.Assert().Equal("team-tools", s.cfg.Marketplaces[0].Name)
	s.Assert().Equal("my-org/claude-plugins", s.cfg.Marketplaces[0].Source)
	s.Assert().Equal("github", s.cfg.Marketplaces[0].Type)

	s.Assert().Equal("local-dev", s.cfg.Marketplaces[1].Name)
	s.Assert().Equal("./plugins", s.cfg.Marketplaces[1].Source)
	s.Assert().Equal("local", s.cfg.Marketplaces[1].Type)
}

// ==========================================
// CROSS-CUTTING CONCERNS
// ==========================================

func (s *V4GenerationSuite) TestAllPresets_Generated() {
	expectedPresets := []string{
		"claude", "cursor", "windsurf", "copilot", "gemini",
		"cline", "junie", "continue-dev", "codex", "opencode",
		"amp", "antigravity", "mcp",
	}

	for _, preset := range expectedPresets {
		_, ok := s.outputs[preset]
		s.Assert().True(ok, "Should have outputs for preset %q", preset)
	}
}

func (s *V4GenerationSuite) TestV4Config_VersionIs4() {
	s.Assert().Equal("4.0", s.cfg.Version)
}

func (s *V4GenerationSuite) TestV4Config_TOMLFormat() {
	// The config was loaded from a .toml file — verify it was parsed correctly
	s.Assert().Equal("v4-test-project", s.cfg.Name)
	s.Assert().Equal("Full V4 test configuration with all presets", s.cfg.Description)
	s.Assert().Equal("compact", s.cfg.GetHeaderStyle())
	s.Assert().Equal("full", s.cfg.GetDefaultProfile())
	s.Assert().Len(s.cfg.Presets, 13, "Should have 13 presets configured")

	// Profiles
	s.Assert().Contains(s.cfg.Profiles, "full")
	s.Assert().Contains(s.cfg.Profiles, "backend")
	s.Assert().Equal([]string{"backend", "frontend"}, s.cfg.Profiles["full"])
}

// TestV4Config_MCPFromTOML verifies MCP servers are loaded from mcp.toml
func (s *V4GenerationSuite) TestV4Config_MCPFromTOML() {
	testServer, ok := s.cfg.MCPServers["test-mcp-server"]
	s.Require().True(ok, "Should have test-mcp-server from mcp.toml")
	s.Assert().Equal("npx", testServer.Command)
	s.Assert().Equal([]string{"-y", "@test/mcp-server"}, testServer.Args)
	s.Assert().Equal("stdio", testServer.GetTransport())
	s.Assert().Equal("test-key", testServer.Env["API_KEY"])

	httpServer, ok := s.cfg.MCPServers["http-mcp-server"]
	s.Require().True(ok, "Should have http-mcp-server from mcp.toml")
	s.Assert().Equal("http", httpServer.GetTransport())
	s.Assert().Equal("http://localhost:8080", httpServer.URL)
}

// Standalone assertions for unused import avoidance
var (
	_ = assert.Equal
	_ = filepath.Join
)
