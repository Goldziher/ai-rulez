package lint

import (
	"maps"
	"slices"
	"sync"
)

// Rule families added after the first strict-validation release register
// themselves from init() in their own file: a RuleInfo for the code registry,
// plus any of a per-item check, a per-text scan or a whole-run check. The
// runner calls the hooks at fixed points, so a new rule never edits lint.go.

type (
	// itemCheck runs once per owned, non-document item with its parsed frontmatter.
	itemCheck func(r *runner, it *item, d doc, fm frontmatter)
	// textScan runs once per scanned text (content, markdown resource, shipped script).
	textScan func(r *runner, t *scanText)
	// runCheck runs once per run, after every item has been checked.
	runCheck func(r *runner)
)

// registered pairs a hook with the unit that gates it.
type registered[F any] struct {
	unit unitSpec
	fn   F
}

// ruleSet is the registry of rule families while it is being built.
type ruleSet struct {
	rules      []RuleInfo
	docs       map[string]RuleDoc
	itemChecks []registered[itemCheck]
	textScans  []registered[textScan]
	runChecks  []registered[runCheck]
}

// ruleFamilies lists, in the order they register, the files that add rules after
// the first release (the order is the file-name order init functions ran in, so
// the registry is the same as it was).
func ruleFamilies() []func(*ruleSet) {
	return []func(*ruleSet){
		registerActivationcodes,
		registerAgentPlugins,
		registerApprovals,
		registerArActivationCases,
		registerArBudget,
		registerArCapability,
		registerArCmdrisk,
		registerArCommands,
		registerArCredtable,
		registerArExfil,
		registerArFmcomponents,
		registerArFrontmatter,
		registerArHooks,
		registerArImports,
		registerArInvocation,
		registerArMarkdown,
		registerArMcp,
		registerArPlugin,
		registerArPrompt,
		registerArQuality,
		registerArReview,
		registerArTaint,
		registerArTrust,
		registerConvertcodes,
		registerDelivery,
		registerEvalcheck,
		registerImprovecodes,
		registerLLMsTxt,
		registerOkf,
		registerPolicycodes,
		registerPublishcodes,
		registerRuledocsAdded,
		registerSbomcodes,
		registerSearchcodes,
		registerSemvercodes,
		registerServeScan,
		registerSigning,
		registerTelemetrycheck,
		registerTrapsLimits,
		registerTrapsPlan,
		registerVerifiercodes,
	}
}

// The tables are derived from the base tables and the families on first use, not
// by an init function: a check function reads them through lookupRule, so a
// package variable initialized from the families would be an initialization cycle.
var (
	tablesOnce sync.Once
	tables     *ruleSet
)

// ruleTables returns the registry, built once.
//
// A family function runs inside tablesOnce.Do, so it must not call ruleTables,
// AnalyzerFor or anything else that reads the registry: sync.Once would deadlock
// on the re-entrant call. A family only adds to the set it is given.
func ruleTables() *ruleSet {
	tablesOnce.Do(func() { tables = buildRuleSet() })
	return tables
}

// addRules adds rules to the code registry.
func (s *ruleSet) addRules(infos ...RuleInfo) { s.rules = append(s.rules, infos...) }

// addDocs adds the long-form explanation of rules by code. Every registered code
// needs one (TestEveryRuleHasDocs).
func (s *ruleSet) addDocs(docs map[string]RuleDoc) {
	for code, d := range docs {
		s.docs[code] = d
	}
}

// The add functions take the analyzers whose rules the hook can report, so a run
// that selects other analyzers skips it (see units.go).
func (s *ruleSet) addItemCheck(fn itemCheck, analyzers ...string) {
	s.itemChecks = append(s.itemChecks, registered[itemCheck]{mustDeclare("item check", fn, analyzers), fn})
}

func (s *ruleSet) addTextScan(fn textScan, analyzers ...string) {
	s.textScans = append(s.textScans, registered[textScan]{mustDeclare("text scan", fn, analyzers), fn})
}

func (s *ruleSet) addRunCheck(fn runCheck, analyzers ...string) {
	s.runChecks = append(s.runChecks, registered[runCheck]{mustDeclare("run check", fn, analyzers), fn})
}

// buildRuleSet returns the base rules and documentation with every family added.
func buildRuleSet() *ruleSet {
	s := &ruleSet{rules: slices.Clone(baseRegistry), docs: maps.Clone(baseRuleDocs)}
	for _, family := range ruleFamilies() {
		family(s)
	}
	return s
}

func (r *runner) runItemChecks(it *item, d doc, fm frontmatter) {
	for _, c := range ruleTables().itemChecks {
		r.unit(c.unit, func() { c.fn(r, it, d, fm) })
	}
}

func (r *runner) runTextScans(abs, raw string) {
	if len(ruleTables().textScans) == 0 {
		return
	}
	var t *scanText
	for _, c := range ruleTables().textScans {
		if !r.selected(c.unit) {
			continue
		}
		if t == nil {
			t = newScanText(r, abs, raw)
		}
		r.unit(c.unit, func() { c.fn(r, t) })
	}
}

func (r *runner) runRunChecks() {
	for _, c := range ruleTables().runChecks {
		r.unit(c.unit, func() { c.fn(r) })
	}
}

// severityOverridden reports whether [lint.severity] names the code.
func (r *runner) severityOverridden(code string) bool {
	for key := range r.lc.Severity {
		if rule, ok := lookupRule(key); ok && rule.Code == code {
			return true
		}
	}
	return false
}

// addSev is add with a severity that depends on the case: a rule whose default
// is a warning can still report its mild cases as info. A [lint.severity]
// entry or `off` by default always wins over the natural severity.
func (r *runner) addSev(natural Severity, code, abs string, line int, format string, args ...any) {
	saved := r.sev[code]
	if saved != SeverityOff && !r.severityOverridden(code) {
		r.sev[code] = natural
	}
	r.add(code, abs, line, format, args...)
	r.sev[code] = saved
}
