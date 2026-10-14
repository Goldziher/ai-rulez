package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reportSchemaFindingProps returns the finding properties and required keys the
// validate report schema declares.
func reportSchemaFindingProps(t *testing.T) (props map[string]bool, required []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "validate-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			Findings struct {
				Items struct {
					Required   []string                   `json:"required"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	props = map[string]bool{}
	for k := range doc.Properties.Findings.Items.Properties {
		props[k] = true
	}
	return props, doc.Properties.Findings.Items.Required
}

func TestReportSchemaDeclaresTheKeysAFindingEmits(t *testing.T) {
	props, required := reportSchemaFindingProps(t)
	f := Finding{
		Code: CodeSecretDetected, Name: "secret-detected", Severity: SeverityError, File: "a.md", Line: 3, Message: "m",
		Meta: &FindingMeta{Source: SourceOutput},
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var emitted map[string]json.RawMessage
	if err := json.Unmarshal(raw, &emitted); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"code", "name", "severity", "file", "line", "message", "source"} {
		if _, ok := emitted[key]; !ok {
			t.Fatalf("finding JSON lacks %q: %s", key, raw)
		}
		if !props[key] {
			t.Errorf("schema/validate-report.schema.json does not declare the emitted finding key %q", key)
		}
	}
	if props["path"] {
		t.Error("the schema declares finding key \"path\", which is never emitted (the key is \"file\")")
	}
	for _, key := range required {
		if _, ok := emitted[key]; !ok {
			t.Errorf("schema requires %q but findings do not emit it", key)
		}
	}
}

func TestSARIFCarriesTheOutputSourceInResultProperties(t *testing.T) {
	f := Finding{
		Code: CodeSecretDetected, Name: "secret-detected", Severity: SeverityError, File: "a.md", Line: 3, Message: "m",
		Meta: &FindingMeta{Source: SourceOutput},
	}
	var sb strings.Builder
	if err := Write(&sb, FormatSARIF, Combine([]*Report{{Root: ".", Findings: []Finding{f}}}), WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Results []struct {
				Properties map[string]any `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(sb.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Results) != 1 {
		t.Fatalf("want one SARIF result: %s", sb.String())
	}
	if got := doc.Runs[0].Results[0].Properties["source"]; got != "output" {
		t.Errorf("results[0].properties.source = %v, want output\n%s", got, sb.String())
	}
}
