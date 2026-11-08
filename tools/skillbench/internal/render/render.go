// Package render writes the published tables into README and docs between markers, from a committed
// summary, so the numbers in prose cannot drift from the data.
package render

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/score"
)

// DefaultBlock names the holdout table's block. Other blocks, such as the dev and real-skill tables,
// are named by the operator.
const DefaultBlock = "skillbench"

func markers(block string) (start, end string) {
	return "<!-- " + block + ":start -->", "<!-- " + block + ":end -->"
}

// Table is the Markdown block for a summary.
func Table(s *score.Summary) string {
	if s.Malicious == 0 {
		return benignTable(s)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d skills (%d malicious, %d benign) from the %s source-disjoint `%s` partition, %s. "+
		"Scored on each tool's own verdict; intervals are 95%% clustered bootstrap.\n\n",
		s.Samples, s.Malicious, s.Benign, s.Dataset, s.Track, s.Claim)
	b.WriteString("| Tool | Version | Recall | False-positive rate | Precision | F1 |\n")
	b.WriteString("| ---- | ------- | ------ | ------------------- | --------- | -- |\n")
	for i := range s.Tools {
		t := &s.Tools[i]
		fmt.Fprintf(&b, "| %s | %s | %.3f (%.3f-%.3f) | %.3f (%.3f-%.3f) | %.3f | %.3f (%.3f-%.3f) |\n",
			t.Tool, t.Version, t.Rates.Recall, t.RecallCI.Low, t.RecallCI.High,
			t.Rates.FPR, t.FPRCI.Low, t.FPRCI.High, t.Rates.Precision, t.Rates.F1, t.F1CI.Low, t.F1CI.High)
	}
	return b.String()
}

// benignTable is the table for a population with nothing malicious: recall means nothing there, and
// every flag is a false alarm.
func benignTable(s *score.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d real skills from the `%s` track, all treated as benign, %s. A flag is a false alarm.\n\n",
		s.Samples, s.Track, s.Claim)
	b.WriteString("| Tool | Version | Skills flagged | Share flagged |\n")
	b.WriteString("| ---- | ------- | -------------- | ------------- |\n")
	for i := range s.Tools {
		t := &s.Tools[i]
		fmt.Fprintf(&b, "| %s | %s | %d of %d | %.1f%% (%.1f-%.1f) |\n", t.Tool, t.Version,
			t.Counts.FP, s.Benign, 100*t.Rates.FPR, 100*t.FPRCI.Low, 100*t.FPRCI.High)
	}
	return b.String()
}

// Load reads a summary.
func Load(path string) (*score.Summary, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the summary
	if err != nil {
		return nil, fmt.Errorf("read summary: %w", err)
	}
	var s score.Summary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse summary: %w", err)
	}
	return &s, nil
}

// File replaces the block between the markers. With check set it writes nothing and reports whether
// the file is stale.
func File(path, name, block string, check bool) error {
	startMarker, endMarker := markers(name)
	data, err := os.ReadFile(path) //nolint:gosec // a repository file named by the operator
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := string(data)
	i, j := strings.Index(text, startMarker), strings.Index(text, endMarker)
	if i < 0 || j < i {
		return fmt.Errorf("%s: no %s block", path, startMarker)
	}
	next := text[:i+len(startMarker)] + "\n\n" + strings.TrimSpace(block) + "\n\n" + text[j:]
	if next == text {
		return nil
	}
	if check {
		return fmt.Errorf("%s is stale: run skillbench render", path)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil { //nolint:gosec // a tracked doc
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
