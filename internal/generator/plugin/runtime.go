package plugin

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// RuntimeRenderer renders a single runtime's plugin bundle rooted at baseDir.
type RuntimeRenderer func(m *Manifest, baseDir string) ([]config.OutputFile, error)

// rendererFor returns the renderer of a runtime. It is a switch rather than a
// table filled from init() functions: the set of runtimes is fixed, and nothing
// is registered at start-up.
func rendererFor(runtime string) (RuntimeRenderer, bool) {
	switch runtime {
	case config.PluginRuntimeAgentPlugins:
		return renderAgentPlugins, true
	case config.PluginRuntimeClaude:
		return renderClaude, true
	case config.PluginRuntimeCodex:
		return renderCodex, true
	case config.PluginRuntimeCopilot:
		return renderCopilot, true
	case config.PluginRuntimeCursor:
		return renderCursor, true
	case config.PluginRuntimeFactory:
		return renderFactory, true
	case config.PluginRuntimeGemini:
		return renderGemini, true
	case config.PluginRuntimeHermes:
		return renderHermes, true
	case config.PluginRuntimeKimi:
		return renderKimi, true
	case config.PluginRuntimeOpenCode:
		return renderOpenCode, true
	case config.PluginRuntimePi:
		return renderPi, true
	}
	return nil, false
}

// Generate renders every requested runtime plus the single-plugin marketplace
// index, returning OutputFiles rooted at baseDir. Runtimes without a registered
// renderer are skipped with a warning.
func Generate(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	outputs, err := renderRuntimes(m, baseDir)
	if err != nil {
		return nil, err
	}

	if slices.Contains(m.Runtimes, config.PluginRuntimeClaude) {
		marketOutputs, err := renderMarketplace(m, baseDir)
		if err != nil {
			return nil, oops.Wrapf(err, "render marketplace")
		}
		outputs = append(outputs, marketOutputs...)
	}
	extraMarkets, err := renderSingleMarketplaces(m, baseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "render runtime marketplaces")
	}
	outputs = append(outputs, extraMarkets...)
	return AddProvenance(outputs, baseDir)
}

// GenerateMember renders a monorepo member's runtime bundles rooted at baseDir
// WITHOUT a marketplace index: in a monorepo only the root emits the aggregate
// marketplace, so members carry manifests and content only.
func GenerateMember(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	outputs, err := renderRuntimes(m, baseDir)
	if err != nil {
		return nil, err
	}
	return AddProvenance(outputs, baseDir)
}

// renderRuntimes runs every requested runtime renderer, rooted at baseDir, then
// de-duplicates outputs by path. De-duplication matters because runtimes that
// share the top-level content directories (Claude and Kimi both bundle root
// skills/) would otherwise emit the same file twice; the copies are byte
// identical, so keeping the first is safe.
func renderRuntimes(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	var kept map[string]bool // skills the Agent Plugins library packaged, when that runtime is requested
	for _, runtime := range m.Runtimes {
		renderer, ok := rendererFor(runtime)
		if !ok {
			m.log().Warn("no plugin renderer for runtime; skipping", "runtime", runtime)
			continue
		}
		outs, err := renderer(m, baseDir)
		if err != nil {
			return nil, oops.With("runtime", runtime).Wrapf(err, "render plugin runtime")
		}
		if runtime == config.PluginRuntimeAgentPlugins {
			kept = packagedSkills(outs, baseDir)
		}
		outputs = append(outputs, outs...)
	}
	if kept != nil {
		outputs = dropUnpackagedSkills(outputs, kept, baseDir)
	}
	outputs, err := composeNPMPackages(outputs, baseDir)
	if err != nil {
		return nil, err
	}
	return dedupeByPath(outputs), nil
}

// packagedSkills returns the names of the skills in an Agent Plugins bundle.
func packagedSkills(outs []config.OutputFile, baseDir string) map[string]bool {
	kept := map[string]bool{}
	for _, o := range outs {
		if name, rest, ok := rootSkillPath(o.Path, baseDir); ok && rest == "SKILL.md" {
			kept[name] = true
		}
	}
	return kept
}

// dropUnpackagedSkills removes the root skills/ files of every skill the Agent
// Plugins library dropped (name mismatch, missing description), so a runtime that
// shares skills/ cannot bring them back.
func dropUnpackagedSkills(outputs []config.OutputFile, kept map[string]bool, baseDir string) []config.OutputFile {
	out := outputs[:0:0]
	for _, o := range outputs {
		if name, _, ok := rootSkillPath(o.Path, baseDir); ok && !kept[name] {
			continue
		}
		out = append(out, o)
	}
	return out
}

// rootSkillPath splits baseDir/skills/<name>/<rest> into name and rest.
func rootSkillPath(p, baseDir string) (name, rest string, ok bool) {
	rel, err := filepath.Rel(baseDir, p)
	if err != nil {
		return "", "", false
	}
	after, found := strings.CutPrefix(filepath.ToSlash(rel), "skills/")
	if !found {
		return "", "", false
	}
	return strings.Cut(after, "/")
}

// dedupeByPath returns outputs with duplicate paths removed, keeping the first
// occurrence of each path.
func dedupeByPath(outputs []config.OutputFile) []config.OutputFile {
	seen := make(map[string]bool, len(outputs))
	deduped := make([]config.OutputFile, 0, len(outputs))
	for _, o := range outputs {
		if seen[o.Path] {
			continue
		}
		seen[o.Path] = true
		deduped = append(deduped, o)
	}
	return deduped
}

// Root variables each runtime substitutes for the plugin install directory.
const (
	rootVarCanonical = "${PLUGIN_ROOT}"
	rootVarClaude    = "${CLAUDE_PLUGIN_ROOT}"
	rootVarGemini    = "${extensionPath}"
)

// rewriteRoot rewrites the canonical ${PLUGIN_ROOT} launch variable to the form
// a given runtime expects. Claude/Gemini use their own named variables; Cursor
// Kimi, and Codex use plugin-relative paths ("./..."); OpenCode/Factory keep
// the canonical variable.
func rewriteRoot(s, runtime string) string {
	switch runtime {
	case config.PluginRuntimeClaude:
		return strings.ReplaceAll(s, rootVarCanonical, rootVarClaude)
	case config.PluginRuntimeGemini:
		return strings.ReplaceAll(s, rootVarCanonical, rootVarGemini)
	case config.PluginRuntimeCursor, config.PluginRuntimeKimi, config.PluginRuntimeCodex,
		config.PluginRuntimeAgentPlugins, config.PluginRuntimeCopilot:
		s = strings.ReplaceAll(s, rootVarCanonical+"/", "./")
		return strings.ReplaceAll(s, rootVarCanonical, ".")
	default:
		return s
	}
}

// jsonOutput marshals v to indented JSON with a trailing newline and returns an
// OutputFile with RawContent so the writer emits it verbatim (no header banner).
func jsonOutput(path string, v any) (config.OutputFile, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return config.OutputFile{}, oops.With("path", path).Wrapf(err, "marshal json")
	}
	data = append(data, '\n')
	return config.OutputFile{Path: path, RawContent: data}, nil
}

// mcpEntry is one entry in a runtime manifest's inline mcpServers map.
type mcpEntry struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// mcpServersFor builds the inline mcpServers map for a runtime. stdio servers
// carry a command (root variable rewritten per runtime); remote servers
// (http/sse) carry a type + URL instead — emitting a command-less stdio entry
// would be an invalid manifest. Returns nil when the plugin has no MCP servers.
func mcpServersFor(m *Manifest, runtime string) map[string]mcpEntry {
	if len(m.MCP) == 0 {
		return nil
	}
	out := make(map[string]mcpEntry, len(m.MCP))
	for _, s := range m.MCP {
		entry := mcpEntry{Env: s.Env, URL: s.URL}
		switch s.Transport {
		case config.TransportHTTP, config.TransportSSE:
			entry.Type = s.Transport
		default:
			entry.Command = rewriteRoot(s.Command, runtime)
			entry.Args = s.Args
		}
		out[s.Name] = entry
	}
	return out
}
