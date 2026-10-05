package lint

import (
	"fmt"
	"regexp"
	"strings"
)

// Codes for the frontmatter value checks.
const (
	CodeFrontmatterValue = "AR304"
	CodeToolUnknown      = "AR305"
)

func init() {
	registerRules(
		RuleInfo{CodeFrontmatterValue, "frontmatter-value-invalid", SeverityWarning, "a frontmatter value is not one the Claude Code skill or subagent reference accepts (effort, context, model, permissionMode, memory, shell, booleans, paths)"},
		RuleInfo{CodeToolUnknown, "tool-name-unknown", SeverityWarning, "allowed-tools, tools or disallowedTools names a tool Claude Code does not have, or lists a tool as both allowed and denied"},
	)
	registerItemCheck(checkFrontmatterValues)
	registerItemCheck(checkToolNames)
}

// Enumerations from the Claude Code skills and subagents references.
var (
	effortValues         = []string{"low", "medium", "high", "xhigh", "max"}
	contextValues        = []string{"fork"}
	permissionModeValues = []string{"default", "acceptEdits", "auto", "dontAsk", "bypassPermissions", "plan"}
	memoryValues         = []string{"user", "project", "local"}
	shellValues          = []string{"bash", "powershell"}
	isolationValues      = []string{"worktree"}
	colorValues          = []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan"}
	modelAliases         = []string{"sonnet", "opus", "haiku", "inherit", "default", "best", "opusplan", "sonnet[1m]", "opus[1m]"}
)

// Keys that must be YAML booleans, per content kind.
var boolKeys = map[string][]string{
	kindSkill:   {keyDisableModel, keyUserInvocable, keyBackground},
	kindCommand: {keyDisableModel, keyUserInvocable, keyBackground},
	kindAgent:   {keyBackground, "omitClaudeMd"},
	kindRule:    {keyAlwaysApply},
	kindContext: {keyAlwaysApply},
}

// vendorModelRe accepts the model IDs other harnesses use, so a subagent written
// for another tool is not reported for a model this check cannot enumerate.
var (
	claudeModelRe = regexp.MustCompile(`^(?:claude-[a-z0-9.@_-]+|(?:[a-z]{2,6}\.)?anthropic\.claude-[a-z0-9.:_-]+|arn:\S+)(?:\[1m\])?$`)
	vendorModelRe = regexp.MustCompile(`^(?:gpt-|o\d|gemini-|llama|mistral|codestral|grok|deepseek|qwen|kimi|glm-|command-)[a-z0-9.:_/-]*$`)
)

func checkFrontmatterValues(r *runner, it *item, _ doc, fm frontmatter) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	bad := func(k fmKey, format string, args ...any) {
		r.add(CodeFrontmatterValue, it.abs, k.Line, "%s", fmt.Sprintf("frontmatter %q: ", k.Name)+fmt.Sprintf(format, args...))
	}
	enum := func(key string, allowed []string, kinds ...string) {
		k, ok := fm.top(key)
		if !ok || (len(kinds) > 0 && !inSet(kinds, it.kind)) {
			return
		}
		v, isStr := k.Value.(string)
		switch {
		case k.Value == nil:
			bad(k, "is empty; use one of %s", strings.Join(allowed, ", "))
		case !isStr:
			bad(k, "must be a string, one of %s", strings.Join(allowed, ", "))
		case !inSet(allowed, strings.TrimSpace(v)):
			hint := ""
			if near := closest(v, allowed, 2); near != "" {
				hint = fmt.Sprintf("; did you mean %q?", near)
			}
			bad(k, "%q is not one of %s%s", v, strings.Join(allowed, ", "), hint)
		}
	}
	enum(keyEffort, effortValues)
	enum("context", contextValues, kindSkill, kindCommand)
	enum("shell", shellValues, kindSkill, kindCommand)
	enum("permissionMode", permissionModeValues, kindAgent)
	enum("memory", memoryValues, kindAgent)
	enum("isolation", isolationValues, kindAgent)
	enum("color", colorValues, kindAgent)

	for _, key := range boolKeys[it.kind] {
		if k, ok := fm.top(key); ok {
			if _, isBool := k.Value.(bool); !isBool {
				bad(k, "must be a YAML boolean (true or false), got %s", describeValue(k.Value))
			}
		}
	}
	if k, ok := fm.top("maxTurns"); ok && it.kind == kindAgent {
		if n, isInt := k.Value.(int); !isInt || n < 1 {
			bad(k, "must be a positive integer, got %s", describeValue(k.Value))
		}
	}
	if k, ok := fm.top("model"); ok && (it.kind == kindSkill || it.kind == kindCommand || it.kind == kindAgent) {
		checkModelValue(k, bad)
	}
	for _, key := range []string{keyPaths, keyGlobs} {
		if k, ok := fm.top(key); ok {
			checkPathsShape(k, bad)
		}
	}
	for _, key := range []string{keyName, keyDescription, "when_to_use", keyArgumentHint} {
		if k, ok := fm.top(key); ok && k.Value != nil {
			if _, isStr := k.Value.(string); !isStr && (key != keyArgumentHint || !isList(k.Value)) {
				bad(k, "must be text, got %s (quote the value)", describeValue(k.Value))
			}
		}
	}
}

func isList(v any) bool { _, ok := v.([]any); return ok }

func describeValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "an empty value"
	case bool:
		return fmt.Sprintf("the boolean %v", t)
	case int, int64, float64:
		return fmt.Sprintf("the number %v", t)
	case string:
		return fmt.Sprintf("the string %q", t)
	case []any:
		return "a list"
	default:
		return "a mapping"
	}
}

func checkModelValue(k fmKey, bad func(fmKey, string, ...any)) {
	v, ok := k.Value.(string)
	if !ok {
		bad(k, "must be a model alias or ID string, got %s", describeValue(k.Value))
		return
	}
	v = strings.TrimSpace(v)
	low := strings.ToLower(v)
	switch {
	case v == "":
		bad(k, "is empty; use an alias such as sonnet, opus, haiku or inherit, or a full model ID")
	case inSet(modelAliases, low) || claudeModelRe.MatchString(v) || vendorModelRe.MatchString(low) || strings.ContainsAny(v, "/:"):
	case strings.ContainsAny(v, " \t"):
		bad(k, "%q contains whitespace", v)
	default:
		hint := ""
		if near := closest(v, modelAliases, 2); near != "" {
			hint = fmt.Sprintf("; did you mean %q?", near)
		}
		bad(k, "%q is not a known alias (sonnet, opus, haiku, inherit) or a full model ID%s", v, hint)
	}
}

func checkPathsShape(k fmKey, bad func(fmKey, string, ...any)) {
	switch t := k.Value.(type) {
	case nil:
		bad(k, "is empty; list the globs the item applies to")
	case string:
		if strings.TrimSpace(t) == "" {
			bad(k, "is empty; list the globs the item applies to")
		}
	case []any:
		if len(t) == 0 {
			bad(k, "is an empty list")
		}
		for _, e := range t {
			if s, isStr := e.(string); !isStr || strings.TrimSpace(s) == "" {
				bad(k, "entries must be non-empty glob strings, got %s", describeValue(e))
			}
		}
	default:
		bad(k, "must be a glob string or a list of globs, got %s", describeValue(k.Value))
	}
}

// Tool names Claude Code provides (tools reference). An MCP tool is mcp__server__tool.
var claudeTools = []string{
	"Agent", "AskUserQuestion", "Bash", "BashOutput", "CronCreate", "CronDelete", "CronList", "Edit", "EnterPlanMode",
	"EnterWorktree", "ExitPlanMode", "ExitWorktree", "Glob", "Grep", "KillBash", "KillShell", "LS", "LSP", "ListMcpResourcesTool",
	"Monitor", "MultiEdit", "NotebookEdit", "NotebookRead", "PowerShell", "Read", "ReadMcpResourceTool", "SendMessage", "Skill",
	"SlashCommand", "Task", "TaskCreate", "TaskGet", "TaskList", "TaskOutput", "TaskStop", "TaskUpdate", "TeamCreate", "TeamDelete",
	"TodoRead", "TodoWrite", "ToolSearch", "WebFetch", "WebSearch", "Write",
}

var mcpToolRe = regexp.MustCompile(`^mcp__[A-Za-z0-9_.-]+?(?:__(?:[A-Za-z0-9_.-]+|\*))?$`)

// toolKeys are the frontmatter keys that list tools, per content kind.
var toolKeys = map[string][][2]string{ // {allow key, deny key}
	kindSkill:   {{keyAllowedTools, keyDisallowedTools}},
	kindCommand: {{keyAllowedTools, keyDisallowedTools}},
	kindAgent:   {{keyTools, "disallowedTools"}},
}

func toolBase(entry string) string {
	name, _, _ := strings.Cut(entry, "(")
	return strings.TrimSpace(name)
}

func checkToolNames(r *runner, it *item, _ doc, fm frontmatter) {
	pairs, ok := toolKeys[it.kind]
	if !ok || !r.targetsClaude() {
		return
	}
	known := map[string]bool{}
	for _, n := range r.lc.KnownNames {
		known[strings.ToLower(n)] = true
	}
	for _, pair := range pairs {
		allowed := map[string]bool{}
		for i, key := range pair {
			k, found := fm.top(key)
			if !found {
				continue
			}
			for _, entry := range splitTools(k.Value) {
				if msg := toolProblem(entry, known); msg != "" {
					r.add(CodeToolUnknown, it.abs, k.Line, "%s: %s", key, msg)
				}
				if i == 0 {
					allowed[entry] = true
				} else if allowed[entry] || (!strings.Contains(entry, "(") && hasBaseTool(allowed, entry)) {
					r.add(CodeToolUnknown, it.abs, k.Line, "%s lists %q, which %s also allows", key, entry, pair[0])
				}
			}
		}
	}
}

func hasBaseTool(set map[string]bool, name string) bool {
	for e := range set {
		if toolBase(e) == name && !strings.Contains(e, "(") {
			return true
		}
	}
	return false
}

// toolProblem explains why an allowed-tools or tools entry is not a tool, or "".
func toolProblem(entry string, known map[string]bool) string {
	if strings.Count(entry, "(") != strings.Count(entry, ")") {
		return fmt.Sprintf("%q has unbalanced parentheses", entry)
	}
	name := toolBase(entry)
	switch {
	case name == "*" || name == "":
		return ""
	case strings.HasPrefix(name, "mcp__"):
		if !mcpToolRe.MatchString(name) {
			return fmt.Sprintf("%q is not a valid MCP tool name (mcp__<server>__<tool>)", entry)
		}
		return ""
	case inSet(claudeTools, name) || known[strings.ToLower(name)]:
		return ""
	}
	hint := ""
	if near := closest(name, claudeTools, 2); near != "" {
		hint = fmt.Sprintf("; did you mean %q?", near)
	} else if strings.HasPrefix(strings.ToLower(name), "mcp") {
		hint = "; MCP tools are named mcp__<server>__<tool>"
	}
	return fmt.Sprintf("%q is not a Claude Code tool%s (list a tool provided elsewhere in lint.known_names)", name, hint)
}
