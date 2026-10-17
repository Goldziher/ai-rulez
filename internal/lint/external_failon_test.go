package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScannerFailOnIsIndependentOfTheRunThreshold(t *testing.T) {
	policy := func(failOn string) string { return "\n[lint.scanner_policy]\nfail_on = \"" + failOn + "\"\n" }
	tests := []struct {
		name       string
		policy     string
		entry      string
		level      string
		runFailOn  string
		wantFailed bool
	}{
		{"default: a warning passes the error gate", "", "", "warning", "error", false},
		{"default: --strict fails on a scanner warning", "", "", "warning", "warning", true},
		{"policy error: --strict does not fail on a scanner warning", policy("error"), "", "warning", "warning", false},
		{"policy error: a scanner error still fails", policy("error"), "", "error", "warning", true},
		{"policy warning: fails although the run gate is error", policy("warning"), "", "warning", "error", true},
		{"entry error beats a policy warning", policy("warning"), "fail_on = \"error\"\n", "warning", "warning", false},
		{"entry warning beats a policy error", policy("error"), "fail_on = \"warning\"\n", "warning", "error", true},
		{"entry error without a policy", "", "fail_on = \"error\"\n", "warning", "warning", false},
		{"none still disables every gate", policy("warning"), "", "error", "none", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fakeProject(t, tt.policy, fakeScannerCommand(t, "--level", tt.level, "--msg", "scanner says no")+tt.entry)

			// Act
			findings := p.run(Options{})

			// Assert
			require.Len(t, scannerFindings(findings), 1, dump(findings))
			assert.Equal(t, tt.wantFailed, Failed(findings, tt.runFailOn), dump(findings))
		})
	}
}

func TestScannerFailOnLeavesOtherFindingsToTheRunThreshold(t *testing.T) {
	// Arrange: a scanner configuration warning (AR9E1) carries no scanner threshold.
	other := Finding{Code: CodeScannerEgressUndeclared, Severity: SeverityWarning}

	// Act / Assert
	assert.True(t, Failed([]Finding{other}, "warning"))
	assert.False(t, Failed([]Finding{other}, "error"))
}

func TestScannerFailOnRejectsAnUnknownValue(t *testing.T) {
	// Arrange
	p := fakeProject(t, "", fakeScannerCommand(t)+"fail_on = \"fatal\"\n")

	// Act
	findings := p.run(Options{})

	// Assert
	invalid := ofCode(findings, CodeScannerConfigInvalid)
	require.Len(t, invalid, 1, dump(findings))
	assert.Contains(t, invalid[0].Message, "fail_on")
	assert.Empty(t, scannerFindings(findings))
}
