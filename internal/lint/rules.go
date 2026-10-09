// Package lint implements `ai-rulez validate`: deep checks that find
// instruction content which parses fine but does not work (globs that match
// nothing, dead links, references to skills that do not exist, hooks that
// cannot run, oversize rules, ...). Every finding carries a stable code, a
// severity, and a file:line so it can be gated in CI and suppressed in config.
package lint

import (
	"encoding/json"
	"path/filepath"
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
	CodeSecretDetected          = "AR001"
	CodeHiddenCharacters        = "AR002"
	CodeCommentInstruction      = "AR003"
	CodeInjectionPhrase         = "AR004"
	CodeShellExec               = "AR005"
	CodeShellAccess             = "AR006"
	CodeToolBreadth             = "AR007"
	CodeOutboundHost            = "AR008"
	CodeEncodedBlob             = "AR009"
	CodeUnpinnedRemote          = "AR010"
	CodeExternalFinding         = "AR011"
	CodeScannerConfigInvalid    = "AR9E0"
	CodeScannerEgressUndeclared = "AR9E1"
	CodeScannerUnavailable      = "AR9E2"
	CodeScannerRunFailed        = "AR9E3"
	CodeScannerEgressBlocked    = "AR9E4"
	CodeScannerBaselineExpired  = "AR9E5"
	CodeScannerOutOfScope       = "AR9E6"
	CodeScannerNoIsolation      = "AR9E7"
	CodeCursorRuleExtension     = "AR9C1"
	CodeCursorRuleNotApplied    = "AR9C2"
	CodeCopilotExcludeAgent     = "AR9C3"
	CodeCopilotInstructionsName = "AR9C4"
	CodeGlobNoMatch             = "AR101"
	CodeLinkUnresolved          = "AR201"
	CodeAnchorUnresolved        = "AR202"
	CodeReferenceUnknown        = "AR301"
	CodeFrontmatterSkill        = "AR302"
	CodeFrontmatterKey          = "AR303"
	CodePathMissing             = "AR401"
	CodeSkillResourceMissing    = "AR402"
	CodeHookMissing             = "AR501"
	CodeHookNotExecutable       = "AR502"
	CodeScriptNotExecutable     = "AR503"
	CodeHookSourceMissing       = "AR504"
	CodeHookSourceNotExec       = "AR505"
	CodePermissionOverbroad     = "AR506"
	CodeMCPCommandNotFound      = "AR601"
	CodeDescriptionDup          = "AR701"
	CodeDescriptionNearDup      = "AR702"
	CodeDuplicateCollapsed      = "AR703"
	CodeDescriptionMissing      = "AR801"
	CodeDescriptionLength       = "AR802"
	CodeDescriptionStyle        = "AR803"
	CodeSkillNameInvalid        = "AR804"
	CodeSizeLines               = "AR901"
	CodeSizeTokens              = "AR902"
	CodeMetadataMissing         = "AR951"
	CodeMetadataInvalid         = "AR952"
	CodeMetadataStale           = "AR953"
	CodeSupersededMissing       = "AR954"
	CodePluginVersionDrift      = "AR961"
	CodeEvalsMissing            = "AR962"
	CodeRoleReferenceUnknown    = "AR971"
	CodeRoleExtendsInvalid      = "AR972"
	CodeRoleUnreachable         = "AR973"
	CodeLockSourceDrift         = "AR981"
	CodeLockOutputDrift         = "AR982"
	CodeLLMConfigInvalid        = "AR9L0"
	CodeLLMUntrustedKey         = "AR9L1"
)

// RuleInfo describes one check.
type RuleInfo struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Default  Severity `json:"default_severity"`
	Describe string   `json:"description"`
}

var baseRegistry = []RuleInfo{
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
	{CodeScannerConfigInvalid, "scanner-config-invalid", SeverityError, "a [[lint.external]] entry has an invalid timeout or an env_pass name an egress = false scanner must not receive; it is not run"},
	{CodeScannerEgressUndeclared, "scanner-egress-undeclared", SeverityWarning, "a [[lint.external]] entry does not declare egress, so it runs with the full environment"},
	{CodeScannerUnavailable, "scanner-unavailable", SeverityWarning, "a [[lint.external]] scanner's binary was not found, so it did not run"},
	{CodeScannerRunFailed, "scanner-run-failed", SeverityError, "a [[lint.external]] scanner timed out, exceeded the output cap, or printed unreadable, unsuccessful or oversized output"},
	{CodeScannerEgressBlocked, "scanner-egress-blocked", SeverityError, "a [[lint.external]] scanner was not run: egress = true without --allow-egress, or a network flag on an egress = false scanner"},
	{CodeScannerBaselineExpired, "scanner-baseline-expired", SeverityWarning, "an entry of scanner-baseline.json passed its expires date, so the finding it accepted is reported again"},
	{CodeScannerOutOfScope, "scanner-out-of-scope-result", SeverityWarning, "a staged [[lint.external]] scanner reported a result for a file that was not staged for it; the result was dropped"},
	{CodeScannerNoIsolation, "scanner-isolation-degraded", SeverityWarning, "isolation = auto found no process isolation backend, so staged scanners ran without network or write confinement"},
	{CodeCursorRuleExtension, "cursor-rule-extension-ignored", SeverityWarning, "a file in .cursor/rules is not .mdc, so Cursor ignores it (error when ai-rulez generated it; runs when cursor is a configured preset or in lint.traps.extra_harnesses)"},
	{CodeCursorRuleNotApplied, "cursor-rule-not-applied", SeverityWarning, "a hand-written .mdc rule has no description, globs or alwaysApply, so it applies only when @-mentioned"},
	{CodeCopilotExcludeAgent, "copilot-exclude-agent-invalid", SeverityWarning, "a Copilot instructions file sets excludeAgent to something other than code-review or cloud-agent"},
	{CodeCopilotInstructionsName, "copilot-instructions-suffix", SeverityWarning, "a file in .github/instructions does not end in .instructions.md, so Copilot skips it (error when ai-rulez generated it)"},
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
	{CodeDescriptionStyle, "description-style", SeverityOff, "a skill or agent description does not say when to use it (enabled by lint.description.require_use_when; commands are exempt, the user invokes them by name)"},
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
	{CodeLLMConfigInvalid, "llm-config-invalid", SeverityError, "the [llm] table is invalid: a literal secret instead of an api_key_env variable name, credentials in base_url, or a negative limit"},
	{CodeLLMUntrustedKey, "llm-untrusted-key", SeverityWarning, "a repository [llm] table sets allow_network, base_url, api_key_env or a price override, which only the user config file and AI_RULEZ_LLM_* may set; the value is ignored"},
}

// Rules returns the registry sorted by code.
func Rules() []RuleInfo {
	out := append([]RuleInfo(nil), ruleTables().rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// lookupRule resolves a code or a name, case-insensitively.
func lookupRule(key string) (RuleInfo, bool) {
	key = strings.TrimSpace(key)
	for _, r := range ruleTables().rules {
		if strings.EqualFold(r.Code, key) || strings.EqualFold(r.Name, key) {
			return r, true
		}
	}
	return RuleInfo{}, false
}

// Finding is one problem found in one place. Keep it under 128 bytes:
// code ranges over []Finding by value, and anything that does not belong in the
// JSON report's fixed keys lives behind Meta.
type Finding struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
	Root     string   `json:"root,omitempty"`
	// Trap describes a harness trap (AR9C*): the tool that ignores the file, the
	// vendor page that says so, the date it was checked, and how to fix it. Nil
	// for every other finding. It is a pointer so that a Finding stays small.
	Trap *TrapInfo `json:"-"`
	// Meta carries the optional annotations (repository path, baseline state,
	// analyzer, fix); nil for a plain finding.
	Meta *FindingMeta `json:"-"`
}

// TrapInfo is the provenance of a harness-trap finding.
type TrapInfo struct {
	Harness, Evidence, VerifiedOn, Hint string
}

// FindingMeta is the optional, non-core data of a finding.
type FindingMeta struct {
	// Fingerprint is stable across line moves: it hashes the code, the
	// repository-relative path and the normalized text of the flagged line.
	Fingerprint string
	// Path is File relative to the repository root (slash separated); SARIF
	// artifact locations and baselines use it.
	Path string
	// Accepted marks a finding recorded in the baseline: it is reported but
	// does not count toward the exit code.
	Accepted bool
	// AcceptReason is the baseline entry's reason.
	AcceptReason string
	// ScannerFailOn is the [lint.scanner_policy] fail_on threshold of a scanner
	// finding: one at least this severe fails the run even when it is below the
	// run's own threshold.
	ScannerFailOn string
	// Analyzer and Scope classify the rule that produced the finding.
	Analyzer, Scope string
	// Fix is the mechanical correction, when one exists.
	Fix *Fix
	// Hop says how far the finding's file is from the changed set in a
	// changed-only report: "changed", "dependent" or "transitive(n)".
	Hop string
	// Metric is the measured value of a size finding (lines, tokens). Its
	// bucket joins the fingerprint, so a baselined finding fires again once the
	// measurement has grown well past what was accepted.
	Metric int
}

func (f *Finding) meta() *FindingMeta {
	if f.Meta == nil {
		f.Meta = &FindingMeta{}
	}
	return f.Meta
}

// RepoPath returns File relative to the repository root, or File itself when
// the location is outside the repository.
func (f *Finding) RepoPath() string {
	if f.Meta != nil && f.Meta.Path != "" {
		return f.Meta.Path
	}
	return filepath.ToSlash(f.File)
}

// Fingerprint returns the stable identity of the finding ("" before a run assigned one).
func (f *Finding) Fingerprint() string {
	if f.Meta == nil {
		return ""
	}
	return f.Meta.Fingerprint
}

// Hop returns the changed-only hop of the finding ("" outside changed-only reports).
func (f *Finding) Hop() string {
	if f.Meta == nil {
		return ""
	}
	return f.Meta.Hop
}

// IsAccepted reports whether the baseline accepts the finding.
func (f *Finding) IsAccepted() bool { return f.Meta != nil && f.Meta.Accepted }

type findingJSON struct {
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	Severity     Severity `json:"severity"`
	File         string   `json:"file"`
	Line         int      `json:"line"`
	Message      string   `json:"message"`
	Root         string   `json:"root,omitempty"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
	Accepted     bool     `json:"accepted,omitempty"`
	AcceptReason string   `json:"accept_reason,omitempty"`
	Analyzer     string   `json:"analyzer,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	Harness      string   `json:"harness,omitempty"`
	Evidence     string   `json:"evidence,omitempty"`
	VerifiedOn   string   `json:"verified_on,omitempty"`
	Hint         string   `json:"hint,omitempty"`
	Hop          string   `json:"hop,omitempty"`
}

// MarshalJSON flattens Meta's reportable fields next to the core ones.
func (f Finding) MarshalJSON() ([]byte, error) {
	out := findingJSON{
		Code: f.Code, Name: f.Name, Severity: f.Severity, File: f.File, Line: f.Line,
		Message: f.Message, Root: f.Root, Fingerprint: f.Fingerprint(),
	}
	if f.Trap != nil {
		out.Harness, out.Evidence, out.VerifiedOn, out.Hint = f.Trap.Harness, f.Trap.Evidence, f.Trap.VerifiedOn, f.Trap.Hint
	}
	if f.Meta != nil {
		out.Accepted, out.AcceptReason = f.Meta.Accepted, f.Meta.AcceptReason
		out.Analyzer, out.Scope = f.Meta.Analyzer, f.Meta.Scope
		out.Hop = f.Meta.Hop
	}
	return json.Marshal(out) //nolint:wrapcheck // plain struct
}
