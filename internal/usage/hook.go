package usage

import (
	"encoding/json"
	"strings"

	"github.com/samber/oops"
)

// HookTemplateOptions configures the hook settings snippet.
type HookTemplateOptions struct {
	// Executable is the command that runs the recorder. Default "ai-rulez".
	Executable string
	// LogPath is the usage log the hook appends to.
	LogPath string
	// SinkCommand is an optional additional sink command.
	SinkCommand string
	// IndexPath is passed through to the recorder when set.
	IndexPath string
	// Harness selects the template: claude (default), codex or cursor. Any other
	// harness yields an UnsupportedHarnessError.
	Harness string
	// Role is passed through to the recorder when set.
	Role string
}

// UnsupportedHarnessError says the repository has no verified hook support for a
// harness, so no template is produced.
type UnsupportedHarnessError struct{ Harness string }

func (e *UnsupportedHarnessError) Error() string {
	return "no usage hook template for the " + e.Harness + " harness: its skill-load hook payload is not documented in ai-rulez (supported: claude, codex, cursor)"
}

// HookFile is the settings file each harness reads its hooks from.
func HookFile(harness string) string {
	switch harness {
	case HarnessCodex:
		return ".codex/hooks.json"
	case HarnessCursor:
		return ".cursor/hooks.json"
	}
	return ".claude/settings.json"
}

// DefaultLogPath is the log location the template uses: machine-local, so a
// usage log is never committed by accident.
const DefaultLogPath = "${CLAUDE_PROJECT_DIR}/.ai-rulez/local/usage.jsonl"

// HookTemplate returns the `hooks` block for a Claude Code settings file that
// records skill invocations. It covers the two ways a skill is used: the model
// calling the Skill tool (PreToolUse) and the user typing the skill's slash
// command (UserPromptExpansion). It is a template to merge into settings by
// hand; nothing installs it.
func HookTemplate(options HookTemplateOptions) ([]byte, error) {
	harness := options.Harness
	if harness == "" {
		harness = HarnessClaude
	}
	if harness != HarnessClaude && harness != HarnessCodex && harness != HarnessCursor {
		return nil, &UnsupportedHarnessError{Harness: harness}
	}
	command := recordCommand(&options, harness)

	var document map[string]any
	switch harness {
	case HarnessCodex:
		// Codex uses Claude Code's event and tool names; it reads skills with its shell tool.
		document = map[string]any{keyHooks: map[string]any{"PreToolUse": matcherHandler("Bash", command)}}
	case HarnessCursor:
		// Cursor's hooks.json: flat handler entries under camelCase event names.
		document = map[string]any{"version": 1, keyHooks: map[string]any{
			"preToolUse": []map[string]any{{keyCommand: command, "matcher": "Shell"}},
		}}
	default:
		document = map[string]any{keyHooks: map[string]any{
			"PreToolUse":          matcherHandler("Skill", command),
			"UserPromptExpansion": matcherHandler("*", command),
		}}
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode hook template")
	}
	return append(data, '\n'), nil
}

const (
	keyHooks   = "hooks"
	keyCommand = "command"
)

// recordCommand builds the `ai-rulez telemetry record` command line a hook runs.
func recordCommand(options *HookTemplateOptions, harness string) string {
	executable := options.Executable
	if executable == "" {
		executable = "ai-rulez"
	}
	logPath := options.LogPath
	if logPath == "" && options.SinkCommand == "" && harness == HarnessClaude {
		// Only Claude Code documents a project-dir variable; the recorder falls back
		// to the hook's working directory for the others.
		logPath = DefaultLogPath
	}
	parts := []string{ShellWord(executable), "telemetry", "record"}
	if harness != HarnessClaude {
		parts = append(parts, "--harness", harness)
	}
	if options.Role != "" {
		parts = append(parts, "--role", singleQuote(options.Role))
	}
	if logPath != "" {
		parts = append(parts, "--log", shellQuote(logPath))
	}
	if options.SinkCommand != "" {
		parts = append(parts, "--sink-command", singleQuote(options.SinkCommand))
	}
	if options.IndexPath != "" {
		parts = append(parts, "--index", shellQuote(options.IndexPath))
	}
	return strings.Join(parts, " ")
}

func matcherHandler(matcher, command string) []map[string]any {
	return []map[string]any{{
		"matcher": matcher,
		keyHooks:  []map[string]any{{"type": keyCommand, keyCommand: command}},
	}}
}

// projectDirVar is the one variable a generated hook command leaves for the
// shell to expand: Claude Code sets it to the project root.
const projectDirVar = "${CLAUDE_PROJECT_DIR}"

// shellQuote quotes a path for the shell. A value with nothing the shell
// interprets is double-quoted as is. The project-directory variable is the only
// expansion kept: it is left live (inside double quotes when the rest is plain,
// else closed out of single quotes). Anything else a double-quoted string would
// still interpret ("$(...)", "$VAR", a backtick, a backslash) makes the whole
// value single-quoted, so it stays literal.
func shellQuote(value string) string {
	rest := strings.ReplaceAll(value, projectDirVar, "")
	if !strings.ContainsAny(rest, "$`\\\"!") {
		return `"` + value + `"`
	}
	if !strings.Contains(value, projectDirVar) {
		return singleQuote(value)
	}
	pieces := strings.Split(value, projectDirVar)
	var b strings.Builder
	for i, piece := range pieces {
		if i > 0 {
			b.WriteString(`"` + projectDirVar + `"`)
		}
		if piece != "" {
			b.WriteString(singleQuote(piece))
		}
	}
	return b.String()
}

// singleQuote quotes a value so the shell passes it through untouched.
func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// ShellWord quotes an executable or other word for the shell only when it holds a
// character the shell would interpret (a space, a quote, "$", a backtick, ...).
// Plain names and paths stay as they are, so the common "ai-rulez" is unchanged.
func ShellWord(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("_-./:@%+=,", r)
	}) < 0 {
		return value
	}
	return singleQuote(value)
}
