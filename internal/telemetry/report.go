package telemetry

import (
	"bufio"
	"encoding/json"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// ItemRow is one item's row in the report.
type ItemRow struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Loads    int    `json:"loads"`
	Used     int    `json:"used,omitempty"`
	Sessions int    `json:"sessions,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
	// LoadReasons counts loads by reason (session_start, path_glob_match, ...).
	LoadReasons map[string]int `json:"load_reasons,omitempty"`
	// Eval is the recorded eval score when the eval store has one for this id.
	Eval *usage.EvalSummary `json:"eval,omitempty"`
}

// ItemGroup is the report for one kind.
type ItemGroup struct {
	Loaded []ItemRow `json:"loaded"`
	// Never lists catalog items with no load: the generated rules or agents
	// nothing ever read. Empty when there is no generated manifest.
	Never []ItemRow `json:"never_loaded"`
	// Unknown lists loaded items that are not in the catalog (user-level files,
	// hand-written rules, plugin agents).
	Unknown []ItemRow `json:"not_in_catalog"`
}

// Distribution summarizes a per-session count.
type Distribution struct {
	Sessions int     `json:"sessions"`
	Median   float64 `json:"median"`
	Mean     float64 `json:"mean"`
	Max      int     `json:"max"`
}

// ItemsReport is the rule, agent and context section of `telemetry report`.
type ItemsReport struct {
	Events   int `json:"events"`
	Sessions int `json:"sessions"`
	// EventsWithoutSession counts loads that cannot join a session statistic.
	EventsWithoutSession int       `json:"events_without_session,omitempty"`
	Rules                ItemGroup `json:"rules"`
	Agents               ItemGroup `json:"agents"`
	Contexts             ItemGroup `json:"contexts"`
	// RulesPerSession is the distribution of distinct rules loaded per session,
	// over sessions that loaded any rule or context file.
	RulesPerSession Distribution `json:"rules_per_session"`
	// LoadReasons counts loads by kind and reason.
	LoadReasons map[string]map[string]int `json:"load_reasons"`
}

// ReadItemEvents reads the item events of a usage log, skipping skill lines,
// feedback lines and anything unreadable.
func ReadItemEvents(path string) ([]Event, error) {
	file, err := safefs.OpenRegular(path)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "open usage log")
	}
	defer file.Close() //nolint:errcheck // read-only
	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Name != EventItem || event.ID == "" || event.Kind == "" {
			continue
		}
		events = append(events, event)
	}
	return events, oops.Wrapf(scanner.Err(), "read usage log")
}

type tallyKey struct{ kind, id string }

type tally struct {
	loads, used int
	sessions    map[string]bool
	last        string
	reasons     map[string]int
}

// tallyEvents counts events per item and fills the session-level statistics of
// the report.
func tallyEvents(events []Event, report *ItemsReport) (tallies map[tallyKey]*tally, rulesBySession map[string]map[string]bool) {
	tallies, rulesBySession = map[tallyKey]*tally{}, map[string]map[string]bool{}
	allSessions := map[string]bool{}
	for i := range events {
		e := &events[i]
		k := tallyKey{e.Kind, e.ID}
		t := tallies[k]
		if t == nil {
			t = &tally{sessions: map[string]bool{}, reasons: map[string]int{}}
			tallies[k] = t
		}
		if e.Time > t.last {
			t.last = e.Time
		}
		switch e.Outcome {
		case OutcomeUsed:
			t.used++
		case OutcomeLoaded:
			t.loads++
			countReason(report, t, e)
			if e.Session == "" {
				report.EventsWithoutSession++
				continue
			}
			t.sessions[e.Session] = true
			allSessions[e.Session] = true
			noteSessionRules(rulesBySession, e)
		}
	}
	report.Sessions = len(allSessions)
	return tallies, rulesBySession
}

func countReason(report *ItemsReport, t *tally, e *Event) {
	if e.LoadReason == "" {
		return
	}
	t.reasons[e.LoadReason]++
	if report.LoadReasons[e.Kind] == nil {
		report.LoadReasons[e.Kind] = map[string]int{}
	}
	report.LoadReasons[e.Kind][e.LoadReason]++
}

// noteSessionRules registers the session for the rules-per-session statistic: a
// session that loaded a context file but no rule counts as zero rules.
func noteSessionRules(bySession map[string]map[string]bool, e *Event) {
	if e.Kind != KindRule && e.Kind != KindContext {
		return
	}
	if bySession[e.Session] == nil {
		bySession[e.Session] = map[string]bool{}
	}
	if e.Kind == KindRule {
		bySession[e.Session][e.ID] = true
	}
}

func withEval(row ItemRow, scores map[string]usage.EvalSummary) ItemRow {
	if s, ok := scores[row.ID]; ok {
		summary := s
		row.Eval = &summary
	}
	return row
}

// buildGroup makes the loaded, never-loaded and not-in-catalog lists of one kind.
func buildGroup(kind string, known []string, tallies map[tallyKey]*tally, scores map[string]usage.EvalSummary) ItemGroup {
	group := ItemGroup{Loaded: []ItemRow{}, Never: []ItemRow{}, Unknown: []ItemRow{}}
	inCatalog := map[string]bool{}
	for _, id := range known {
		inCatalog[id] = true
	}
	for k, t := range tallies {
		if k.kind != kind || k.id == ListID || (t.loads == 0 && t.used == 0) {
			continue
		}
		row := ItemRow{Kind: kind, ID: k.id, Loads: t.loads, Used: t.used, Sessions: len(t.sessions), LastSeen: t.last}
		if len(t.reasons) > 0 {
			row.LoadReasons = t.reasons
		}
		row = withEval(row, scores)
		group.Loaded = append(group.Loaded, row)
		if len(known) > 0 && !inCatalog[k.id] {
			group.Unknown = append(group.Unknown, row)
		}
	}
	for _, id := range known {
		if t := tallies[tallyKey{kind, id}]; t == nil || t.loads == 0 {
			group.Never = append(group.Never, withEval(ItemRow{Kind: kind, ID: id}, scores))
		}
	}
	sortRows(group.Loaded, true)
	sortRows(group.Unknown, true)
	sortRows(group.Never, false)
	return group
}

// BuildItemsReport joins item events with the catalog. Output is sorted and
// deterministic. scores, when non-nil, attaches eval summaries by id.
func BuildItemsReport(events []Event, catalog *Catalog, scores map[string]usage.EvalSummary) *ItemsReport {
	if catalog == nil {
		catalog = &Catalog{}
	}
	report := &ItemsReport{Events: len(events), LoadReasons: map[string]map[string]int{}}
	tallies, rulesBySession := tallyEvents(events, report)
	report.RulesPerSession = distribution(rulesBySession)
	report.Rules = buildGroup(KindRule, catalog.Rules, tallies, scores)
	report.Agents = buildGroup(KindAgent, catalog.Agents, tallies, scores)
	report.Contexts = buildGroup(KindContext, catalog.Contexts, tallies, scores)
	return report
}

func distribution(bySession map[string]map[string]bool) Distribution {
	if len(bySession) == 0 {
		return Distribution{}
	}
	counts := make([]int, 0, len(bySession))
	total := 0
	for _, rules := range bySession {
		counts = append(counts, len(rules))
		total += len(rules)
	}
	sort.Ints(counts)
	d := Distribution{Sessions: len(counts), Max: counts[len(counts)-1], Mean: round2(float64(total) / float64(len(counts)))}
	mid := len(counts) / 2
	if len(counts)%2 == 1 {
		d.Median = float64(counts[mid])
	} else {
		d.Median = float64(counts[mid-1]+counts[mid]) / 2
	}
	return d
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func sortRows(rows []ItemRow, byLoads bool) {
	sort.SliceStable(rows, func(a, b int) bool {
		if byLoads && rows[a].Loads != rows[b].Loads {
			return rows[a].Loads > rows[b].Loads
		}
		return rows[a].ID < rows[b].ID
	})
}
