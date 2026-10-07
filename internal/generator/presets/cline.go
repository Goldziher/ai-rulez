package presets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"gopkg.in/yaml.v3"
)

const presetNameCline = "cline"

var clineRulesTarget = rulefiles.Target{
	Preset: presetNameCline, Dir: ".clinerules", Ext: extMarkdown, Dialect: rulefiles.DialectCline, Banner: true,
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{ alwaysFileLocalRules }

func (g *ClinePresetGenerator) GetName() string {
	return presetNameCline
}

func (g *ClinePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".clinerules"),
		filepath.Join(baseDir, ".cline"),
		filepath.Join(baseDir, ".cline", "skills"),
		filepath.Join(baseDir, ".cline", "agents"),
		filepath.Join(baseDir, ".clinerules", "workflows"),
	}
}

// clineWorkflows is the folder of Cline workflows, its custom slash commands. A
// workflow is a plain markdown file invoked as /{id}.md.
var clineWorkflows = commandFilesSpec{preset: presetNameCline, dir: ".clinerules/workflows", ext: extMarkdown, noFrontmatter: true}

// ProjectLayout is where the preset writes project-level files; user scope maps them
// onto GlobalOutputPaths.
func (g *ClinePresetGenerator) ProjectLayout() ProjectLayout {
	return ProjectLayout{RulesDir: ".clinerules", SkillsDir: ".cline/skills", AgentsDir: ".cline/agents", CommandsDir: ".clinerules/workflows"}
}

// GlobalOutputPaths is the Cline user-scope layout: rules and workflows under
// ~/Documents/Cline, skills and agents under ~/.cline. Cline has no project MCP
// file (its servers live in the extension's global settings).
func (g *ClinePresetGenerator) GlobalOutputPaths(home string, getenv func(string) string) *GlobalPaths {
	hooks := map[string]string{}
	for _, event := range settings.ClineHookEvents() {
		hooks[settings.ClineHooksDir+"/"+event] = "Documents/Cline/Hooks/" + event
	}
	return GlobalLayout{
		RulesDir:    "Documents/Cline/Rules",
		SkillsDir:   ".cline/skills",
		AgentsDir:   ".cline/agents",
		CommandsDir: "Documents/Cline/Workflows",
		Sidecars:    hooks,
	}.Resolve(home, getenv)
}

func (g *ClinePresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	// Create directory structure
	outputs = append(outputs,
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline"),
			IsDir: true,
		},
		config.OutputFile{
			Path:  filepath.Join(baseDir, ".cline", "skills"),
			IsDir: true,
		},
	)

	// Rules and context are written as native rule files with frontmatter
	ruleOutputs, err := rulesFolderOutputs(clineRulesTarget, content, baseDir, cfg, routingWithSharedAgentsMD(cfg, presetNameCline, rulefiles.RoutingEverything), nil)
	if err != nil {
		return nil, fmt.Errorf("generate rule files: %w", err)
	}
	outputs = append(outputs, rulesDirMarker(cfg, presetNameCline, filepath.Join(baseDir, ".clinerules"), ruleOutputs)...)
	outputs = append(outputs, ruleOutputs...)

	outputs = append(outputs, g.hooksOutputs(cfg, baseDir)...)

	// Generate skill files to .cline/skills/
	allSkills := allSkills(content)
	for idx := range allSkills {
		skill := allSkills[idx]
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".cline", "skills", skillID)
		outputs = append(outputs,
			config.OutputFile{
				Path:  skillDir,
				IsDir: true,
			},
			config.OutputFile{
				Path:    filepath.Join(skillDir, "SKILL.md"),
				Content: g.renderSkillFile(skill),
			},
		)
		outputs = append(outputs, SkillResourceOutputs(&skill, skillDir)...)
	}

	// Commands are workflows: .clinerules/workflows/{id}.md.
	workflows, err := commandFileOutputs(content, baseDir, clineWorkflows)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, workflows...)

	// Add .cline/agents directory
	outputs = append(outputs, config.OutputFile{
		Path:  filepath.Join(baseDir, ".cline", "agents"),
		IsDir: true,
	})

	// Generate agent files to .cline/agents/
	allAgents := allAgents(content)
	for idx := range allAgents {
		agent := allAgents[idx]
		agentID := sanitizeAgentID(agent.Name)
		agentContent, err := g.renderClineAgentFile(agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("generate agent %s: %w", agent.Name, err)
		}

		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, ".cline", "agents", agentID+".yaml"),
			Content: agentContent,
		})
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Cline
func (g *ClinePresetGenerator) renderSkillFile(skill config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(yamlScalar(skill.Name))
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n")
	writeSkillSpecFields(&builder, skill, nil)
	builder.WriteString("---\n\n")
	builder.WriteString(skill.Content)
	builder.WriteString(RenderSkillResourcesIndex(&skill))

	return builder.String()
}

// renderClineAgentFile renders an agent file with YAML frontmatter for Cline
func (g *ClinePresetGenerator) renderClineAgentFile(agent config.ContentFile, cfg *config.Config) (string, error) {
	var builder strings.Builder

	frontmatter := g.buildClineAgentFrontmatter(agent, cfg)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("marshal agent frontmatter: %w", err)
	}

	builder.WriteString("---\n")
	builder.Write(yamlData)
	builder.WriteString("---\n\n")
	builder.WriteString(agent.Content)

	return builder.String(), nil
}

// buildClineAgentFrontmatter builds frontmatter for a Cline agent file
func (g *ClinePresetGenerator) buildClineAgentFrontmatter(agent config.ContentFile, cfg *config.Config) map[string]interface{} {
	frontmatter := map[string]interface{}{
		keyName: agent.Name,
	}

	// Resolve model via the shared resolver before the metadata-nil short-circuit so a
	// defaults-only model still applies to agents with no frontmatter.
	if model := ResolveNativeAgentModel(presetNameCline, agent, cfg); model != "" {
		frontmatter["modelId"] = model
	}
	// Cline requires a description and falls back to "<name> subagent" itself.
	frontmatter[keyDescription] = agent.Name + " subagent"

	if agent.Metadata == nil {
		return frontmatter
	}

	clineScalarFields := []string{keyDescription}
	for _, field := range clineScalarFields {
		if val, ok := agent.Metadata.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
	if EmitAgentField(cfg, "tools") {
		if tools := clineTools(agent.Metadata.Tools); len(tools) > 0 {
			frontmatter["tools"] = tools
		}
	}

	return frontmatter
}

// yamlScalar renders s as a YAML scalar, quoted only when plain would change its
// meaning (a colon, a hash, a leading indicator).
func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return quoteYAMLString(s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

// clineToolNames maps the tool names assistants commonly list to Cline's own
// vocabulary (ClineDefaultTool in the Cline source). Cline throws on a name it does
// not know, which skips the whole agent, so anything unmapped is dropped.
var clineToolNames = map[string]string{
	"read": "read_file", "write": "write_to_file", "edit": clineReplaceInFile, "multiedit": clineReplaceInFile,
	"bash": "execute_command", "grep": "search_files", "glob": clineListFiles, "ls": clineListFiles,
	"webfetch": "web_fetch", "websearch": "web_search", "task": "use_subagents", "skill": "use_skill",
	"askuserquestion": "ask_followup_question",
}

var clineNativeTools = map[string]bool{
	"ask_followup_question": true, "attempt_completion": true, "execute_command": true, clineReplaceInFile: true,
	"read_file": true, "write_to_file": true, "search_files": true, clineListFiles: true,
	"list_code_definition_names": true, "browser_action": true, "use_mcp_tool": true, "access_mcp_resource": true,
	"load_mcp_documentation": true, "new_task": true, "plan_mode_respond": true, "act_mode_respond": true,
	"focus_chain": true, "web_fetch": true, "web_search": true, "condense": true, "summarize_task": true,
	"report_bug": true, "new_rule": true, "apply_patch": true, "use_skill": true, "use_subagents": true,
}

// clineTools translates tool names into Cline's vocabulary, keeping order and
// dropping duplicates and names Cline has no equivalent for.
func clineTools(tools []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tool := range tools {
		tool = strings.TrimSpace(tool)
		name := tool
		if !clineNativeTools[name] {
			mapped, ok := clineToolNames[strings.ToLower(tool)]
			if !ok {
				continue
			}
			name = mapped
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}
