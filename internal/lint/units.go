package lint

import (
	"fmt"
	"path"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// The runner is a list of units: an item check, a text scan or a whole-run
// check, each declaring the analyzers whose rules it can report. A run that
// selects analyzers (--analyzer, [lint] analyzers) executes only the units that
// declare one of them, so a security-only job does not read frontmatter for
// metadata checks, count tokens for budgets or resolve links. The result is the
// filtered result of a full run: the report is filtered by analyzer at the end,
// so a unit that also reports other analyzers' rules can never leak them.

// unitSpec names one unit and the analyzers it reports for.
type unitSpec struct {
	name      string
	analyzers []string
	// deps marks a unit that records the reference graph `--since` needs: it
	// still runs when Options.NeedDeps is set, its findings filtered out.
	deps bool
}

func unitOf(name string, analyzers ...string) unitSpec {
	return unitSpec{name: name, analyzers: analyzers}
}

func depUnitOf(name string, analyzers ...string) unitSpec {
	return unitSpec{name: name, analyzers: analyzers, deps: true}
}

// onUndeclaredEmission is set by tests: it is called when a unit reports a rule
// of an analyzer it did not declare, which would be skipped wrongly when only
// that analyzer is selected.
var onUndeclaredEmission func(unit, code string)

// parseSelection normalizes analyzer names into a set. nil means every analyzer.
func parseSelection(names []string) map[string]bool {
	var sel map[string]bool
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if sel == nil {
			sel = map[string]bool{}
		}
		sel[n] = true
	}
	return sel
}

// SelectedAnalyzers returns the sorted, normalized analyzer names, or nil when
// every analyzer runs.
func SelectedAnalyzers(names []string) []string {
	sel := parseSelection(names)
	if sel == nil {
		return nil
	}
	out := make([]string, 0, len(sel))
	for n := range sel {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// AnalyzerSelected reports whether an analyzer runs under a selection (an empty
// selection runs everything). Callers use it to skip inputs that only other
// analyzers need.
func AnalyzerSelected(selection []string, analyzers ...string) bool {
	sel := parseSelection(selection)
	if sel == nil {
		return true
	}
	for _, a := range analyzers {
		if sel[a] {
			return true
		}
	}
	return false
}

// ValidateAnalyzerNames returns the names that are not analyzers of any rule.
func ValidateAnalyzerNames(names []string) []string {
	known := AnalyzerNames()
	var bad []string
	for _, n := range names {
		if !slices.Contains(known, strings.ToLower(strings.TrimSpace(n))) {
			bad = append(bad, n)
		}
	}
	return bad
}

func (r *runner) selected(u unitSpec) bool {
	if r.sel == nil {
		return true
	}
	if u.deps && r.opts.NeedDeps {
		return true
	}
	for _, a := range u.analyzers {
		if r.sel[a] {
			return true
		}
	}
	return false
}

// unit runs fn when the unit's analyzers are selected.
func (r *runner) unit(u unitSpec, fn func()) {
	if !r.selected(u) {
		return
	}
	if r.units == nil {
		r.units = map[string]unitRun{}
	}
	run := r.units[u.name]
	run.analyzers, run.count = u.analyzers, run.count+1
	r.units[u.name] = run
	prev := r.cur
	r.cur = &u
	defer func() { r.cur = prev }()
	fn()
}

// audit reports a unit that emits a rule outside its declared analyzers.
func (r *runner) audit(code string) {
	if r.cur == nil || onUndeclaredEmission == nil {
		return
	}
	if !slices.Contains(r.cur.analyzers, AnalyzerFor(code).Name) {
		onUndeclaredEmission(r.cur.name, code)
	}
}

// unitRun is what one unit did in a run.
type unitRun struct {
	analyzers []string
	count     int
}

// funcName names a registered function for unit accounting.
func funcName(fn any) string {
	name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	return path.Base(name)[strings.Index(path.Base(name), ".")+1:]
}

func mustDeclare(kind string, fn any, analyzers []string) unitSpec {
	if len(analyzers) == 0 {
		panic(fmt.Sprintf("lint: %s %s registered without analyzers", kind, funcName(fn)))
	}
	return unitSpec{name: funcName(fn), analyzers: analyzers}
}
