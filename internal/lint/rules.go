// Package lint implements `ai-rulez validate --strict`: deep checks that find
// instruction content which parses fine but does not work (globs that match
// nothing, dead links, references to skills that do not exist, hooks that
// cannot run, oversize rules, ...). Every finding carries a stable code, a
// severity, and a file:line so it can be gated in CI and suppressed in config.
package lint

import (
	"sort"
	"strings"
)

// Severity ranks a finding. SeverityOff drops the finding.
type Severity string

// Severities, lowest first.
const (
	SeverityOff     Severity = "off"
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

func (s Severity) rank() int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// AtLeast reports whether s is as severe as min.
func (s Severity) AtLeast(threshold Severity) bool {
	return s.rank() >= threshold.rank() && s.rank() > 0
}

// ParseSeverity parses a severity name; ok is false for anything unknown.
func ParseSeverity(v string) (Severity, bool) {
	switch s := Severity(strings.ToLower(strings.TrimSpace(v))); s {
	case SeverityOff, SeverityInfo, SeverityWarning, SeverityError:
		return s, true
	}
	return "", false
}

// Rule codes. They are part of the public contract (config, JSON output,
// inline ignores): never renumber or reuse one.
const (
	CodeGlobNoMatch          = "AR101"
	CodeLinkUnresolved       = "AR201"
	CodeAnchorUnresolved     = "AR202"
	CodeReferenceUnknown     = "AR301"
	CodeFrontmatterSkill     = "AR302"
	CodePathMissing          = "AR401"
	CodeSkillResourceMissing = "AR402"
	CodeHookMissing          = "AR501"
	CodeHookNotExecutable    = "AR502"
	CodeScriptNotExecutable  = "AR503"
	CodeMCPCommandNotFound   = "AR601"
	CodeDescriptionDup       = "AR701"
	CodeDescriptionNearDup   = "AR702"
	CodeDescriptionMissing   = "AR801"
	CodeDescriptionLength    = "AR802"
	CodeDescriptionStyle     = "AR803"
	CodeSkillNameInvalid     = "AR804"
	CodeSizeLines            = "AR901"
	CodeSizeTokens           = "AR902"
	CodeMetadataMissing      = "AR951"
)

// RuleInfo describes one check.
type RuleInfo struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Default  Severity `json:"default_severity"`
	Describe string   `json:"description"`
}

var registry = []RuleInfo{
	{CodeGlobNoMatch, "glob-no-match", SeverityError, "a paths/globs pattern matches no tracked file"},
	{CodeLinkUnresolved, "link-unresolved", SeverityError, "a relative markdown link does not resolve to a file"},
	{CodeAnchorUnresolved, "anchor-unresolved", SeverityWarning, "a markdown link anchor matches no heading in the target"},
	{CodeReferenceUnknown, "reference-unknown", SeverityError, "a skill, agent, rule or command referenced by name does not exist"},
	{CodeFrontmatterSkill, "frontmatter-skill-unknown", SeverityError, "frontmatter skills: lists a skill that does not exist"},
	{CodePathMissing, "path-missing", SeverityWarning, "a backticked repo path does not exist"},
	{CodeSkillResourceMissing, "skill-resource-missing", SeverityError, "a skill references a references/, scripts/ or assets/ file it does not ship"},
	{CodeHookMissing, "hook-missing", SeverityError, "a hook command points at a repo file that does not exist"},
	{CodeHookNotExecutable, "hook-not-executable", SeverityError, "a hook command runs a repo file that lacks the executable bit"},
	{CodeScriptNotExecutable, "script-not-executable", SeverityWarning, "a skill script with a shebang lacks the executable bit"},
	{CodeMCPCommandNotFound, "mcp-command-not-found", SeverityWarning, "a stdio MCP server command is not on PATH"},
	{CodeDescriptionDup, "description-duplicate", SeverityWarning, "two skills, agents or commands share an identical description"},
	{CodeDescriptionNearDup, "description-near-duplicate", SeverityWarning, "two descriptions are near-identical, so the model cannot tell them apart"},
	{CodeDescriptionMissing, "description-missing", SeverityWarning, "a skill, agent or command has no description"},
	{CodeDescriptionLength, "description-length", SeverityWarning, "a description is shorter or longer than the configured bounds"},
	{CodeDescriptionStyle, "description-style", SeverityOff, "a description does not say when to use the item (enabled by lint.description.require_use_when)"},
	{CodeSkillNameInvalid, "skill-name-invalid", SeverityWarning, "a skill name is not lowercase-hyphen, exceeds 64 characters, or differs from its directory"},
	{CodeSizeLines, "size-lines", SeverityWarning, "an item exceeds its line budget"},
	{CodeSizeTokens, "size-tokens", SeverityWarning, "an item exceeds its token budget"},
	{CodeMetadataMissing, "metadata-missing", SeverityError, "an item lacks a frontmatter key required by lint.require_metadata"},
}

// Rules returns the registry sorted by code.
func Rules() []RuleInfo {
	out := append([]RuleInfo(nil), registry...)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// lookupRule resolves a code or a name, case-insensitively.
func lookupRule(key string) (RuleInfo, bool) {
	key = strings.TrimSpace(key)
	for _, r := range registry {
		if strings.EqualFold(r.Code, key) || strings.EqualFold(r.Name, key) {
			return r, true
		}
	}
	return RuleInfo{}, false
}

// Finding is one problem found in one place.
type Finding struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
	Root     string   `json:"root,omitempty"`
}
