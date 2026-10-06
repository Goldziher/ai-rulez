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
// The classification is an explicit table: every registered code is listed
// under its analyzer (TestEveryRegisteredCodeHasAnExplicitAnalyzer fails for a
// code that is not). `validate --analyzer` filters the report by it.

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
	AnalyzerConfig       = "config"
	AnalyzerRoles        = "roles"
	AnalyzerLock         = "lock"
	AnalyzerDelivery     = "delivery"
	AnalyzerEvals        = "evals"
	AnalyzerOKF          = "okf"
	AnalyzerTraps        = "traps"
	AnalyzerConvert      = "convert"
)

// Scopes.
const (
	ScopeFile   = "file"
	ScopeItem   = "item"
	ScopeBundle = "bundle"
)

// AnalyzerInfo is the classification of one rule.
type AnalyzerInfo struct{ Name, Scope string }

// analyzerGroup lists the codes of one analyzer with one scope.
type analyzerGroup struct {
	name, scope string
	codes       []string
}

// analyzerGroups is the classification table. Codes that carry a scope other
// than the one of their family (a file-level security rule, a bundle-level
// hook rule) are listed in their own group.
var analyzerGroups = []analyzerGroup{
	{AnalyzerSecurity, ScopeFile, []string{
		"AR001", "AR002", "AR003", "AR004", "AR005", "AR006", "AR008", "AR009", "AR011",
	}},
	{AnalyzerSecurity, ScopeItem, []string{
		"AR007", "AR012", "AR013", "AR014", "AR015", "AR016", "AR017", "AR018", "AR019", "AR020", "AR021", "AR022",
		"AR023", "AR024", "AR025", "AR026", "AR027", "AR028", "AR029", "AR030", "AR031", "AR032", "AR033", "AR034",
	}},
	// Project-level security: supply chain, permissions, scanner egress and
	// the trust rule of the user-only [llm] and [telemetry] keys.
	{AnalyzerSecurity, ScopeBundle, []string{
		"AR010", "AR506", "AR9E0", "AR9E1", "AR9E2", "AR9E3", "AR9E4", "AR9E5", "AR9E6", "AR9E7", "AR9K1", "AR9L1",
	}},
	{AnalyzerReferences, ScopeFile, []string{"AR201", "AR202", "AR301", "AR401", "AR402"}},
	{AnalyzerReferences, ScopeItem, []string{"AR101", "AR210", "AR302", "AR303", "AR304", "AR305", "AR403"}},
	{AnalyzerHooks, ScopeBundle, []string{"AR501", "AR502", "AR504", "AR505"}},
	{AnalyzerHooks, ScopeItem, []string{"AR503", "AR507"}},
	{AnalyzerMCP, ScopeBundle, []string{"AR601"}},
	{AnalyzerMCP, ScopeItem, []string{"AR602"}},
	{AnalyzerDuplicates, ScopeBundle, []string{"AR701", "AR702", "AR703"}},
	{AnalyzerDescriptions, ScopeItem, []string{"AR801", "AR802", "AR803", "AR804", "AR805", "AR806", "AR807"}},
	{AnalyzerBudgets, ScopeItem, []string{"AR901", "AR902"}},
	{AnalyzerMetadata, ScopeItem, []string{"AR951", "AR952", "AR953", "AR954"}},
	{AnalyzerPlugin, ScopeBundle, []string{"AR961"}},
	{AnalyzerPlugin, ScopeItem, []string{"AR962", "AR963", "AR964"}},
	{AnalyzerRoles, ScopeItem, []string{"AR971", "AR972", "AR973"}},
	// Drift against ai-rulez.lock, including the served-skill lock.
	{AnalyzerLock, ScopeBundle, []string{"AR981", "AR982", "AR995"}},
	{AnalyzerDelivery, ScopeItem, []string{"AR989", "AR990", "AR991", "AR992", "AR993", "AR994"}},
	{AnalyzerEvals, ScopeItem, []string{"AR996", "AR997", "AR998", "AR9A0"}},
	{AnalyzerOKF, ScopeItem, []string{"AR9B0", "AR9B1", "AR9B2", "AR9B3", "AR9B4", "AR9B5", "AR9B6", "AR9B7", "AR9B8", "AR9B9"}},
	{AnalyzerTraps, ScopeFile, []string{"AR9C0", "AR9C1", "AR9C2", "AR9C3", "AR9C4", "AR9C5", "AR9C6", "AR9C7", "AR9C8", "AR9C9", "AR9CA"}},
	// Invalid [telemetry] and [llm] tables.
	{AnalyzerConfig, ScopeBundle, []string{"AR9K0", "AR9L0", "AR9G8"}},
	// Review dimensions: reported by `ai-rulez review`, never by validate.
	{AnalyzerDescriptions, ScopeItem, []string{"AR9G0", "AR9G1", "AR9G2", "AR9G3", "AR9G4", "AR9G5", "AR9G6", "AR9G7", "AR9G9"}},
	{AnalyzerConvert, ScopeItem, []string{"AR9F0", "AR9F1", "AR9F2", "AR9F3", "AR9F4", "AR9F5"}},
	{AnalyzerEvals, ScopeItem, []string{"AR9J1", "AR9J2", "AR9J3"}}, // improve report codes
}

var analyzerOverrides = buildAnalyzerTable()

func buildAnalyzerTable() map[string]AnalyzerInfo {
	table := map[string]AnalyzerInfo{}
	for _, g := range analyzerGroups {
		for _, code := range g.codes {
			table[code] = AnalyzerInfo{g.name, g.scope}
		}
	}
	return table
}

// SetAnalyzer classifies a rule that is not in the table above. Rule packages
// may call it from an init function next to their registration.
func SetAnalyzer(code, analyzer, scope string) {
	analyzerOverrides[code] = AnalyzerInfo{Name: analyzer, Scope: scope}
}

// AnalyzerFor returns the analyzer and scope of a rule code. Only codes that
// are not registered fall back on their family (the hundreds digit): every
// registered code has an explicit entry.
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
