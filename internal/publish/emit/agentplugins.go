package emit

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

// AgentPluginsEmitter is the name of the Agent Plugins emitter.
const AgentPluginsEmitter = "agent-plugins"

const (
	apManifest = "plugin.json"
	apMCP      = "mcp.json"
	apSkills   = "skills"
)

// agentPlugins writes each plugin as one Agent Plugins directory
// (https://agent-plugins.org, specification 1.0.0 and 1.1.0): <name>/plugin.json,
// skills/, mcp.json and the extension namespaces, and nothing of the other
// runtimes the bundle carries. The package is read back with the library's
// importer and written again, so the bytes are the library's canonical ones and
// anything a conformant client would skip is reported instead of shipped.
//
// The option spec selects the version; without it the version the bundle's
// plugin.json declares is kept. The spec defines no archive, registry or
// signature: the directory travels in the publish archive, an npm package or an
// OCI artifact like the rest of the dist, and is signed with it.
type agentPlugins struct{}

func init() { register(agentPlugins{}) }

func (agentPlugins) Name() string   { return AgentPluginsEmitter }
func (agentPlugins) Status() string { return StatusVerified }

func (agentPlugins) Emit(in Input) ([]File, []Finding, error) {
	spec := in.Options["spec"]
	if spec != "" && !agentplugins.SupportedSpec(spec) {
		return nil, nil, oops.Hint("supported specs: "+strings.Join(agentplugins.Specs, ", ")).
			Errorf("agent-plugins: unsupported spec %q", spec)
	}
	var (
		files    []File
		findings []Finding
	)
	for i := range in.Plugins {
		p := &in.Plugins[i]
		if !hasFile(p.Files, apManifest) {
			continue
		}
		out, notes, err := packagePlugin(p, spec)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, out...)
		findings = append(findings, notes...)
	}
	if len(files) == 0 {
		return nil, nil, oops.Hint("publish with the agent-plugins runtime in [plugin] runtimes or --runtime").
			Errorf("agent-plugins: the bundle holds no plugin.json to package")
	}
	out, err := finish(files)
	return out, findings, err
}

// packageFile reports whether a bundle file belongs to the Agent Plugins package:
// the manifest, mcp.json, skills/, the evals/ tree and the extension namespace
// directories. Manifests of other runtimes are left behind.
func packageFile(p string) bool {
	first, _, nested := strings.Cut(p, "/")
	switch {
	case p == apManifest || p == apMCP:
		return true
	case !nested:
		return false
	case first == apSkills || first == "evals" || first == "bin":
		return true
	}
	return agentplugins.ValidNamespace(first)
}

func packagePlugin(p *Plugin, spec string) ([]File, []Finding, error) {
	mem := fstest.MapFS{}
	for _, f := range p.Files {
		if packageFile(f.Path) {
			mem[f.Path] = &fstest.MapFile{Data: f.Data}
		}
	}
	pkg, res := agentplugins.Import(mem)
	if res.Rejected {
		return nil, nil, oops.Errorf("agent-plugins: plugin %q is rejected: %s", p.Name, summarize(res.Findings, agentplugins.SeverityError))
	}
	built, buildFindings, err := agentplugins.Build(pkg, agentplugins.Options{Spec: cmp.Or(spec, res.Spec)})
	if err != nil {
		return nil, nil, oops.Wrapf(err, "agent-plugins: build plugin %q", p.Name)
	}
	var notes []Finding
	for _, f := range slices.Concat(res.Findings, buildFindings) {
		if f.Severity != agentplugins.SeverityInfo {
			notes = append(notes, Finding{Message: p.Name + ": " + f.Path + ": " + f.Message})
		}
	}
	out := make([]File, 0, len(built))
	for _, rel := range slices.Sorted(maps.Keys(built)) {
		out = append(out, File{Path: pkg.Metadata.Name + "/" + rel, Data: built[rel]})
	}
	return out, notes, nil
}

func summarize(fs []agentplugins.Finding, sev agentplugins.Severity) string {
	var parts []string
	for _, f := range fs {
		if f.Severity == sev {
			parts = append(parts, f.Path+": "+f.Message)
		}
	}
	return strings.Join(parts, "; ")
}

// AgentPluginEntry names one Agent Plugins package an emitter writes.
type AgentPluginEntry struct {
	Name    string
	Version string
	// Path is the package directory, relative to the emitter's output directory
	// (dist/emit/agent-plugins).
	Path string
}

// AgentPluginEntries lists the packages the agent-plugins emitter writes for in,
// sorted by name. It is the plugin list a registry catalog points at.
func AgentPluginEntries(in Input) []AgentPluginEntry {
	var out []AgentPluginEntry
	for i := range in.Plugins {
		p := &in.Plugins[i]
		if hasFile(p.Files, apManifest) {
			out = append(out, AgentPluginEntry{Name: p.Name, Version: p.Version, Path: p.Name})
		}
	}
	slices.SortFunc(out, func(a, b AgentPluginEntry) int { return cmp.Compare(a.Name, b.Name) })
	return out
}
