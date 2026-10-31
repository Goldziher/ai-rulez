package exposure

import "sort"

// SchemaReport is the schema_version of a Report, whose JSON is described by
// schema/exposure-report.v1.schema.json.
const SchemaReport = 1

// The skill listing budget of Claude Code: 1% of the context window, counted
// at 4 characters per token.
const (
	ListingBudgetPercent = 1
	ListingCharsPerToken = 4
)

// ListingBudgetChars is the character budget of the skill listing of a model
// with a contextWindow-token window: 8000 at 200000.
func ListingBudgetChars(contextWindow int) int {
	return contextWindow * ListingCharsPerToken * ListingBudgetPercent / 100
}

// ListingBudget is the skill listing of a session against Claude Code's budget.
// Over budget, Claude Code keeps every skill name but drops descriptions, the
// text that carries the trigger words.
type ListingBudget struct {
	ContextWindow int `json:"context_window"`
	// Skills and Chars are the start-of-session skills and the characters of
	// their listing lines.
	Skills int `json:"skills"`
	Chars  int `json:"chars"`
	// BudgetChars is ListingBudgetChars(ContextWindow).
	BudgetChars int `json:"budget_chars"`
	// OverBudgetChars is how far Chars is past BudgetChars, 0 when it fits.
	OverBudgetChars int `json:"over_budget_chars"`
	// DescriptionsCapped counts the listed skills whose description and
	// when_to_use exceed the per-skill listing cap.
	DescriptionsCapped int `json:"descriptions_capped"`
}

// ReportItem is one artifact of a Report: when it reaches the model and its
// estimated tokens.
type ReportItem struct {
	Kind Kind `json:"kind"`
	// Name is the artifact's qualified name (`plugin:skill` for a plugin skill).
	Name   string `json:"name"`
	Path   string `json:"path"`
	Scope  string `json:"scope"`
	Phase  Phase  `json:"phase"`
	Tokens int    `json:"tokens"`
	// Area is the artifact's area when the inventory was classified.
	Area string `json:"area,omitempty"`
}

// Report is the start-of-session exposure of an inventory, plus what waits on
// demand or for the user. It counts an artifact of PhaseUnknown as start
// (fail closed, as Inventory.Start does) in StartTokens, StartTokensByKind and
// Listing; TokensByPhase and CountsByPhaseAndKind keep it under its own phase.
type Report struct {
	SchemaVersion int           `json:"schema_version"`
	Harness       string        `json:"harness"`
	Root          string        `json:"root"`
	Plugins       PluginSummary `json:"plugins"`
	// StartBudget is the [exposure] start_budget the report was checked
	// against, 0 when there is none.
	StartBudget int           `json:"start_budget"`
	Listing     ListingBudget `json:"listing"`
	StartTokens int           `json:"start_tokens"`
	// StartImportsUnmodeled lists the `@path` imports of start-of-session
	// instruction files that could not be modeled (an absolute or home path, a
	// path that leaves the root, or an unreadable target). With a start budget
	// set, any entry fails the budget closed even when StartTokens fits.
	StartImportsUnmodeled []UnmodeledImport `json:"start_imports_unmodeled"`
	// TokensByPhase sums the tokens of the phases that have artifacts.
	TokensByPhase     map[Phase]int `json:"tokens_by_phase"`
	StartTokensByKind map[Kind]int  `json:"start_tokens_by_kind"`
	// CountsByPhaseAndKind always holds start, on_demand and manual, and
	// unknown when an artifact has that phase.
	CountsByPhaseAndKind map[Phase]map[Kind]int `json:"counts_by_phase_and_kind"`
	Items                []ReportItem           `json:"items"`
}

// ReportOptions configure NewReport.
type ReportOptions struct {
	// ContextWindow is the model context window in tokens; it must be positive.
	ContextWindow int
	// StartBudget is the most start tokens allowed, 0 for no limit.
	StartBudget int
}

// UnmodeledImport is a start-of-session `@path` import the inventory could not
// follow, and why. A configured start budget fails closed on one.
type UnmodeledImport struct {
	// Artifact is the file that writes the import, relative to the root.
	Artifact string `json:"artifact"`
	// Import is the directive as written, or the artifact path when the file
	// itself could not be read.
	Import string `json:"import"`
	Reason string `json:"reason"`
}

// NewReport measures the exposure of inv.
func NewReport(inv *Inventory, opts ReportOptions) *Report {
	rep := &Report{
		SchemaVersion:         SchemaReport,
		Harness:               inv.Harness,
		Root:                  inv.Root,
		Plugins:               inv.Plugins,
		StartBudget:           opts.StartBudget,
		Listing:               ListingBudget{ContextWindow: opts.ContextWindow, BudgetChars: ListingBudgetChars(opts.ContextWindow)},
		TokensByPhase:         map[Phase]int{},
		StartTokensByKind:     map[Kind]int{},
		CountsByPhaseAndKind:  map[Phase]map[Kind]int{PhaseStart: {}, PhaseOnDemand: {}, PhaseManual: {}},
		Items:                 make([]ReportItem, 0, len(inv.Artifacts)),
		StartImportsUnmodeled: []UnmodeledImport{},
	}
	for i := range inv.Artifacts {
		a := &inv.Artifacts[i]
		rep.Items = append(rep.Items, ReportItem{Kind: a.Kind, Name: a.QualifiedName, Path: a.Path, Scope: a.Scope, Phase: a.Phase, Tokens: a.Tokens, Area: a.Area})
		rep.TokensByPhase[a.Phase] += a.Tokens
		if rep.CountsByPhaseAndKind[a.Phase] == nil {
			rep.CountsByPhaseAndKind[a.Phase] = map[Kind]int{}
		}
		rep.CountsByPhaseAndKind[a.Phase][a.Kind]++
		if !atStart(a.Phase) {
			continue
		}
		rep.StartTokens += a.Tokens
		rep.StartTokensByKind[a.Kind] += a.Tokens
		for _, p := range a.ImportProblems {
			rep.StartImportsUnmodeled = append(rep.StartImportsUnmodeled, UnmodeledImport{Artifact: a.Path, Import: p.Path, Reason: p.Reason})
		}
		if a.Kind == KindSkill {
			rep.Listing.Skills++
			rep.Listing.Chars += a.ListingChars
			if a.Truncated {
				rep.Listing.DescriptionsCapped++
			}
		}
	}
	rep.Listing.OverBudgetChars = max(0, rep.Listing.Chars-rep.Listing.BudgetChars)
	return rep
}

// OverStartBudget reports a start budget that the start tokens exceed, or that
// cannot be verified because a start-of-session instruction import could not be
// modeled (fail closed).
func (r *Report) OverStartBudget() bool {
	return r.StartBudget > 0 && (r.StartTokens > r.StartBudget || len(r.StartImportsUnmodeled) > 0)
}

// StartArtifacts counts the artifacts at session start, unknown included.
func (r *Report) StartArtifacts() int {
	n := 0
	for _, it := range r.Items {
		if atStart(it.Phase) {
			n++
		}
	}
	return n
}

// Heaviest is up to n start-of-session items (unknown included), most tokens
// first, in inventory order on a tie.
func (r *Report) Heaviest(n int) []ReportItem {
	var start []ReportItem
	for _, it := range r.Items {
		if atStart(it.Phase) {
			start = append(start, it)
		}
	}
	sort.SliceStable(start, func(i, j int) bool { return start[i].Tokens > start[j].Tokens })
	return start[:min(n, len(start))]
}

// atStart reports a phase a session guard counts at start: PhaseStart, and
// PhaseUnknown, which fails closed.
func atStart(p Phase) bool { return p == PhaseStart || p == PhaseUnknown }
