package lint

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func scanResult(rule, level, text string, line int) string {
	return `{"ruleId":"` + rule + `","level":"` + level + `","message":{"text":"` + text + `"},"locations":[{"physicalLocation":` +
		`{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":` + strconv.Itoa(line) + `}}}]}`
}

var threeLevels = `printf '{"version":"2.1.0","runs":[{"results":[` +
	scanResult("SEC-1", "error", `first line\\nsecond line`, 1) + "," +
	scanResult("STYLE-2", "warning", "warn me", 3) + "," +
	scanResult("NOTE-3", "note", "note me", 4) + `]}]}'` + "\n"

func TestWriteBaselineFilterAndReasons(t *testing.T) {
	tests := []struct {
		name string
		opts ScannerOptions
		want map[string]string // rule -> reason
	}{
		{
			name: "no filter records everything with the template",
			opts: ScannerOptions{Reason: "{severity} {rule}"},
			want: map[string]string{"SEC-1": "error SEC-1", "STYLE-2": "warning STYLE-2", "NOTE-3": "info NOTE-3"},
		},
		{
			name: "min severity",
			opts: ScannerOptions{Reason: "r", Filter: BaselineFilter{MinSeverity: SeverityWarning}},
			want: map[string]string{"SEC-1": "r", "STYLE-2": "r"},
		},
		{
			name: "rule globs",
			opts: ScannerOptions{Reason: "r", Filter: BaselineFilter{Rules: []string{"STYLE-*", "fake:NOTE-3"}}},
			want: map[string]string{"STYLE-2": "r", "NOTE-3": "r"},
		},
		{
			name: "per rule reason beats the default",
			opts: ScannerOptions{Reason: "default", ReasonFor: map[string]string{"STYLE-*": "style debt", "SEC-1": "tracked in {rule}"}},
			want: map[string]string{"SEC-1": "tracked in SEC-1", "STYLE-2": "style debt", "NOTE-3": "default"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newScannerProject(t, threeLevels, "egress = false\ninputs = [\"rules\"]\n", nil)
			baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
			// Act
			p.run(Options{Scanner: withWrite(tt.opts)})
			// Assert
			b, err := LoadBaseline(baseline)
			if err != nil || b == nil {
				t.Fatalf("load: %v", err)
			}
			got := map[string]string{}
			for _, e := range b.Entries {
				got[e.Rule] = e.Reason
				if strings.Contains(e.Message, "\n") {
					t.Errorf("message must be one line: %q", e.Message)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("entries %v, want %v", got, tt.want)
			}
			for rule, reason := range tt.want {
				if got[rule] != reason {
					t.Errorf("%s reason %q, want %q", rule, got[rule], reason)
				}
			}
		})
	}
}

func TestWriteBaselineFilterKeepsEntriesItSkips(t *testing.T) {
	// Arrange: everything is recorded with one reason first.
	p := newScannerProject(t, threeLevels, "egress = false\ninputs = [\"rules\"]\n", nil)
	baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
	p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "first", NoCache: true})})
	// Act: a filtered rewrite must not drop or re-reason the entries it skips.
	p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "second", Filter: BaselineFilter{MinSeverity: SeverityError}})})
	// Assert
	b, err := LoadBaseline(baseline)
	if err != nil || b == nil || len(b.Entries) != 3 {
		t.Fatalf("baseline = %+v, %v", b, err)
	}
	for _, e := range b.Entries {
		if e.Reason != "first" {
			t.Errorf("%s reason %q, want the existing one", e.Rule, e.Reason)
		}
	}
}

func TestParseBaselineFilterAndReasonFor(t *testing.T) {
	if _, err := ParseBaselineFilter("loud", nil); err == nil {
		t.Error("an unknown severity must fail")
	}
	if _, err := ParseBaselineFilter("off", nil); err == nil {
		t.Error("off is not a severity to record")
	}
	if _, err := ParseBaselineFilter("", []string{" "}); err == nil {
		t.Error("an empty rule must fail")
	}
	if _, err := ParseReasonFor([]string{"NOEQUALS"}); err == nil {
		t.Error("RULE=text is required")
	}
	m, err := ParseReasonFor([]string{"A-*=because = reasons"})
	if err != nil || m["A-*"] != "because = reasons" {
		t.Errorf("got %v, %v", m, err)
	}
}

func TestShortBaselineMessage(t *testing.T) {
	long := strings.Repeat("x", 400)
	if got := shortBaselineMessage("a\nb"); got != "a" {
		t.Errorf("got %q", got)
	}
	if got := shortBaselineMessage(long); len([]rune(got)) != baselineMessageLimit {
		t.Errorf("length %d", len([]rune(got)))
	}
}

func withWrite(o ScannerOptions) ScannerOptions {
	o.WriteBaseline = true
	return o
}

// A rewrite reflects the current findings: an entry that matches none is
// dropped, while the entries of a finding the filter skips are kept.
func TestWriteBaselineDropsStaleEntries(t *testing.T) {
	tests := []struct {
		name   string
		filter BaselineFilter
		before []string // rule ids the scanner reported on the first write
		after  []string // rule ids the scanner reports on the rewrite
		want   []string // rules left in the baseline
	}{
		{name: "stale entry is dropped", before: []string{"SEC-1", "STYLE-2"}, after: []string{"SEC-1", "NOTE-3"}, want: []string{"NOTE-3", "SEC-1"}},
		{name: "all findings gone empties the baseline", before: []string{"SEC-1"}, after: nil, want: nil},
		{name: "entries the filter skips are kept", filter: BaselineFilter{MinSeverity: SeverityError}, before: []string{"SEC-1", "STYLE-2"}, after: []string{"SEC-1", "STYLE-2"}, want: []string{"SEC-1", "STYLE-2"}},
	}
	levels := map[string]string{"SEC-1": "error", "STYLE-2": "warning", "NOTE-3": "note"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			result := filepath.Join(t.TempDir(), "result.json")
			emit := func(rules []string) {
				var parts []string
				for i, r := range rules {
					parts = append(parts, scanResult(r, levels[r], "msg "+r, i+1))
				}
				doc := `{"version":"2.1.0","runs":[{"results":[` + strings.Join(parts, ",") + `]}]}`
				if err := os.WriteFile(result, []byte(doc), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			emit(tt.before)
			p := newScannerProject(t, "cat '"+result+"'\n", "egress = false\ninputs = [\"rules\"]\n", nil)
			baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
			p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "first", NoCache: true})})
			// Act
			emit(tt.after)
			p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "second", NoCache: true, Filter: tt.filter})})
			// Assert
			b, err := LoadBaseline(baseline)
			if err != nil || b == nil {
				t.Fatalf("load: %v", err)
			}
			var got []string
			for _, e := range b.Entries {
				got = append(got, e.Rule)
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("entries %v, want %v", got, tt.want)
			}
		})
	}
}

// A rewrite must not grow a message that an earlier write shortened, even when
// the scanner words the same finding differently.
func TestWriteBaselineKeepsShortenedMessagesOnRewrite(t *testing.T) {
	// Arrange
	result := filepath.Join(t.TempDir(), "result.json")
	emit := func(text string) {
		doc := `{"version":"2.1.0","runs":[{"results":[` + scanResult("SEC-1", "error", text, 1) + `]}]}`
		if err := os.WriteFile(result, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	emit(strings.Repeat("word ", 80) + `\\nsecond line`)
	p := newScannerProject(t, "cat '"+result+"'\n", "egress = false\ninputs = [\"rules\"]\n", nil)
	baseline := filepath.Join(p.root, ".ai-rulez", ScannerBaselineFile)
	p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "first", NoCache: true})})

	// Act
	emit(strings.Repeat("other ", 80) + `\\nsecond line`)
	p.run(Options{Scanner: withWrite(ScannerOptions{Reason: "second", NoCache: true})})

	// Assert
	b, err := LoadBaseline(baseline)
	if err != nil || b == nil || len(b.Entries) != 1 {
		t.Fatalf("baseline = %+v, %v", b, err)
	}
	if n := len([]rune(b.Entries[0].Message)); n > baselineMessageLimit || strings.Contains(b.Entries[0].Message, "\n") {
		t.Errorf("rewritten message is %d runes: %q", n, b.Entries[0].Message)
	}
}
