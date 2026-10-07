package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// reportItems forces the rule/agent/context section of `telemetry report` even when
// the log holds no item events (it then lists the generated rules nothing loaded).
var reportItems bool

// usageReportJSON is the `telemetry report --format json` document: the skill report as it
// has always been, plus an "items" object when item events are present.
type usageReportJSON struct {
	*usage.Report
	Items *telemetry.ItemsReport `json:"items,omitempty"`
}

// itemsSection builds the rule, agent and context section for a log. It returns
// nil when the log has no item events and --items was not given, so the output of
// `telemetry report` is unchanged for a project that does not use item telemetry.
func itemsSection(logPath string) (*telemetry.ItemsReport, error) {
	events, err := telemetry.ReadItemEvents(logPath)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 && !reportItems {
		return nil, nil
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	catalog, err := telemetry.LoadCatalog(root, configDirName())
	if err != nil {
		return nil, fmt.Errorf("read generated manifest: %w", err)
	}
	scores, err := loadEvalSummaries(reportEvals)
	if err != nil {
		return nil, err
	}
	return telemetry.BuildItemsReport(events, catalog, scores), nil
}

func writeItemsReport(w reportWriter, items *telemetry.ItemsReport) {
	w.printf("\nItem loads: %d events in %d sessions", items.Events, items.Sessions)
	if items.EventsWithoutSession > 0 {
		w.printf(" (%d without a session)", items.EventsWithoutSession)
	}
	w.printf("\n")
	rps := items.RulesPerSession
	w.printf("Rules per session: median %.1f, mean %.2f, max %d (over %d sessions)\n", rps.Median, rps.Mean, rps.Max, rps.Sessions)

	group := func(title string, g telemetry.ItemGroup) {
		w.printf("\n%s loaded (%d)\n", title, len(g.Loaded))
		for _, row := range g.Loaded {
			w.printf("  %-40s %6d loads  %4d sessions  last %s%s%s\n", truncate(row.ID, 40), row.Loads, row.Sessions, row.LastSeen, reasonText(row.LoadReasons), itemEval(row))
		}
		w.printf("%s never loaded (%d)\n", title, len(g.Never))
		for _, row := range g.Never {
			w.printf("  %s%s\n", row.ID, itemEval(row))
		}
		if len(g.Unknown) > 0 {
			w.printf("%s not in the generated manifest (%d)\n", title, len(g.Unknown))
			for _, row := range g.Unknown {
				w.printf("  %-40s %6d loads\n", truncate(row.ID, 40), row.Loads)
			}
		}
	}
	group("Rules", items.Rules)
	group("Agents", items.Agents)
	group("Context files", items.Contexts)

	kinds := make([]string, 0, len(items.LoadReasons))
	for kind := range items.LoadReasons {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	w.printf("\nLoad reasons\n")
	for _, kind := range kinds {
		w.printf("  %-8s %s\n", kind, strings.TrimSpace(reasonText(items.LoadReasons[kind])))
	}
}

func reasonText(reasons map[string]int) string {
	if len(reasons) == 0 {
		return ""
	}
	names := make([]string, 0, len(reasons))
	for name := range reasons {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, reasons[name]))
	}
	return "  [" + strings.Join(parts, " ") + "]"
}

func itemEval(row telemetry.ItemRow) string {
	if row.Eval == nil {
		return ""
	}
	return fmt.Sprintf("  eval: %.0f%%", row.Eval.PassRate*100)
}

func init() {
	telemetryReportCmd.Flags().BoolVar(&reportItems, "items", false, "Always include the rule, agent and context section (default: only when the log holds item events)")
}
