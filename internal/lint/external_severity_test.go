package lint

import (
	"strings"
	"testing"
)

func TestScannerBand(t *testing.T) {
	tests := []struct {
		name   string
		sevMap map[string]string
		f      externalFinding
		want   Severity
	}{
		{"severity_map wins over everything", map[string]string{"YARA-*": "low"}, externalFinding{Rule: "YARA-7", Severity: "error", Score: 9.9, HasScore: true}, SeverityInfo},
		{"exact id beats glob", map[string]string{"YARA-*": "low", "YARA-7": "critical"}, externalFinding{Rule: "YARA-7"}, SeverityError},
		{"longest glob beats shorter", map[string]string{"*": "info", "YARA-*": "high"}, externalFinding{Rule: "YARA-7"}, SeverityError},
		{"score 9.0 is critical", nil, externalFinding{Severity: "note", Score: 9, HasScore: true}, SeverityError},
		{"score 7.0 is high", nil, externalFinding{Score: 7, HasScore: true}, SeverityError},
		{"score 4.0 is medium", nil, externalFinding{Score: 4, HasScore: true}, SeverityWarning},
		{"score 0.1 is low", nil, externalFinding{Score: 0.1, HasScore: true}, SeverityInfo},
		{"score 0 falls to the level", nil, externalFinding{Score: 0, HasScore: true, Severity: "error"}, SeverityError},
		{"level error", nil, externalFinding{Severity: "error"}, SeverityError},
		{"level warning", nil, externalFinding{Severity: "warning"}, SeverityWarning},
		{"level note", nil, externalFinding{Severity: "note"}, SeverityInfo},
		{"level none", nil, externalFinding{Severity: "none"}, SeverityInfo},
		{"json severity high", nil, externalFinding{Severity: "HIGH"}, SeverityError},
		{"unknown word is medium", nil, externalFinding{Severity: "whatever"}, SeverityWarning},
		{"rule default level", nil, externalFinding{DefaultLevel: "error"}, SeverityError},
		{"level wins over rule default", nil, externalFinding{Severity: "note", DefaultLevel: "error"}, SeverityInfo},
		{"nothing is medium", nil, externalFinding{}, SeverityWarning},
		{"bad severity_map value is ignored at run time", map[string]string{"X": "scary"}, externalFinding{Rule: "X", Severity: "error"}, SeverityError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := bandSeverity(scannerBand(tt.sevMap, tt.f))
			// Assert
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSeverityMapProblems(t *testing.T) {
	if got := severityMapProblems(map[string]string{"A*": "high", "B": "info"}, "warning"); len(got) != 0 {
		t.Errorf("valid settings: %v", got)
	}
	got := severityMapProblems(map[string]string{"[": "high", "B": "scary"}, "huge")
	if len(got) != 3 {
		t.Fatalf("want 3 problems, got %v", got)
	}
}

func TestParseExternalSARIFMetadata(t *testing.T) {
	doc := `{"version":"2.1.0","runs":[{"tool":{"driver":{"rules":[
		{"id":"R0","helpUri":"https://example.test/r0","defaultConfiguration":{"level":"error"},"properties":{"security-severity":"7.5"}},
		{"id":"R1"}]}},
	"results":[
		{"ruleIndex":0,"message":{"text":"by index"},"partialFingerprints":{"b":"2","primaryLocationLineHash":"abc:1"}},
		{"ruleId":"R1","message":{"text":"by id"},"fingerprints":{"z":"9","a":"1"},"properties":{"security-severity":3.5}},
		{"rule":{"id":"R1"},"message":{"text":"nested rule"},"partialFingerprints":{"b":"2","a":"1"}}]}]}`
	// Act
	got, err := parseExternal("sarif", []byte(doc), 0)
	// Assert
	if err != nil || len(got) != 3 {
		t.Fatalf("got %d findings, err %v", len(got), err)
	}
	if got[0].Rule != "R0" || got[0].HelpURI != "https://example.test/r0" || got[0].DefaultLevel != "error" ||
		!got[0].HasScore || got[0].Score != 7.5 || got[0].Fingerprint != "abc:1" {
		t.Errorf("by index: %+v", got[0])
	}
	if got[1].Rule != "R1" || got[1].Score != 3.5 || got[1].Fingerprint != "a=1" {
		t.Errorf("by id: %+v", got[1])
	}
	if got[2].Rule != "R1" || got[2].Fingerprint != "a=1" {
		t.Errorf("nested: %+v", got[2])
	}
}

func TestParseExternalKeepSuppressed(t *testing.T) {
	doc := `{"runs":[{"results":[{"ruleId":"A","message":{"text":"m"},"suppressions":[{"kind":"external"}]}]}]}`
	dropped, _ := parseExternalKeep("sarif", []byte(doc), 0, false)
	kept, err := parseExternalKeep("sarif", []byte(doc), 0, true)
	if err != nil || len(dropped) != 0 || len(kept) != 1 || !kept[0].Suppressed {
		t.Fatalf("dropped %d, kept %+v, err %v", len(dropped), kept, err)
	}
}

func TestEvidenceURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.test/r":  "https://example.test/r",
		"http://example.test/r":   "",
		"javascript:alert(1)":     "",
		"https://x.test/a b":      "",
		"https://x.test/\x1b[31m": "",
		"":                        "",
		"https://x.test/" + strings.Repeat("a", 300): "",
	} {
		if got := evidenceURL(in); got != want {
			t.Errorf("evidenceURL(%q) = %q, want %q", in, got, want)
		}
	}
}
