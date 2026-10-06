package lint

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPolicyWarnModeDowngradesThePolicyFindings(t *testing.T) {
	tests := []struct {
		name string
		warn bool
		want Severity
	}{
		{"enforce reports errors", false, SeverityError},
		{"warn reports warnings", true, SeverityWarning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := policyFixture("[lint]\nignore_paths = [\".ai-rulez/rules/r.md\", \"rules/r.md\"]\n")
			writeFiles(t, root, files)
			gitAdd(t, root)
			out := &config.PolicyOutcome{
				Warn:          tt.warn,
				RequiredCodes: []string{"AR001"},
				MaxFindings:   map[string]int{"AR001": 0},
				Violations:    []config.PolicyViolation{{Code: CodePolicyLoosened, Key: "lint.severity.AR008", Line: 3, Message: "below the floor"}},
			}
			// Act
			findings := lintWithOutcome(t, root, out)
			// Assert
			for _, code := range []string{CodePolicyLoosened, CodePolicyBudgetExceeded} {
				for _, f := range findings {
					if f.Code == code && f.Severity != tt.want {
						t.Errorf("%s severity = %q, want %q: %v", code, f.Severity, tt.want, f)
					}
				}
			}
			attempt := false
			for _, f := range findings {
				if f.Code == CodePolicyLoosened && strings.Contains(f.Message, "ignore_paths") {
					attempt = true
					if f.Severity != tt.want {
						t.Errorf("the suppression attempt has severity %q, want %q", f.Severity, tt.want)
					}
				}
			}
			if !attempt {
				t.Fatalf("the ignore_paths attempt must still be reported: %v", findings)
			}
			if got := countCode(findings, CodeSecretDetected); got != 1 {
				t.Fatalf("a protected code stays reported in warn mode: %v", findings)
			}
		})
	}
}

func TestPolicyWarnModeBaselineRefusalIsAWarning(t *testing.T) {
	// Arrange
	rep := &Report{ConfigFile: ".ai-rulez/config.toml", Root: "r", PolicyWarn: true}
	// Act
	rep.RefuseTolerate([]string{"AR001"})
	// Assert
	if len(rep.Findings) != 1 || rep.Findings[0].Severity != SeverityWarning {
		t.Fatalf("want one warning, got %v", rep.Findings)
	}
}
