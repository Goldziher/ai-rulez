package plugin

import (
	"encoding/json"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// renderAgentPlugins emits the portable Agent Plugins package: a root
// plugin.json, the fixed skills/ directory, and (when configured) mcp.json.
// Commands, agents, hooks, and marketplaces are outside Agent Plugins v1.
func renderAgentPlugins(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	return agentPluginsCore(m, baseDir)
}

// agentPluginsExtensions is the client extension data a bundle carries. Only the
// Codex interface block is written, and only when the Codex root layout is active.
func agentPluginsExtensions(m *Manifest) ([]agentplugins.Extension, error) {
	if !codexRootLayout(m) {
		return nil, nil
	}
	iface := buildInterface(m.Interface)
	if iface == nil {
		return nil, nil
	}
	// Round-trip through JSON so the data is a plain map: its keys are then
	// written sorted, the way an import of the package reads them back.
	raw, err := json.Marshal(map[string]any{"interface": iface})
	if err != nil {
		return nil, oops.Wrapf(err, "encode the codex interface")
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, oops.Wrapf(err, "decode the codex interface")
	}
	return []agentplugins.Extension{{Namespace: agentplugins.NamespaceCodex, Manifest: data}}, nil
}

// agentPluginsMCPServers maps the plugin's MCP servers onto the library model.
// The library rewrites a ${PLUGIN_ROOT}-rooted command to the plugin-relative
// form the standard requires and refuses any other placeholder.
func agentPluginsMCPServers(m *Manifest) []agentplugins.MCPServer {
	servers := make([]agentplugins.MCPServer, 0, len(m.MCP))
	for _, s := range m.MCP {
		server := agentplugins.MCPServer{
			Name: s.Name, Command: s.Command, Args: s.Args, Env: s.Env,
			Transport: s.Transport, URL: s.URL,
		}
		if s.Disabled {
			off := false
			server.Enabled = &off
		}
		servers = append(servers, server)
	}
	return servers
}

// agentPluginsModel assembles the library Plugin from the manifest and the
// verbatim content outputs (skills, and eval cases when bundled).
func agentPluginsModel(m *Manifest, content []config.OutputFile, baseDir string) (*agentplugins.Plugin, error) {
	exts, err := agentPluginsExtensions(m)
	if err != nil {
		return nil, err
	}
	p := &agentplugins.Plugin{
		Metadata: agentplugins.Metadata{
			Name: m.Name, Version: m.Version, Description: m.Description,
			Homepage: m.Homepage, Repository: m.Repository, License: m.License, Keywords: m.Keywords,
		},
		MCPServers: agentPluginsMCPServers(m),
		Extensions: exts,
		Files:      map[string][]byte{},
	}
	if a := m.Author; a != nil {
		p.Metadata.Author = &agentplugins.Author{Name: a.Name, Email: a.Email, URL: a.URL}
	}
	skills := map[string]*agentplugins.Skill{}
	for _, o := range content {
		rel, err := filepath.Rel(baseDir, o.Path)
		if err != nil {
			return nil, oops.With("path", o.Path).Wrapf(err, "resolve bundle path")
		}
		rel = filepath.ToSlash(rel)
		data := outputBytes(o)
		name, inSkill, ok := strings.Cut(strings.TrimPrefix(rel, "skills/"), "/")
		if !strings.HasPrefix(rel, "skills/") || !ok {
			p.Files[rel] = data
			continue
		}
		sk := skills[name]
		if sk == nil {
			sk = &agentplugins.Skill{Name: name, Files: map[string][]byte{}}
			skills[name] = sk
		}
		if inSkill == "SKILL.md" {
			sk.SkillMD = data
			continue
		}
		sk.Files[inSkill] = data
	}
	for _, name := range slices.Sorted(maps.Keys(skills)) {
		if skills[name].SkillMD != nil {
			p.Skills = append(p.Skills, *skills[name])
		}
	}
	return p, nil
}

// agentPluginsBuild builds the package of m through the library, which is the one
// implementation of the standard: it validates plugin.json and mcp.json against
// the vendored official schemas and drops the skills and servers a conformant
// client would skip, reporting each as a finding. The content outputs carry the
// file modes of the sources; the returned map is keyed by slash path.
func agentPluginsBuild(m *Manifest, baseDir string) (files map[string][]byte, content []config.OutputFile, findings []agentplugins.Finding, err error) {
	content, err = bundleContent(m, baseDir, contentLayout{Skills: true})
	if err != nil {
		return nil, nil, nil, err
	}
	model, err := agentPluginsModel(m, content, baseDir)
	if err != nil {
		return nil, nil, nil, err
	}
	files, findings, err = agentplugins.Build(model, agentplugins.Options{Spec: m.Spec})
	if err != nil {
		return nil, nil, nil, oops.With("plugin", m.Name).Wrapf(err, "build the Agent Plugins package")
	}
	return files, content, findings, nil
}

// AgentPluginFindings builds the Agent Plugins package of m in memory and
// returns what the library reports, then validates the built package the way a
// conformant client loads it. baseDir only anchors the content paths.
func AgentPluginFindings(m *Manifest, baseDir string) ([]agentplugins.Finding, error) {
	files, _, findings, err := agentPluginsBuild(m, baseDir)
	if err != nil {
		return nil, err
	}
	mem := fstest.MapFS{}
	for rel, data := range files {
		mem[rel] = &fstest.MapFile{Data: data}
	}
	findings = append(findings, agentplugins.Validate(mem).Findings...)
	return findings, nil
}

// AgentPluginEntry names a built Agent Plugins package: what a registry entry
// (an ARD catalog) needs to point at it.
type AgentPluginEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Path is the package directory relative to the project root, slash separated.
	Path string `json:"path"`
}

// AgentPluginEntryFor describes the package a manifest builds at baseDir. rootDir
// is the project root the path is relative to.
func AgentPluginEntryFor(m *Manifest, baseDir, rootDir string) AgentPluginEntry {
	rel, err := filepath.Rel(rootDir, baseDir)
	if err != nil {
		rel = baseDir
	}
	return AgentPluginEntry{Name: m.Name, Version: m.Version, Path: path.Clean(filepath.ToSlash(rel))}
}

// agentPluginsCore writes the root plugin.json, skills/ and mcp.json from the
// library's output. Findings are logged: the dropped content is also reported by
// `validate --strict` and `publish`, which fail on it.
func agentPluginsCore(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	files, content, findings, err := agentPluginsBuild(m, baseDir)
	if err != nil {
		return nil, err
	}
	for _, f := range findings {
		if f.Severity != agentplugins.SeverityInfo {
			m.log().Warn("Agent Plugins: "+f.Message, "plugin", m.Name, "code", f.Code, "path", f.Path, "severity", string(f.Severity))
		}
	}
	byPath := make(map[string]config.OutputFile, len(content))
	for _, o := range content {
		if rel, err := filepath.Rel(baseDir, o.Path); err == nil {
			byPath[filepath.ToSlash(rel)] = o
		}
	}
	outputs := make([]config.OutputFile, 0, len(files))
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		// A passthrough file keeps its mode (a skill script stays executable).
		if src, ok := byPath[rel]; ok && string(outputBytes(src)) == string(files[rel]) {
			outputs = append(outputs, src)
			continue
		}
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(baseDir, filepath.FromSlash(rel)), RawContent: files[rel]})
	}
	return outputs, nil
}
