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
	executable := options.Executable
	if executable == "" {
		executable = "ai-rulez"
	}
	logPath := options.LogPath
	if logPath == "" && options.SinkCommand == "" {
		logPath = DefaultLogPath
	}

	parts := []string{executable, "usage", "record"}
	if logPath != "" {
		parts = append(parts, "--log", shellQuote(logPath))
	}
	if options.SinkCommand != "" {
		parts = append(parts, "--sink-command", singleQuote(options.SinkCommand))
	}
	if options.IndexPath != "" {
		parts = append(parts, "--index", shellQuote(options.IndexPath))
	}
	command := strings.Join(parts, " ")

	handler := func(matcher string) []map[string]any {
		return []map[string]any{{
			"matcher": matcher,
			"hooks":   []map[string]any{{"type": "command", "command": command}},
		}}
	}
	document := map[string]any{"hooks": map[string]any{
		"PreToolUse":          handler("Skill"),
		"UserPromptExpansion": handler("*"),
	}}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode hook template")
	}
	return append(data, '\n'), nil
}

// shellQuote double-quotes a value for the shell while leaving ${VAR}
// expansion (such as CLAUDE_PROJECT_DIR) to the shell.
func shellQuote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`").Replace(value)
	return `"` + escaped + `"`
}

// singleQuote quotes a value so the shell passes it through untouched.
func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
