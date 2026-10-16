package judge

import (
	"strings"
	"sync"
)

// CodeInfo describes one finding code.
type CodeInfo struct {
	Code     string
	Name     string
	Category Category
	Summary  string
}

var codeTable = sync.OnceValue(func() []CodeInfo {
	return []CodeInfo{
		{"JR-S01", "instruction_override", CategorySecurity, "Directs the agent to ignore, replace or outrank its other instructions or safeguards, or addresses the reviewer."},
		{"JR-S02", "concealment", CategorySecurity, "Directs the agent to hide actions or output from the user, or hides text from a human reader."},
		{"JR-S03", "exfiltration", CategorySecurity, "Sensitive local data reaches an external sink."},
		{"JR-S04", "credential_access", CategorySecurity, "Reads credential stores or secrets beyond what the purpose needs, or passes raw secrets through the model."},
		{"JR-S05", "code_execution", CategorySecurity, "Downloaded, decoded, generated or untrusted-derived text is executed."},
		{"JR-S06", "permission_bypass", CategorySecurity, "Widens or bypasses the agent's guardrails."},
		{"JR-S07", "tool_poisoning", CategorySecurity, "Tool, MCP or command descriptions carry instructions to the agent or shadow other tools."},
		{"JR-S08", "untrusted_to_privileged", CategorySecurity, "Attacker-writable content drives a privileged action without confirmation."},
		{"JR-S09", "persistence", CategorySecurity, "Survives the session or spreads to other scopes."},
		{"JR-S10", "supply_chain", CategorySecurity, "Executable code or instructions fetched from a mutable or unverified source."},
		{"JR-S11", "purpose_mismatch", CategorySecurity, "Behavior materially exceeds or contradicts the declared name and description."},
		{"JR-S12", "destructive_unconfirmed", CategorySecurity, "Irreversible or shared-state operations the agent runs without human approval."},
		{"JR-S13", "trigger_hijack", CategorySecurity, "Description or trigger text built to be selected over other skills or to run unasked."},
		{"JR-S14", "embedded_secret", CategorySecurity, "A live-looking credential literal."},
		{"JR-S15", "resource_abuse", CategorySecurity, "Unbounded loops, fork bombs or mining-like workloads the purpose does not need."},
		{"JR-Q01", "trigger_quality", CategoryQuality, "Description missing, vague or too broad to decide when to load the file."},
		{"JR-Q02", "scope_mismatch", CategoryQuality, "The body does materially more, less or other than the description says."},
		{"JR-Q03", "contradiction", CategoryQuality, "Two statements that cannot both be followed."},
		{"JR-Q04", "non_actionable", CategoryQuality, "An aspiration with no behavior or check, or an instruction the agent cannot carry out."},
		{"JR-Q05", "ambiguity", CategoryQuality, "An undefined term, dangling pronoun or unordered steps that need an order."},
		{"JR-Q06", "stale_reference", CategoryQuality, "A path, command, flag, skill, agent, tool or function that does not exist."},
		{"JR-Q07", "incorrect_example", CategoryQuality, "An example that contradicts the rule it illustrates or would not run."},
		{"JR-Q08", "verbosity", CategoryQuality, "Text that costs tokens on every load and changes no behavior."},
		{"JR-Q09", "misplaced_content", CategoryQuality, "Content in the wrong layer."},
		{"JR-Q10", "emphasis_abuse", CategoryQuality, "Capitals and absolutes on so many lines that nothing stands out."},
		{"JR-Q11", "no_done_criterion", CategoryQuality, "A procedure with no way to tell it worked."},
		{"JR-Q12", "overlap", CategoryQuality, "Trigger or content overlaps a sibling."},
		{"JR-Q13", "unwarned_side_effect", CategoryQuality, "A destructive or externally visible action with no word that the user should be told."},
		{"JR-G01", "violates_convention", CategoryGovernance, "The file does or says something an attached convention forbids, or omits something it requires."},
		{"JR-G02", "contradicts_parent", CategoryGovernance, "The file contradicts a nearer or farther CLAUDE.md or AGENTS.md on the same topic."},
		{"JR-G03", "restates_convention", CategoryGovernance, "The file copies an attached convention without adding anything."},
		{"JR-G04", "scope_mismatch", CategoryGovernance, "A rule's paths globs do not fit its content."},
		{"JR-G05", "conflicting_conventions", CategoryGovernance, "Two attached conventions disagree and the file cannot satisfy both."},
		{codeRubricFailure, "rubric_failure", CategoryGovernance, "A rubric criterion fails."},
		{"JR-G07", "artifact_standard", CategoryGovernance, "The file violates a written convention about artifacts themselves."},
	}
})

var codeIndex = sync.OnceValue(func() map[string]CodeInfo {
	table := codeTable()
	m := make(map[string]CodeInfo, len(table))
	for i := range table {
		m[table[i].Code] = table[i]
	}
	return m
})

// Codes lists every finding code.
func Codes() []CodeInfo { return append([]CodeInfo(nil), codeTable()...) }

// LookupCode returns the description of a code.
func LookupCode(code string) (CodeInfo, bool) {
	c, ok := codeIndex()[strings.ToUpper(strings.TrimSpace(code))]
	return c, ok
}

// CodesOf returns the codes of one category, in order.
func CodesOf(c Category) []string {
	var out []string
	table := codeTable()
	for i := range table {
		ci := &table[i]
		if ci.Category == c {
			out = append(out, ci.Code)
		}
	}
	return out
}

// CategoryOfCode returns the category a code belongs to.
func CategoryOfCode(code string) (Category, bool) {
	ci, ok := codeIndex()[code]
	return ci.Category, ok
}
