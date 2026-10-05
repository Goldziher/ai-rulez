package lint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The risk score is advisory. It summarizes how much unresolved trouble an item
// (one source file) or a bundle (one root, or the whole run) carries, so a
// reviewer can triage; it never changes the exit code, which stays a threshold
// on finding severity (--fail-on). Each unaccepted finding adds the weight of
// its severity, the sum is capped at 100, and the label is the larger of the
// score's band and a floor set by the worst finding, so one error is never
// labeled "low".

// Risk labels, lowest first.
const (
	RiskClean    = "clean"
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// RiskMax caps a score.
const RiskMax = 100

// RiskWeights are the points one finding of each severity adds.
type RiskWeights struct {
	Error, Warning, Info int
}

// DefaultRiskWeights are the weights used when [lint.risk] sets none.
func DefaultRiskWeights() RiskWeights { return RiskWeights{Error: 25, Warning: 8, Info: 1} }

// RiskWeightsFrom applies the [lint.risk] overrides to the defaults.
func RiskWeightsFrom(c *config.LintRisk) RiskWeights {
	w := DefaultRiskWeights()
	if c == nil {
		return w
	}
	if c.Error != nil {
		w.Error = *c.Error
	}
	if c.Warning != nil {
		w.Warning = *c.Warning
	}
	if c.Info != nil {
		w.Info = *c.Info
	}
	return w
}

func (w RiskWeights) of(s Severity) int {
	switch s {
	case SeverityError:
		return w.Error
	case SeverityWarning:
		return w.Warning
	case SeverityInfo:
		return w.Info
	default:
		return 0
	}
}

// Risk is a score with its label.
type Risk struct {
	Score    int    `json:"score"`
	Label    string `json:"label"`
	Findings int    `json:"findings"`
}

// ItemRisk is the risk of one source file.
type ItemRisk struct {
	Item string `json:"item"`
	Risk
}

// RootRisk is the risk of one root.
type RootRisk struct {
	Root string `json:"root"`
	Risk
}

// RiskReport is the advisory risk of a report: the whole bundle and each item.
type RiskReport struct {
	Bundle Risk       `json:"bundle"`
	Items  []ItemRisk `json:"items,omitempty"`
}

func labelRank(label string) int {
	switch label {
	case RiskCritical:
		return 4
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	case RiskLow:
		return 1
	default:
		return 0
	}
}

func labelByScore(score int) string {
	switch {
	case score <= 0:
		return RiskClean
	case score <= 25:
		return RiskLow
	case score <= 50:
		return RiskMedium
	case score <= 75:
		return RiskHigh
	default:
		return RiskCritical
	}
}

func floorLabel(worst Severity) string {
	switch worst {
	case SeverityError:
		return RiskHigh
	case SeverityWarning:
		return RiskMedium
	case SeverityInfo:
		return RiskLow
	default:
		return RiskClean
	}
}

func maxLabel(a, b string) string {
	if labelRank(b) > labelRank(a) {
		return b
	}
	return a
}

type riskAcc struct {
	sum, n int
	worst  Severity
}

func (a *riskAcc) add(f *Finding, w RiskWeights) {
	a.sum += w.of(f.Severity)
	a.n++
	if f.Severity.rank() > a.worst.rank() {
		a.worst = f.Severity
	}
}

func (a riskAcc) risk() Risk {
	score := min(max(a.sum, 0), RiskMax)
	label := RiskClean
	if a.n > 0 {
		label = maxLabel(labelByScore(score), floorLabel(a.worst))
	}
	return Risk{Score: score, Label: label, Findings: a.n}
}

// ComputeRisk scores findings; accepted ones (a baseline) are not counted.
// Items are ordered riskiest first, then by path.
func ComputeRisk(findings []Finding, w RiskWeights) RiskReport {
	var bundle riskAcc
	items := map[string]*riskAcc{}
	for i := range findings {
		f := &findings[i]
		if f.IsAccepted() || f.Severity.rank() == 0 {
			continue
		}
		bundle.add(f, w)
		path := f.RepoPath()
		if items[path] == nil {
			items[path] = &riskAcc{}
		}
		items[path].add(f, w)
	}
	out := RiskReport{Bundle: bundle.risk()}
	for path, acc := range items {
		out.Items = append(out.Items, ItemRisk{Item: path, Risk: acc.risk()})
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Item < b.Item
	})
	return out
}

// CombinedRisk is the advisory risk of a whole run.
type CombinedRisk struct {
	Bundle Risk       `json:"bundle"`
	Roots  []RootRisk `json:"roots,omitempty"`
	Items  []ItemRisk `json:"items,omitempty"`
}

// combineRisk merges per-root risks: the bundle score is the capped sum of the
// roots' scores and its label the worst label.
func combineRisk(reports []*Report) *CombinedRisk {
	var out CombinedRisk
	seen := false
	for _, r := range reports {
		if r.Risk == nil {
			continue
		}
		seen = true
		out.Roots = append(out.Roots, RootRisk{Root: r.Root, Risk: r.Risk.Bundle})
		out.Bundle.Score = min(out.Bundle.Score+r.Risk.Bundle.Score, RiskMax)
		out.Bundle.Findings += r.Risk.Bundle.Findings
		out.Bundle.Label = maxLabel(out.Bundle.Label, r.Risk.Bundle.Label)
		out.Items = append(out.Items, r.Risk.Items...)
	}
	if !seen {
		return nil
	}
	if out.Bundle.Label == "" {
		out.Bundle.Label = RiskClean
	}
	if len(out.Roots) == 1 {
		out.Roots = nil // the bundle is the root
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].Score > out.Items[j].Score })
	return &out
}

const riskTopItems = 5

func riskLine(r Risk) string {
	return fmt.Sprintf("%s (score %d/%d)", r.Label, r.Score, RiskMax)
}

// writeRiskText adds the advisory risk block to the text report.
func writeRiskText(sb *strings.Builder, c Combined) {
	if c.Risk == nil || c.Risk.Bundle.Findings == 0 {
		return
	}
	fmt.Fprintf(sb, "risk (advisory, never fails the run): %s\n", riskLine(c.Risk.Bundle))
	for i, it := range c.Risk.Items {
		if i == riskTopItems {
			fmt.Fprintf(sb, "  ... and %d more item(s)\n", len(c.Risk.Items)-riskTopItems)
			break
		}
		fmt.Fprintf(sb, "  %-9s %3d  %s\n", it.Label, it.Score, it.Item)
	}
}

func writeRiskMarkdown(sb *strings.Builder, c Combined) {
	if c.Risk == nil || c.Risk.Bundle.Findings == 0 {
		return
	}
	fmt.Fprintf(sb, "\nRisk (advisory, does not block): **%s**\n\n| Item | Risk | Score |\n| --- | --- | --- |\n", riskLine(c.Risk.Bundle))
	for i, it := range c.Risk.Items {
		if i == riskTopItems {
			break
		}
		fmt.Fprintf(sb, "| `%s` | %s | %d |\n", it.Item, it.Label, it.Score)
	}
}
