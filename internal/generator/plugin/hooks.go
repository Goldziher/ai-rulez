package plugin

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

const (
	// rootVarCursorHook is Cursor's documented plugin-root variable for hook
	// commands, with a shell fallback to the current directory.
	rootVarCursorHook = "${CURSOR_PLUGIN_ROOT:-.}"

	// hooksDirName is the bundle directory that holds both hooks.json and every
	// passed-through hook script. Scripts live next to hooks.json so the rendered
	// command is ${PLUGIN_ROOT}/hooks/<basename> for every runtime.
	hooksDirName = "hooks"
)

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
// unconditionally (false must serialize) to match the runtime hook contract;
// every other optional handler field is omitted when unset so a runtime never
// sees an empty args list or a zero timeout it would treat as a real value.
type hookActionDoc struct {
	Type          string   `json:"type"`
	Command       string   `json:"command"`
	Args          []string `json:"args,omitempty"`
	Timeout       int      `json:"timeout,omitempty"`
	Async         bool     `json:"async"`
	If            string   `json:"if,omitempty"`
	StatusMessage string   `json:"statusMessage,omitempty"`
}

// hookGroupDoc is one matcher group under a lifecycle event.
type hookGroupDoc struct {
	Matcher string          `json:"matcher,omitempty"`
	Hooks   []hookActionDoc `json:"hooks"`
}

// hookCommand resolves the command a runtime should spawn for an action. A
// declared Script is addressed through the bundle (the script is copied to
// hooks/<basename> by bundleHookScripts); a declared Command is passed through
// verbatim. Both go through the runtime's root rewrite. Slash-joined on purpose:
// the result is a command string a runtime interprets, not a host filesystem path.
func hookCommand(action *config.HookAction, runtime string) string {
	command := action.Command
	if action.Script != "" {
		command = rootVarCanonical + "/" + path.Join(hooksDirName, path.Base(filepath.ToSlash(action.Script)))
	}
	return rewriteHookRoot(command, runtime)
}

// hooksBlock builds the {event: [groups...]} map for a runtime, rewriting each
// command's ${PLUGIN_ROOT} to the runtime-specific root. Event order follows the
// first appearance of each event in the declared hook groups so output is stable.
// Returns nil when the plugin declares no hooks.
//
// This renders declarations only. Callers that write a hooks/ directory must also
// call bundleHookScripts, or a command pointing at hooks/<script> will dangle.
func hooksBlock(m *Manifest, runtime string) map[string][]hookGroupDoc {
	if len(m.Hooks) == 0 {
		return nil
	}
	block := make(map[string][]hookGroupDoc)
	for _, g := range m.Hooks {
		actions := make([]hookActionDoc, 0, len(g.Hooks))
		for i := range g.Hooks {
			action := &g.Hooks[i]
			typ := action.Type
			if typ == "" {
				typ = config.HookTypeCommand
			}
			actions = append(actions, hookActionDoc{
				Type:          typ,
				Command:       hookCommand(action, runtime),
				Args:          action.Args,
				Timeout:       action.Timeout,
				Async:         action.Async,
				If:            action.If,
				StatusMessage: action.StatusMessage,
			})
		}
		block[g.Event] = append(block[g.Event], hookGroupDoc{
			Matcher: g.Matcher,
			Hooks:   actions,
		})
	}
	return block
}

// bundleHookScripts copies every declared hook script into hooksDir, preserving
// the executable bit. Bundling the script (rather than only referencing a path)
// is what lets a hook run in a checkout that has never been generated: the
// project's generated outputs are gitignored, so a fresh clone or a new git
// worktree contains nothing but the plugin bundle itself.
//
// Scripts are flattened to their basename, so two different sources sharing a
// basename are rejected instead of silently overwriting one another.
func bundleHookScripts(m *Manifest, hooksDir string) ([]config.OutputFile, error) {
	sourceByBase := make(map[string]string)
	var outputs []config.OutputFile
	for _, group := range m.Hooks {
		for _, action := range group.Hooks {
			if action.Script == "" {
				continue
			}
			script := filepath.ToSlash(action.Script)
			base := path.Base(script)
			if existing, seen := sourceByBase[base]; seen {
				if existing == script {
					continue
				}
				return nil, oops.
					With("event", group.Event).
					With("script", script).
					With("conflicting_script", existing).
					Hint("Hook scripts are bundled flat into hooks/<basename>; rename one of them").
					Errorf("plugin %q declares conflicting hook script basename %q", m.Name, base)
			}
			sourceByBase[base] = script
			out, err := passthroughFile(m.SourceDir, action.Script, filepath.Join(hooksDir, base))
			if err != nil {
				return nil, oops.
					With("event", group.Event).
					With("script", script).
					Hint("Point 'script' at a file that exists in the project").
					Wrapf(err, "bundle hook script")
			}
			outputs = append(outputs, out)
		}
	}
	return outputs, nil
}

// hooksFileDoc wraps the hooks block for the standalone hooks.json file
// (Claude/Cursor/Codex) which nests everything under a top-level "hooks" key.
type hooksFileDoc struct {
	Hooks map[string][]hookGroupDoc `json:"hooks"`
}

// renderHooksFile emits a hooks.json OutputFile at filePath plus every bundled
// hook script, or nothing when the plugin declares no hooks. Scripts land beside
// hooks.json, which is the directory each runtime's hook root variable resolves
// to, so the rendered command and the bundled file always agree.
func renderHooksFile(m *Manifest, runtime, filePath string) ([]config.OutputFile, error) {
	block := hooksBlock(m, runtime)
	if block == nil {
		return nil, nil
	}
	outputs, err := bundleHookScripts(m, filepath.Dir(filePath))
	if err != nil {
		return nil, err
	}
	out, err := jsonOutput(filePath, hooksFileDoc{Hooks: block})
	if err != nil {
		return nil, err
	}
	return append(outputs, out), nil
}
