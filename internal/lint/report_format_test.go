package lint

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleCombined() Combined {
	return Combine([]*Report{{Root: ".", Findings: []Finding{
		{
			Code: CodeSecretDetected, Name: "secret-detected", Severity: SeverityError,
			File: ".ai-rulez/rules/a b.md", Meta: &FindingMeta{Path: ".ai-rulez/rules/a b.md", Fingerprint: "ar1:aaaaaaaaaaaaaaaaaaaaaaaa"}, Line: 7, Root: ".",
			Message: "AWS access key id AKIA**** found",
		},
		{
			Code: CodePathMissing, Name: "path-missing", Severity: SeverityWarning,
			File: ".ai-rulez/skills/s/SKILL.md", Meta: &FindingMeta{Path: ".ai-rulez/skills/s/SKILL.md", Fingerprint: "ar1:bbbbbbbbbbbbbbbbbbbbbbbb"}, Line: 12, Root: ".",
			Message: "path \"src/x.go\" does not exist | really,\nreally",
		},
		{
			Code: CodeAnchorUnresolved, Name: "anchor-unresolved", Severity: SeverityInfo,
			File: "README.md", Meta: &FindingMeta{Path: "README.md", Fingerprint: "ar1:cccccccccccccccccccccccc"}, Line: 3, Root: ".",
			Message: "no heading produces #x",
		},
	}}})
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, got, 0o644)) //nolint:gosec // test data
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; UPDATE_GOLDEN=1 go test ./internal/lint")
	assert.Equal(t, string(want), string(got))
}

func render(t *testing.T, format string, c Combined, o WriteOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, format, c, o))
	return buf.Bytes()
}

func TestFormatsGolden(t *testing.T) {
	c := sampleCombined()
	o := WriteOptions{Version: "1.2.3", FailOn: "error"}
	for format, file := range map[string]string{
		FormatSARIF: "sample.sarif", FormatGitHub: "sample.github.txt", FormatJUnit: "sample.junit.xml", FormatMarkdown: "sample.md",
	} {
		t.Run(format, func(t *testing.T) { golden(t, file, render(t, format, c, o)) })
	}
}

func TestFormatsEmptyReport(t *testing.T) {
	c := Combine([]*Report{{Root: "."}})
	assert.Empty(t, render(t, FormatGitHub, c, WriteOptions{}))
	assert.Contains(t, string(render(t, FormatMarkdown, c, WriteOptions{})), "No findings")
	validateSARIF(t, render(t, FormatSARIF, c, WriteOptions{}))
	var suites junitSuites
	require.NoError(t, xml.Unmarshal(render(t, FormatJUnit, c, WriteOptions{}), &suites))
	assert.Equal(t, 0, suites.Tests)
}

func validateSARIF(t *testing.T, doc []byte) {
	t.Helper()
	schemaBytes, err := os.ReadFile(filepath.Join("testdata", "sarif-schema-2.1.0.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	var v any
	require.NoError(t, json.Unmarshal(doc, &v))
	res := compiled.Validate(v)
	if !res.IsValid() {
		for field, e := range res.Errors {
			t.Errorf("SARIF schema violation at %s: %v", field, e)
		}
		t.Fatalf("SARIF output does not match the SARIF 2.1.0 schema:\n%s", doc)
	}
}

func TestSARIFMatchesOfficialSchema(t *testing.T) {
	validateSARIF(t, render(t, FormatSARIF, sampleCombined(), WriteOptions{Version: "1.2.3"}))
}

func TestSARIFSchemaRejectsBrokenDocument(t *testing.T) {
	// The validator must be able to fail, or the test above proves nothing.
	schemaBytes, err := os.ReadFile(filepath.Join("testdata", "sarif-schema-2.1.0.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(schemaBytes)
	require.NoError(t, err)
	var v any
	require.NoError(t, json.Unmarshal([]byte(`{"version":"2.0.0","runs":[{"tool":{}}]}`), &v))
	assert.False(t, compiled.Validate(v).IsValid())
}

func TestSARIFContent(t *testing.T) {
	var doc struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID         string         `json:"id"`
						HelpURI    string         `json:"helpUri"`
						Help       sarifText      `json:"help"`
						Properties map[string]any `json:"properties"`
						Default    struct {
							Level string `json:"level"`
						} `json:"defaultConfiguration"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID              string            `json:"ruleId"`
				RuleIndex           int               `json:"ruleIndex"`
				Level               string            `json:"level"`
				PartialFingerprints map[string]string `json:"partialFingerprints"`
				Properties          map[string]any    `json:"properties"`
				Locations           []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI       string `json:"uri"`
							URIBaseID string `json:"uriBaseId"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(render(t, FormatSARIF, sampleCombined(), WriteOptions{}), &doc))
	run := doc.Runs[0]
	rules := run.Tool.Driver.Rules
	require.Len(t, rules, 3)
	assert.Equal(t, "AR001", rules[0].ID)
	assert.Equal(t, "https://goldziher.github.io/ai-rulez/strict-validation/#ar001-secret-detected", rules[0].HelpURI)
	assert.Equal(t, "8.0", rules[0].Properties["security-severity"], "AR0xx carries security-severity")
	assert.NotContains(t, rules[1].Properties, "security-severity", "quality rules do not")
	assert.NotEmpty(t, rules[0].Help.Text)
	assert.Equal(t, "error", rules[0].Default.Level)

	levels := map[string]string{}
	for _, r := range run.Results {
		levels[r.RuleID] = r.Level
		assert.Equal(t, rules[r.RuleIndex].ID, r.RuleID)
		assert.Equal(t, "%SRCROOT%", r.Locations[0].PhysicalLocation.ArtifactLocation.URIBaseID)
		assert.NotEmpty(t, r.PartialFingerprints[sarifFingerprint])
	}
	assert.Equal(t, map[string]string{"AR001": "error", "AR401": "warning", "AR202": "note"}, levels)
	assert.Equal(t, ".ai-rulez/rules/a%20b.md", run.Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, 7, run.Results[0].Locations[0].PhysicalLocation.Region.StartLine)
}

func TestGitHubAnnotationEscaping(t *testing.T) {
	out := string(render(t, FormatGitHub, sampleCombined(), WriteOptions{}))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3, "a message newline must not split the command")
	assert.True(t, strings.HasPrefix(lines[0], "::error file=.ai-rulez/rules/a b.md,line=7,title=AR001 secret-detected::"))
	assert.True(t, strings.HasPrefix(lines[1], "::warning "))
	assert.Contains(t, lines[1], "really,%0Areally")
	assert.True(t, strings.HasPrefix(lines[2], "::notice "))
	assert.Equal(t, "a%3Ab%2Cc", ghEscape("a:b,c", true))
	assert.Equal(t, "100%25%0D%0A", ghEscape("100%\r\n", false))
}

func TestJUnitThresholdDecidesFailure(t *testing.T) {
	parse := func(failOn string) junitSuites {
		var s junitSuites
		require.NoError(t, xml.Unmarshal(render(t, FormatJUnit, sampleCombined(), WriteOptions{FailOn: failOn}), &s))
		return s
	}
	s := parse("error")
	assert.Equal(t, 3, s.Tests)
	assert.Equal(t, 1, s.Failures)
	assert.Equal(t, 2, s.Skipped)
	assert.Equal(t, 2, parse("warning").Failures)
}

func TestMarkdownGroupsBySeverity(t *testing.T) {
	out := string(render(t, FormatMarkdown, sampleCombined(), WriteOptions{}))
	e, w, i := strings.Index(out, "### Errors (1)"), strings.Index(out, "### Warnings (1)"), strings.Index(out, "### Infos (1)")
	assert.True(t, e >= 0 && e < w && w < i, out)
	assert.Contains(t, out, `does not exist \| really`)
}

func TestWriteRejectsUnknownFormat(t *testing.T) {
	assert.Error(t, Write(&bytes.Buffer{}, "xml", Combined{}, WriteOptions{}))
	assert.True(t, IsFormat("") && IsFormat(FormatSARIF) && !IsFormat("xml"))
}

func TestFingerprintIgnoresLineNumbersAndIndentation(t *testing.T) {
	a := fingerprintOf(CodePathMissing, "a.md", "  see `src/x.go`  ", 0)
	assert.Equal(t, a, fingerprintOf(CodePathMissing, "a.md", "see   `src/x.go`", 0))
	assert.NotEqual(t, a, fingerprintOf(CodePathMissing, "b.md", "see `src/x.go`", 0))
	assert.NotEqual(t, a, fingerprintOf(CodeLinkUnresolved, "a.md", "see `src/x.go`", 0))
	assert.NotEqual(t, a, fingerprintOf(CodePathMissing, "a.md", "see `src/x.go`", 1))
}
