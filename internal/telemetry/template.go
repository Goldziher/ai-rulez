package telemetry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// Template output formats.
const (
	FormatJSON = "json"
	FormatTOML = "toml"
)

// hookTimeout is the per-hook timeout written to the template, in seconds.
const hookTimeout = 5

// TemplateOptions configures HookTemplate.
type TemplateOptions struct {
	// Executable is the command that runs ai-rulez. Default "ai-rulez".
	Executable string
	// Harness is claude (default), codex or cursor.
	Harness string
	// Role is passed to the recorders when set.
	Role string
	// Format is "json" (a hooks block to merge into the harness settings file,
	// default) or "toml" ([[hooks]] groups for config.toml, which `generate`
	// writes into .claude/settings.json and owns).
	Format string
}

// telemetryEvents are the Claude Code events the item recorder handles, in the
// order they appear in the template.
var telemetryEvents = []string{HookInstructionsLoaded, HookSubagentStart, HookSubagentStop}

// HookTemplate returns the hook configuration that records skill, rule, context
// and agent loads. For Claude Code it is the usage template (PreToolUse on the
// Skill tool, UserPromptExpansion) plus InstructionsLoaded, SubagentStart and
// SubagentStop; every handler is async with a short timeout because the recorders
// only append to a local file. Codex and Cursor document no instruction-load
// event, so they get the usage template plus the subagent events they do document
// (Codex SubagentStart/SubagentStop, Cursor subagentStart/subagentStop).
func HookTemplate(options TemplateOptions) ([]byte, error) {
	harness := options.Harness
	if harness == "" {
		harness = usage.HarnessClaude
	}
	executable := options.Executable
	if executable == "" {
		executable = "ai-rulez"
	}
	format := options.Format
	if format == "" {
		format = FormatJSON
	}
	if format != FormatJSON && format != FormatTOML {
		return nil, oops.Errorf("unknown template format %q (use json or toml)", format)
	}
	if format == FormatTOML && harness != usage.HarnessClaude {
		return nil, oops.Errorf("the [[hooks]] snippet is generated for the claude harness only (codex and cursor use `usage hook`)")
	}
	base, err := usage.HookTemplate(usage.HookTemplateOptions{Executable: options.Executable, Harness: harness, Role: options.Role})
	if err != nil {
		return nil, err
	}
	if harness != usage.HarnessClaude {
		return otherHarnessJSON(base, harness, recordCommand(executable, options.Role, harness))
	}
	command := recordCommand(executable, options.Role, usage.HarnessClaude)
	if format == FormatTOML {
		return []byte(tomlTemplate(base, command)), nil
	}

	return claudeJSON(base, command)
}

// claudeJSON adds the three telemetry events to the usage template and marks
// every handler async with a short timeout.
func claudeJSON(base []byte, command string) ([]byte, error) {
	var doc struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(base, &doc); err != nil {
		return nil, oops.Wrapf(err, "decode usage template")
	}
	for _, event := range telemetryEvents {
		doc.Hooks[event] = []map[string]any{{"hooks": []any{map[string]any{"type": "command", "command": command}}}}
	}
	for _, groups := range doc.Hooks {
		for _, group := range groups {
			handlers, _ := group["hooks"].([]any) //nolint:errcheck // decoded JSON array
			for _, h := range handlers {
				if handler, ok := h.(map[string]any); ok {
					handler["async"], handler["timeout"] = true, hookTimeout
				}
			}
		}
	}
	out, err := json.MarshalIndent(map[string]any{"hooks": doc.Hooks}, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode hook template")
	}
	return append(out, '\n'), nil
}

// otherHarnessJSON adds the documented subagent events to the usage template of
// Codex (nested matcher groups, like Claude Code) or Cursor (flat entries, camelCase
// names, a documented per-hook timeout). Codex's `async` and `timeout` handler
// fields are not documented, so they are not written.
func otherHarnessJSON(base []byte, harness, command string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(base, &doc); err != nil {
		return nil, oops.Wrapf(err, "decode usage template")
	}
	hooks, _ := doc["hooks"].(map[string]any) //nolint:errcheck // produced by usage.HookTemplate
	if harness == usage.HarnessCursor {
		for _, event := range []string{"subagentStart", "subagentStop"} {
			hooks[event] = []any{map[string]any{"command": command, "timeout": hookTimeout}}
		}
	} else {
		for _, event := range []string{HookSubagentStart, HookSubagentStop} {
			hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}}
		}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode hook template")
	}
	return append(out, '\n'), nil
}

func recordCommand(executable, role, harness string) string {
	parts := []string{usage.ShellWord(executable), "telemetry", "record"}
	if harness != usage.HarnessClaude {
		parts = append(parts, "--harness", harness)
	}
	if role != "" {
		parts = append(parts, "--role", singleQuote(role))
	}
	return strings.Join(parts, " ")
}

func singleQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }

// tomlTemplate renders [[hooks]] groups for config.toml. The skill events reuse
// the command the usage template carries.
func tomlTemplate(usageJSON []byte, telemetryCommand string) string {
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	_ = json.Unmarshal(usageJSON, &doc) //nolint:errcheck // produced by usage.HookTemplate
	type group struct{ event, matcher, command string }
	var groups []group
	names := make([]string, 0, len(doc.Hooks))
	for name := range doc.Hooks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, g := range doc.Hooks[name] {
			for _, h := range g.Hooks {
				groups = append(groups, group{name, g.Matcher, h.Command})
			}
		}
	}
	for _, event := range telemetryEvents {
		groups = append(groups, group{event: event, command: telemetryCommand})
	}
	var b strings.Builder
	b.WriteString("# Paste into .ai-rulez/config.toml, then run `ai-rulez generate`:\n# the entries are written into .claude/settings.json, which ai-rulez owns key by key.\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "\n[[hooks]]\nevent = %q\ntargets = [\"claude\"]\n", g.event)
		if g.matcher != "" {
			fmt.Fprintf(&b, "matcher = %q\n", g.matcher)
		}
		fmt.Fprintf(&b, "[[hooks.hooks]]\ncommand = %s\nasync = true\ntimeout = %d\n", tomlString(g.command), hookTimeout)
	}
	return b.String()
}

func tomlString(s string) string {
	b, _ := json.Marshal(s) //nolint:errcheck // a string always encodes
	return string(b)
}
