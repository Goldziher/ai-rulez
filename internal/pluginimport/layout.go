package pluginimport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// layout describes where one harness keeps the parts of a plugin.
type layout struct {
	format   string
	harness  string
	manifest string
	skills   []string
	agents   []string
	commands []string
	rules    []string
	// hooks and mcp are default files, read when the manifest names none.
	hooks []string
	mcp   []string
	// context names a manifest field holding the instruction file(s) of the extension.
	context string
	// tomlCommands marks a layout whose commands are TOML files (Gemini).
	tomlCommands bool
}

// Directory and file names shared across the layouts.
const (
	dirSkills     = "skills"
	dirAgents     = "agents"
	dirCommands   = "commands"
	fileHooksJSON = "hooks/hooks.json"
	fileMCPJSON   = ".mcp.json"
)

// layouts lists the supported plugin layouts in detection order. An agent
// plugin has no entry: it is read by the Agent Plugins importer.
func layouts() []layout {
	return []layout{
		{format: config.PluginFormatClaude, harness: config.HarnessClaude, manifest: ".claude-plugin/plugin.json",
			skills: []string{dirSkills}, agents: []string{dirAgents}, commands: []string{dirCommands},
			hooks: []string{fileHooksJSON}, mcp: []string{fileMCPJSON}},
		{format: config.PluginFormatCodex, harness: config.HarnessCodex, manifest: ".codex-plugin/plugin.json",
			skills: []string{dirSkills}, hooks: []string{"hooks.json", fileHooksJSON}, mcp: []string{fileMCPJSON}},
		{format: config.PluginFormatCursor, harness: config.HarnessCursor, manifest: ".cursor-plugin/plugin.json",
			skills: []string{dirSkills}, agents: []string{dirAgents}, commands: []string{dirCommands}, rules: []string{"rules"},
			hooks: []string{fileHooksJSON}, mcp: []string{fileMCPJSON, "mcp.json"}},
		{format: config.PluginFormatAgentPlugins, harness: config.HarnessClaude, manifest: "plugin.json",
			skills: []string{dirSkills}, agents: []string{"com.anthropic.claude-code/agents"},
			commands: []string{"com.anthropic.claude-code/commands"}, rules: []string{"io.github.goldziher.ai-rulez/rules"},
			hooks: []string{"com.anthropic.claude-code/hooks/hooks.json"}, mcp: []string{"mcp.json"}},
		{format: config.PluginFormatGemini, harness: config.HarnessGemini, manifest: "gemini-extension.json",
			skills: []string{dirSkills}, agents: []string{dirAgents}, commands: []string{dirCommands}, tomlCommands: true,
			hooks: []string{fileHooksJSON}, context: "contextFileName"},
	}
}

func layoutFor(format string) (layout, bool) {
	all := layouts()
	for i := range all {
		if all[i].format == format {
			return all[i], true
		}
	}
	return layout{}, false
}

// detect picks the layout of a plugin directory: the first whose manifest exists;
// a directory with Claude's default component directories and no manifest is a
// Claude plugin, as Claude Code itself treats it.
func detect(dir string) (format string, found bool) {
	all := layouts()
	for i := range all {
		if all[i].format == config.PluginFormatAgentPlugins {
			continue // its plugin.json is told apart by its $schema below
		}
		if info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(all[i].manifest))); err == nil && info.Mode().IsRegular() {
			return all[i].format, true
		}
	}
	if isAgentPlugin(dir) {
		return config.PluginFormatAgentPlugins, true
	}
	for _, d := range []string{dirSkills, dirAgents, dirCommands, "hooks"} {
		if info, err := os.Lstat(filepath.Join(dir, d)); err == nil && info.IsDir() {
			return config.PluginFormatClaude, true
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, fileMCPJSON)); err == nil && info.Mode().IsRegular() {
		return config.PluginFormatClaude, true
	}
	return "", false
}

// manifestDoc is the decoded manifest of a plugin.
type manifestDoc struct {
	Name, Version, Description string
	Fields                     map[string]json.RawMessage
}

// readManifest reads the manifest of a layout; a missing manifest is empty, a
// malformed one an error.
func readManifest(dir string, l layout) (manifestDoc, error) {
	file := filepath.Join(dir, filepath.FromSlash(l.manifest))
	data, err := readRegular(file, MaxManifestBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifestDoc{Fields: map[string]json.RawMessage{}}, nil
		}
		return manifestDoc{}, err
	}
	doc := manifestDoc{Fields: map[string]json.RawMessage{}}
	if err := json.Unmarshal(data, &doc.Fields); err != nil {
		return manifestDoc{}, oops.With("file", l.manifest).Wrapf(err, "%s is not valid JSON", l.manifest)
	}
	doc.Name = stringField(doc.Fields, "name")
	doc.Version = stringField(doc.Fields, "version")
	doc.Description = stringField(doc.Fields, "description")
	return doc, nil
}

func stringField(fields map[string]json.RawMessage, key string) string {
	var s string
	if raw, ok := fields[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// stringList reads a manifest field that is a string or a list of strings.
func stringList(raw json.RawMessage) ([]string, bool) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, true
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many, true
	}
	return nil, false
}
