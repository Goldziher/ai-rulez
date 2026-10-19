package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint/scanners"
)

const propsSARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"results":[{
 "ruleId":"PE3","level":"error","message":{"text":"Credential Access"},
 "properties":{"evidence":"cat ~/.aws/credentials","snippet":"line one\nline\ttwo\u202e","score":7.5,"tags":["a","b"],"nested":{"text":"deep"},"obj":{"k":"v"}},
 "locations":[{"physicalLocation":{"artifactLocation":{"uri":"SKILL.md"},"region":{"startLine":3,"snippet":{"text":"region text"}}}}]}]}]}`

func TestSARIFMessagePropertiesExtra(t *testing.T) {
	tests := []struct {
		name  string
		props []string
		want  string
	}{
		{"none configured", nil, ""},
		{"one string property", []string{"evidence"}, "evidence: cat ~/.aws/credentials"},
		{"order follows the list and whitespace collapses", []string{"snippet", "evidence"},
			"snippet: line one line two; evidence: cat ~/.aws/credentials"},
		{"numbers and string arrays", []string{"score", "tags"}, "score: 7.5; tags: a, b"},
		{"dotted path", []string{"nested.text"}, "text: deep"},
		{"standard region snippet", []string{"location.snippet"}, "snippet: region text"},
		{"missing and object-valued properties are skipped", []string{"nope", "obj", "evidence"},
			"evidence: cat ~/.aws/credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			found, err := parseExternalProps("sarif", []byte(propsSARIF), 0, false, tt.props)

			// Assert
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, tt.want, found[0].Extra)
			assert.Equal(t, "Credential Access", found[0].Message, "the message itself is unchanged")
		})
	}
}

func TestScannerMessageAppendsExtraWithinLimit(t *testing.T) {
	tests := []struct {
		name  string
		f     externalFinding
		limit int
		want  string
	}{
		{"no extra", externalFinding{Rule: "PE3", Message: "Credential Access"}, 500, "PE3: Credential Access"},
		{"extra is appended", externalFinding{Rule: "PE3", Message: "Credential Access", Extra: "evidence: x"}, 500,
			"PE3: Credential Access [evidence: x]"},
		{"extra counts against max_message", externalFinding{Message: "Credential Access", Extra: "evidence: " + strings.Repeat("y", 100)}, 30,
			"Credential Access [evidence: y..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			msg, _ := scannerMessage(tt.f, tt.limit, "", "")

			// Assert
			assert.Equal(t, tt.want, msg)
		})
	}
}

func TestMessagePropertiesValidation(t *testing.T) {
	tooMany := make([]string, maxMessageProperties+1)
	for i := range tooMany {
		tooMany[i] = "p"
	}
	tests := []struct {
		name  string
		props []string
		want  bool
	}{
		{"unset", nil, false},
		{"valid paths", []string{"evidence", "nested.text", "security-severity", "location.snippet"}, false},
		{"empty entry", []string{""}, true},
		{"whitespace in a path", []string{"a b"}, true},
		{"empty path segment", []string{"a..b"}, true},
		{"too many", tooMany, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			problems := messagePropertiesProblems(tt.props)

			// Assert
			if !tt.want {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0], "message_properties")
		})
	}
}

func TestExternalProblemsRejectsBadMessageProperties(t *testing.T) {
	problems := externalProblems(config.LintExternal{MessageProperties: []string{"bad path"}}, 0)
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "message_properties")
}

func TestSkillspectorProfileAppendsEvidenceAndSnippet(t *testing.T) {
	// Arrange
	p, ok := scanners.Lookup("skillspector")
	require.True(t, ok)

	// Act
	rs := fromProfile(config.LintExternal{Name: "skillspector", Profile: "skillspector"})

	// Assert
	assert.Equal(t, []string{"evidence", "snippet"}, p.MessageProperties)
	assert.Equal(t, []string{"evidence", "snippet"}, rs.MessageProperties)
}
