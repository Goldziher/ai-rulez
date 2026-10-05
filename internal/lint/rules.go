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
	CodeSecretDetected       = "AR001"
	CodeHiddenCharacters     = "AR002"
	CodeCommentInstruction   = "AR003"
	CodeInjectionPhrase      = "AR004"
	CodeShellExec            = "AR005"
	CodeShellAccess          = "AR006"
	CodeToolBreadth          = "AR007"
	CodeOutboundHost         = "AR008"
	CodeEncodedBlob          = "AR009"
	CodeUnpinnedRemote       = "AR010"
	CodeExternalFinding      = "AR011"
	CodeGlobNoMatch          = "AR101"
	CodeLinkUnresolved       = "AR201"
	CodeAnchorUnresolved     = "AR202"
	CodeReferenceUnknown     = "AR301"
	CodeFrontmatterSkill     = "AR302"
	CodeFrontmatterKey       = "AR303"
	CodePathMissing          = "AR401"
	CodeSkillResourceMissing = "AR402"
	CodeHookMissing          = "AR501"
	CodeHookNotExecutable    = "AR502"
	CodeScriptNotExecutable  = "AR503"
	CodeHookSourceMissing    = "AR504"
	CodeHookSourceNotExec    = "AR505"
	CodePermissionOverbroad  = "AR506"
	CodeMCPCommandNotFound   = "AR601"
	CodeDescriptionDup       = "AR701"
	CodeDescriptionNearDup   = "AR702"
	CodeDuplicateCollapsed   = "AR703"
	CodeDescriptionMissing   = "AR801"
	CodeDescriptionLength    = "AR802"
	CodeDescriptionStyle     = "AR803"
	CodeSkillNameInvalid     = "AR804"
	CodeSizeLines            = "AR901"
	CodeSizeTokens           = "AR902"
	CodeMetadataMissing      = "AR951"
	CodeMetadataInvalid      = "AR952"
	CodeMetadataStale        = "AR953"
	CodeSupersededMissing    = "AR954"
	CodePluginVersionDrift   = "AR961"
	CodeEvalsMissing         = "AR962"
	CodeRoleReferenceUnknown = "AR971"
	CodeRoleExtendsInvalid   = "AR972"
	CodeRoleUnreachable      = "AR973"
	CodeLockSourceDrift      = "AR981"
	CodeLockOutputDrift      = "AR982"
	CodeLLMConfigInvalid     = "AR9C0"
	CodeLLMUntrustedKey      = "AR9C1"
)

// RuleInfo describes one check.
type RuleInfo struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Default  Severity `json:"default_severity"`
	Describe string   `json:"description"`
}

var registry = []RuleInfo{
	{CodeSecretDetected, "secret-detected", SeverityError, "a credential pattern (cloud key, token, private key or a configured pattern) appears in content or a script"},
	{CodeHiddenCharacters, "hidden-characters", SeverityError, "zero-width, bidirectional-control or Unicode tag characters hide text from a reviewer"},
	{CodeCommentInstruction, "html-comment-instruction", SeverityWarning, "an HTML comment carries imperative or injection-style text the reader will not see"},
	{CodeInjectionPhrase, "prompt-injection-phrase", SeverityWarning, "text tries to override earlier instructions or hide actions from the user"},
	{CodeShellExec, "risky-shell-exec", SeverityError, "a download piped to a shell, eval of dynamic text, or a decoded payload executed"},
	{CodeShellAccess, "risky-shell-access", SeverityWarning, "a command reads a credential location or writes outside the project"},
	{CodeToolBreadth, "tool-breadth", SeverityWarning, "allowed-tools grants an unrestricted tool such as Bash(*)"},
	{CodeOutboundHost, "outbound-host", SeverityWarning, "a URL points to a host outside lint.security.allowed_hosts (checked only when the list is set)"},
	{CodeEncodedBlob, "encoded-blob", SeverityWarning, "a long base64-like blob that a reviewer cannot read"},
	{CodeUnpinnedRemote, "unpinned-remote", SeverityWarning, "a remote include or installed skill follows a moving ref and ai-rulez.lock does not pin it"},
	{CodeExternalFinding, "external-finding", SeverityWarning, "a finding reported by a scanner configured in lint.external"},
	{CodeGlobNoMatch, "glob-no-match", SeverityError, "a paths/globs pattern matches no tracked file"},
	{CodeLinkUnresolved, "link-unresolved", SeverityError, "a relative markdown link does not resolve to a file"},
	{CodeAnchorUnresolved, "anchor-unresolved", SeverityWarning, "a markdown link anchor matches no heading in the target"},
	{CodeReferenceUnknown, "reference-unknown", SeverityError, "a skill, agent, rule or command referenced by name does not exist"},
	{CodeFrontmatterSkill, "frontmatter-skill-unknown", SeverityError, "frontmatter skills: lists a skill that does not exist"},
	{CodeFrontmatterKey, "frontmatter-key-unknown", SeverityWarning, "a frontmatter key is not a known Agent Skills, Claude Code or ai-rulez key (a typo is silently ignored by the tools)"},
	{CodePathMissing, "path-missing", SeverityWarning, "a backticked repo path does not exist"},
	{CodeSkillResourceMissing, "skill-resource-missing", SeverityError, "a skill references a references/, scripts/ or assets/ file it does not ship"},
	{CodeHookMissing, "hook-missing", SeverityError, "a hook command points at a repo file that does not exist"},
	{CodeHookNotExecutable, "hook-not-executable", SeverityError, "a hook command runs a repo file that lacks the executable bit"},
	{CodeHookSourceMissing, "hook-source-missing", SeverityError, "a [[hooks]] script in config.toml does not exist"},
	{CodeHookSourceNotExec, "hook-source-not-executable", SeverityError, "a [[hooks]] script in config.toml lacks the executable bit"},
	{CodePermissionOverbroad, "permission-overbroad", SeverityWarning, "a [permissions] allow rule permits every call of a tool"},
	{CodeScriptNotExecutable, "script-not-executable", SeverityWarning, "a skill script with a shebang lacks the executable bit"},
	{CodeMCPCommandNotFound, "mcp-command-not-found", SeverityWarning, "a stdio MCP server command is not on PATH"},
	{CodeDescriptionDup, "description-duplicate", SeverityWarning, "two skills, agents or commands share an identical description"},
	{CodeDescriptionNearDup, "description-near-duplicate", SeverityWarning, "two descriptions are near-identical, so the model cannot tell them apart"},
	{CodeDuplicateCollapsed, "duplicate-collapsed", SeverityWarning, "two sources define the same name and one was silently dropped (allow intentional shadowing with lint.allow_overrides)"},
	{CodeDescriptionMissing, "description-missing", SeverityWarning, "a skill, agent or command has no description"},
	{CodeDescriptionLength, "description-length", SeverityWarning, "a description is shorter or longer than the configured bounds"},
	{CodeDescriptionStyle, "description-style", SeverityOff, "a description does not say when to use the item (enabled by lint.description.require_use_when)"},
	{CodeSkillNameInvalid, "skill-name-invalid", SeverityWarning, "a skill name is not lowercase-hyphen, exceeds 64 characters, or differs from its directory"},
	{CodeSizeLines, "size-lines", SeverityWarning, "an item exceeds its line budget"},
	{CodeSizeTokens, "size-tokens", SeverityWarning, "an item exceeds its token budget"},
	{CodeMetadataMissing, "metadata-missing", SeverityError, "an item lacks a frontmatter key required by lint.require_metadata or a required lint.metadata rule"},
	{CodeMetadataInvalid, "metadata-invalid", SeverityError, "a frontmatter value is not the type or enum value its lint.metadata rule demands"},
	{CodeMetadataStale, "metadata-stale", SeverityWarning, "a dated frontmatter value is older than its lint.metadata max_age_days"},
	{CodeSupersededMissing, "superseded-by-missing", SeverityError, "a deprecated item names a superseded_by replacement that does not exist"},
	{CodePluginVersionDrift, "plugin-version-drift", SeverityWarning, "a generated plugin's content changed since HEAD but its version did not, so installs keep the cached copy"},
	{CodeEvalsMissing, "evals-missing", SeverityOff, "a skill has no eval cases (enabled by lint.evals.require or lint.severity; exempt skills go in lint.evals.allow)"},
	{CodeRoleReferenceUnknown, "role-reference-unknown", SeverityError, "a role names a domain, skill, rule, agent or command that does not exist (or exists only in a domain the role does not select)"},
	{CodeRoleExtendsInvalid, "role-extends-invalid", SeverityError, "a role extends an unknown role, takes part in an extends cycle, or extends a role that itself extends another (one level only)"},
	{CodeRoleUnreachable, "role-unreachable-dependency", SeverityWarning, "a kept item lists a skill in its skills: frontmatter that the role drops or hides from the model"},
	{CodeLockSourceDrift, "lock-source-drift", SeverityError, "an authored item differs from the content pinned in ai-rulez.lock (raised only when a lock exists and [lock] enforce = true)"},
	{CodeLockOutputDrift, "lock-output-drift", SeverityError, "a generated output differs from the digest pinned in ai-rulez.lock (raised only when a lock exists and [lock] enforce = true)"},
	{CodeLLMConfigInvalid, "llm-config-invalid", SeverityError, "the [llm] table is invalid: unknown backend, a literal secret instead of an api_key_env variable name, credentials in base_url, or a negative limit"},
	{CodeLLMUntrustedKey, "llm-untrusted-key", SeverityWarning, "a repository [llm] table sets allow_network, base_url, api_key_env or a price override, which only the user config file and AI_RULEZ_LLM_* may set; the value is ignored"},
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
