package importer

import (
	"encoding/json"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
)

// Agent Plugins (https://agent-plugins.org): a directory with a root plugin.json,
// skills/<name>/, an optional mcp.json and client content in reverse-domain
// extension namespaces. The importer reads it with the same library that builds
// the package (internal/agentplugins), so what `generate --plugin` writes is read
// back unchanged: `export, import, export` gives identical bytes.

const (
	agentPluginsName     = "agent-plugins"
	agentPluginsManifest = "plugin.json"
	// namespaceCopilot is the namespace the Copilot runtime writes its agents to.
	namespaceCopilot = "com.github.copilot"
)

type agentPluginsImporter struct{}

func (agentPluginsImporter) Name() string { return agentPluginsName }

func (agentPluginsImporter) Description() string {
	return "Agent Plugins directory: plugin.json, skills/, mcp.json and extension namespaces (agent-plugins.org), mapped to a [plugin] block, skills and MCP servers"
}

// Detect recognizes a plugin.json that names an Agent Plugins schema. Claude,
// Cursor and Codex keep their manifests in dot directories, so a bare root
// plugin.json with this $schema is the standard's. A package ai-rulez generated
// into its own project is skipped: its sources are already there.
func (agentPluginsImporter) Detect(fsys fs.FS) []string {
	r := newReader(fsys)
	data, err := r.read(agentPluginsManifest)
	if err != nil {
		return nil
	}
	var doc struct {
		Schema string `json:"$schema"`
	}
	if json.Unmarshal(trimBOM(data), &doc) != nil || !strings.HasPrefix(doc.Schema, "https://agent-plugins.org/schemas/") {
		return nil
	}
	if _, own := r.exists(DefaultConfigDir); own {
		return nil
	}
	return []string{agentPluginsManifest}
}

func (agentPluginsImporter) Plan(fsys fs.FS, _ Options) (*Plan, error) {
	pkg, res := agentplugins.Import(fsys)
	if res.Rejected {
		return nil, oops.With("file", agentPluginsManifest).Errorf("%s is not an Agent Plugins manifest: %s (%s)",
			agentPluginsManifest, firstMessage(res.Findings), CodeInvalid)
	}
	b := &agentPluginsPlanner{p: &Plan{}, r: newReader(fsys), pkg: pkg, spec: res.Spec}
	b.findings(res.Findings)
	b.metadata()
	b.skills()
	b.mcp()
	b.extensions()
	b.files()
	b.r.flushProblems(b.p)
	return b.p, nil
}

func firstMessage(fs []agentplugins.Finding) string {
	for _, f := range fs {
		if f.Severity == agentplugins.SeverityError {
			return f.Message
		}
	}
	return "invalid manifest"
}

type agentPluginsPlanner struct {
	p    *Plan
	r    *reader
	pkg  *agentplugins.Plugin
	spec string
}

// findings reports what the library skipped or warned about: skipped content is
// dropped, a warning is an approximation.
func (b *agentPluginsPlanner) findings(fs []agentplugins.Finding) {
	for _, f := range fs {
		source, field, _ := strings.Cut(f.Path, "#")
		switch f.Severity {
		case agentplugins.SeverityError:
			b.p.add(newFinding(StatusDropped, source, field, "", f.Message))
		case agentplugins.SeverityWarning:
			b.p.add(newFinding(StatusApproximated, source, field, "", f.Message))
		case agentplugins.SeverityInfo:
		}
	}
}

// metadata maps plugin.json to a [plugin] block. The block needs a name, a
// version and a description to be valid configuration; when the manifest lacks
// one, nothing is written and the report says what to add.
func (b *agentPluginsPlanner) metadata() {
	m := b.pkg.Metadata
	var missing []string
	if m.Version == "" {
		missing = append(missing, "version")
	}
	if m.Description == "" {
		missing = append(missing, "description")
	}
	if len(missing) > 0 {
		b.p.add(newFinding(StatusNeedsAction, agentPluginsManifest, strings.Join(missing, ", "), "plugin",
			"a [plugin] block needs a version and a description, which this manifest lacks; add a [plugin] block to config.toml by hand to publish the content as a plugin"))
		return
	}
	pl := &config.PluginAuthoring{
		Name: m.Name, Version: m.Version, Description: m.Description,
		Homepage: m.Homepage, Repository: m.Repository, License: m.License, Keywords: m.Keywords,
		Runtimes: []string{config.PluginRuntimeAgentPlugins},
	}
	if a := m.Author; a != nil {
		pl.Author = &config.Author{Name: a.Name, Email: a.Email, URL: a.URL}
	}
	if b.spec != agentplugins.DefaultSpec {
		pl.Spec = b.spec
	}
	b.p.Plugin = pl
	b.p.add(newFinding(StatusMapped, agentPluginsManifest, "", "plugin", "plugin.json imported as the [plugin] block with runtimes = [\"agent-plugins\"]"))
}

// skills imports skills/<name>/ with its resources. A file the library read from
// a generated package carries ai-rulez's provenance header; it is removed so the
// source is what the package was generated from.
func (b *agentPluginsPlanner) skills() {
	for i := range b.pkg.Skills {
		sk := &b.pkg.Skills[i]
		dir := path.Join("skills", sk.Name)
		it := Item{
			Kind: KindSkill, Name: sk.Name, Sources: []string{dir},
			Main: plugin.StripProvenance(path.Join(dir, "SKILL.md"), sk.SkillMD),
		}
		for _, rel := range slices.Sorted(mapKeys(sk.Files)) {
			full := path.Join(dir, rel)
			it.Resources = append(it.Resources, File{
				Path: rel, Data: plugin.StripProvenance(full, sk.Files[rel]), Exec: b.r.executable(full),
			})
		}
		b.p.Items = append(b.p.Items, it)
	}
}

func mapKeys[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// mcp imports mcp.json servers as [[mcp_servers]], so they reach every preset
// and not only the plugin. Cwd has no equivalent there and is reported.
func (b *agentPluginsPlanner) mcp() {
	for i := range b.pkg.MCPServers {
		s := &b.pkg.MCPServers[i]
		srv := config.MCPServer{Name: s.Name, Command: s.Command, Args: s.Args, Env: s.Env, URL: s.URL, Headers: s.Headers}
		switch s.Transport {
		case agentplugins.TransportSSE:
			srv.Transport = config.TransportSSE
		case agentplugins.TransportHTTP, agentplugins.TransportStreamableHTTP:
			srv.Transport = config.TransportHTTP
		}
		field := "mcpServers." + s.Name
		if s.Cwd != "" {
			b.p.add(newFinding(StatusDropped, "mcp.json", field+".cwd", "", "cwd has no [[mcp_servers]] equivalent; start the server from a wrapper script instead"))
		}
		b.p.MCPServers = append(b.p.MCPServers, srv)
		b.p.add(newFinding(StatusMapped, "mcp.json", field, "mcp_servers."+s.Name, ""))
	}
}

// extensions imports what the namespaces ai-rulez writes (and Copilot's agents)
// and reports the rest: a client ignores a namespace it does not implement, and
// so does ai-rulez.
func (b *agentPluginsPlanner) extensions() {
	for i := range b.pkg.Extensions {
		e := &b.pkg.Extensions[i]
		switch e.Namespace {
		case agentplugins.NamespaceCodex:
			b.codexInterface(e)
		case agentplugins.NamespaceClaudeCode:
			b.namespaceFiles(e, map[string]Kind{"agents": KindAgent, "commands": KindCommand}, ".md")
		case agentplugins.NamespaceAIRulez:
			b.namespaceFiles(e, map[string]Kind{"rules": KindRule, "context": KindContext}, ".md")
		case namespaceCopilot:
			b.namespaceFiles(e, map[string]Kind{"agents": KindAgent}, ".agent.md")
		default:
			b.p.add(newFinding(StatusDropped, e.Namespace, "", "", "extension namespace has no ai-rulez equivalent"))
		}
	}
}

func (b *agentPluginsPlanner) codexInterface(e *agentplugins.Extension) {
	for key := range e.Manifest {
		if key != "interface" {
			b.p.add(newFinding(StatusDropped, agentPluginsManifest, "extensions."+e.Namespace+"."+key, "", "extension data has no ai-rulez equivalent"))
		}
	}
	doc, ok := e.Manifest["interface"].(map[string]any)
	if !ok || b.p.Plugin == nil {
		return
	}
	iface, err := plugin.InterfaceFromDoc(doc)
	if err != nil {
		b.p.add(newFinding(StatusDropped, agentPluginsManifest, "extensions."+e.Namespace+".interface", "", err.Error()))
		return
	}
	b.p.Plugin.Interface = iface
	b.p.add(newFinding(StatusMapped, agentPluginsManifest, "extensions."+e.Namespace+".interface", "plugin.interface", ""))
}

// namespaceFiles maps <dir>/<name><suffix> files of a namespace to content kinds
// and reports every other file.
func (b *agentPluginsPlanner) namespaceFiles(e *agentplugins.Extension, kinds map[string]Kind, suffix string) {
	for _, rel := range slices.Sorted(mapKeys(e.Files)) {
		source := path.Join(e.Namespace, rel)
		dir, file, nested := strings.Cut(rel, "/")
		kind, known := kinds[dir]
		name, hasSuffix := strings.CutSuffix(file, suffix)
		if !known || !nested || !hasSuffix || strings.Contains(name, "/") {
			b.p.add(newFinding(StatusDropped, source, "", "", "file has no ai-rulez equivalent"))
			continue
		}
		safe, _ := safeName(name)
		b.p.Items = append(b.p.Items, Item{Kind: kind, Name: safe, Sources: []string{source}, Main: e.Files[rel]})
	}
}

// files reports the package's other files. They are not copied: ai-rulez has no
// plugin-root file tree to put them in.
func (b *agentPluginsPlanner) files() {
	for _, rel := range slices.Sorted(mapKeys(b.pkg.Files)) {
		if rel == plugin.ProvenanceFileName {
			continue
		}
		reason := "file of the package root has no ai-rulez source; keep it next to the plugin and bundle it by hand"
		if strings.HasPrefix(rel, "evals/") {
			reason = "eval cases are not imported from a plugin; add them under the config directory's evals/"
		}
		b.p.add(newFinding(StatusDropped, rel, "", "", reason))
	}
	sort.SliceStable(b.p.Findings, func(i, j int) bool { return b.p.Findings[i].Source < b.p.Findings[j].Source })
}
