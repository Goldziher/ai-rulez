package judge

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileSARIFSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "lint", "testdata", "sarif-schema-2.1.0.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	return compiled
}

func assertValidSARIF(t *testing.T, doc []byte) {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(doc, &v))
	res := compileSARIFSchema(t).Validate(v)
	if !res.IsValid() {
		for field, e := range res.Errors {
			t.Errorf("SARIF schema violation at %s: %v", field, e)
		}
		t.Fatalf("not valid SARIF 2.1.0:\n%s", doc)
	}
}

func sarifReport(fix *string) *Report {
	f := &Finding{
		ID: "f1", Code: "JR-S04", Category: CategorySecurity, Severity: SeverityHigh, Confidence: "high", Verdict: "confirmed",
		Title: "reads credentials", Location: Location{Path: ".claude/skills/a/SKILL.md", StartLine: 7, EndLine: 7},
		Quote: "cat ~/.aws/credentials", Why: "sends them out", SuggestedFix: fix, Origin: "security", Fingerprint: "abc",
	}
	return &Report{
		SchemaVersion: 2, Mode: "judge",
		Units:   []UnitReport{{ID: "skill:a", Path: ".claude/skills/a", Findings: []*Finding{f}, Gaps: []string{"chunk 2 not reviewed"}}},
		Summary: Summary{Decision: "fail"},
	}
}

func TestSARIFOutputMatchesTheSchemaWithAndWithoutASuggestedFix(t *testing.T) {
	fix := "remove the line"
	empty := ""
	tests := []struct {
		name string
		fix  *string
		want bool
	}{
		{"suggested fix", &fix, true},
		{"empty suggestion", &empty, false},
		{"no suggestion", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer

			// Act
			require.NoError(t, sarifReport(tc.fix).WriteSARIF(&buf))

			// Assert
			assertValidSARIF(t, buf.Bytes())
			var doc struct {
				Runs []struct {
					Results []map[string]any `json:"results"`
				} `json:"runs"`
			}
			require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
			res := doc.Runs[0].Results[0]
			assert.NotContains(t, res, "fixes", "a fix needs artifactChanges, which prose cannot supply")
			props, _ := res["properties"].(map[string]any)
			_, has := props["suggestedFix"]
			assert.Equal(t, tc.want, has)
			if tc.want {
				assert.Contains(t, buf.String(), "Suggested fix: remove the line")
			}
		})
	}
}

func TestSARIFSchemaRejectsAFixWithoutArtifactChanges(t *testing.T) {
	// The validator must be able to fail, or the test above proves nothing.
	var v any
	bad := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x"}},"results":[{"message":{"text":"m"},"fixes":[{"description":{"text":"d"}}]}]}]}`
	require.NoError(t, json.Unmarshal([]byte(bad), &v))
	assert.False(t, compileSARIFSchema(t).Validate(v).IsValid())
}
