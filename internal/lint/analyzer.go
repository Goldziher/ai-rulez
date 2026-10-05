package lint

import (
	"sort"
	"strings"
)

// Every rule belongs to an analyzer (a family of related checks) and has a
// scope: what unit of input one finding is about.
//
//   - file:   one scanned text file (a line of prose, a script line)
//   - item:   one content item (a rule, skill, agent or command) or one config entry
//   - bundle: a relation between items, or the project as a whole
//
// The classification is a lookup table, not a plugin interface: the runner
// still executes every check, and `validate --analyzer` filters the report.
// That is enough to select a family in CI and to label SARIF rules; a
// per-analyzer execution model would be a rewrite of the runner.

// Analyzer names.
const (
	AnalyzerSecurity     = "security"
	AnalyzerReferences   = "references"
	AnalyzerHooks        = "hooks"
	AnalyzerMCP          = "mcp"
	AnalyzerDuplicates   = "duplicates"
	AnalyzerDescriptions = "descriptions"
	AnalyzerBudgets      = "budgets"
	AnalyzerMetadata     = "metadata"
	AnalyzerPlugin       = "plugin"
)

// Scopes.
const (
	ScopeFile   = "file"
	ScopeItem   = "item"
	ScopeBundle = "bundle"
)

// AnalyzerInfo is the classification of one rule.
type AnalyzerInfo struct{ Name, Scope string }

var analyzerOverrides = map[string]AnalyzerInfo{
	CodeSecretDetected: {AnalyzerSecurity, ScopeFile}, CodeHiddenCharacters: {AnalyzerSecurity, ScopeFile},
	CodeCommentInstruction: {AnalyzerSecurity, ScopeFile}, CodeInjectionPhrase: {AnalyzerSecurity, ScopeFile},
	CodeShellExec: {AnalyzerSecurity, ScopeFile}, CodeShellAccess: {AnalyzerSecurity, ScopeFile},
	CodeOutboundHost: {AnalyzerSecurity, ScopeFile}, CodeEncodedBlob: {AnalyzerSecurity, ScopeFile},
	CodeExternalFinding:  {AnalyzerSecurity, ScopeFile},
	CodeToolBreadth:      {AnalyzerSecurity, ScopeItem},
	CodeUnpinnedRemote:   {AnalyzerSecurity, ScopeBundle},
	CodeLinkUnresolved:   {AnalyzerReferences, ScopeFile},
	CodeAnchorUnresolved: {AnalyzerReferences, ScopeFile},
	CodePathMissing:      {AnalyzerReferences, ScopeFile}, CodeSkillResourceMissing: {AnalyzerReferences, ScopeFile},
	CodeReferenceUnknown: {AnalyzerReferences, ScopeFile},
	CodeHookMissing:      {AnalyzerHooks, ScopeBundle}, CodeHookNotExecutable: {AnalyzerHooks, ScopeBundle},
	CodeHookSourceMissing: {AnalyzerHooks, ScopeBundle}, CodeHookSourceNotExec: {AnalyzerHooks, ScopeBundle},
	CodeScriptNotExecutable: {AnalyzerHooks, ScopeItem}, CodePermissionOverbroad: {AnalyzerSecurity, ScopeBundle},
	CodeMCPCommandNotFound: {AnalyzerMCP, ScopeBundle},
	CodeDescriptionDup:     {AnalyzerDuplicates, ScopeBundle}, CodeDescriptionNearDup: {AnalyzerDuplicates, ScopeBundle},
	CodeDuplicateCollapsed: {AnalyzerDuplicates, ScopeBundle},
	CodeSizeLines:          {AnalyzerBudgets, ScopeItem}, CodeSizeTokens: {AnalyzerBudgets, ScopeItem},
	CodePluginVersionDrift: {AnalyzerPlugin, ScopeBundle}, CodeEvalsMissing: {AnalyzerPlugin, ScopeItem},
}

// SetAnalyzer classifies a rule, overriding the family default. Rule packages
// call it from an init function next to their registration.
func SetAnalyzer(code, analyzer, scope string) {
	analyzerOverrides[code] = AnalyzerInfo{Name: analyzer, Scope: scope}
}

// AnalyzerFor returns the analyzer and scope of a rule code. Codes without an
// explicit entry fall back on their family (the hundreds digit).
func AnalyzerFor(code string) AnalyzerInfo {
	if a, ok := analyzerOverrides[code]; ok {
		return a
	}
	if len(code) < 5 || !strings.HasPrefix(code, "AR") {
		return AnalyzerInfo{Name: AnalyzerReferences, Scope: ScopeItem}
	}
	family := map[byte]string{
		'0': AnalyzerSecurity, '1': AnalyzerReferences, '2': AnalyzerReferences, '3': AnalyzerReferences,
		'4': AnalyzerReferences, '5': AnalyzerHooks, '6': AnalyzerMCP, '7': AnalyzerDuplicates, '8': AnalyzerDescriptions,
		'9': AnalyzerBudgets,
	}[code[2]]
	if family == "" {
		family = AnalyzerReferences
	}
	if code[2] == '9' {
		switch code[3] { // AR95x metadata, AR96x and up plugin and project-level
		case '5':
			family = AnalyzerMetadata
		case '6', '7', '8', '9':
			family = AnalyzerPlugin
		}
	}
	return AnalyzerInfo{Name: family, Scope: ScopeItem}
}

// AnalyzerNames lists the analyzers that at least one registered rule uses.
func AnalyzerNames() []string {
	seen := map[string]bool{}
	for _, r := range registry {
		seen[AnalyzerFor(r.Code).Name] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// FilterAnalyzers keeps the findings of the named analyzers.
func FilterAnalyzers(r *Report, names []string) {
	if len(names) == 0 {
		return
	}
	want := map[string]bool{}
	for _, n := range names {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	kept := r.Findings[:0:0]
	for i := range r.Findings {
		if want[AnalyzerFor(r.Findings[i].Code).Name] {
			kept = append(kept, r.Findings[i])
		}
	}
	r.Findings = kept
}

// annotateAnalyzer records the analyzer and scope on a finding.
func annotateAnalyzer(f *Finding) {
	a := AnalyzerFor(f.Code)
	m := f.meta()
	m.Analyzer, m.Scope = a.Name, a.Scope
}
