package plugin

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// rootVarCursorHook is Cursor's documented plugin-root variable for hook
// commands, with a shell fallback to the current directory.
const rootVarCursorHook = "${CURSOR_PLUGIN_ROOT:-.}"

// rewriteHookRoot rewrites the canonical ${PLUGIN_ROOT} in a hook command to the
// root variable a runtime resolves for hooks. This differs from rewriteRoot (used
// for MCP launch commands): hooks run in the runtime's hook context, where Cursor
// exposes ${CURSOR_PLUGIN_ROOT} rather than a plugin-relative path.
func rewriteHookRoot(s, runtime string) string {
	if runtime == config.PluginRuntimeCursor {
		return strings.ReplaceAll(s, rootVarCanonical, rootVarCursorHook)
	}
	return rewriteRoot(s, runtime)
}

// hookActionDoc is one action in a rendered hooks.json group. Async is emitted
// unconditionally (false must serialize) to match the runtime hook contract.
type hookActionDoc struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async"`
}

// hookGroupDoc is one matcher group under a lifecycle event.
type hookGroupDoc struct {
	Matcher string          `json:"matcher,omitempty"`
	Hooks   []hookActionDoc `json:"hooks"`
}

// hooksBlock builds the {event: [groups...]} map for a runtime, rewriting each
// command's ${PLUGIN_ROOT} to the runtime-specific root. Event order follows the
// first appearance of each event in the declared hook groups so output is stable.
// Returns nil when the plugin declares no hooks.
func hooksBlock(m *Manifest, runtime string) map[string][]hookGroupDoc {
	if len(m.Hooks) == 0 {
		return nil
	}
	block := make(map[string][]hookGroupDoc)
	for _, g := range m.Hooks {
		actions := make([]hookActionDoc, 0, len(g.Hooks))
		for _, a := range g.Hooks {
			typ := a.Type
			if typ == "" {
				typ = "command"
			}
			actions = append(actions, hookActionDoc{
				Type:    typ,
				Command: rewriteHookRoot(a.Command, runtime),
				Async:   a.Async,
			})
		}
		block[g.Event] = append(block[g.Event], hookGroupDoc{
			Matcher: g.Matcher,
			Hooks:   actions,
		})
	}
	return block
}

// hooksFileDoc wraps the hooks block for the standalone hooks.json file
// (Claude/Cursor/Codex) which nests everything under a top-level "hooks" key.
type hooksFileDoc struct {
	Hooks map[string][]hookGroupDoc `json:"hooks"`
}

// renderHooksFile emits a hooks.json OutputFile at path for the given runtime,
// or nothing when the plugin declares no hooks.
func renderHooksFile(m *Manifest, runtime, path string) ([]config.OutputFile, error) {
	block := hooksBlock(m, runtime)
	if block == nil {
		return nil, nil
	}
	out, err := jsonOutput(path, hooksFileDoc{Hooks: block})
	if err != nil {
		return nil, err
	}
	return []config.OutputFile{out}, nil
}
