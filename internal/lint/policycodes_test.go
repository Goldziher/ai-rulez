package lint

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func policyFixture(lintTable string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml": "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + lintTable,
		".ai-rulez/rules/r.md":  "---\nname: r\n---\nkey AKIAIOSFODNN7EXAMPLE\n",
	}
}

func lintWithOutcome(t *testing.T, root string, out *config.PolicyOutcome) []Finding {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PolicyOutcome = out
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func severityOf(fs []Finding, code string) Severity {
	for _, f := range fs {
		if f.Code == code {
			return f.Severity
		}
	}
	return ""
}

func TestPolicyOutcomeIsReportedAsErrors(t *testing.T) {
	// Arrange: the repository tries to switch the policy codes off.
	root := t.TempDir()
	writeFiles(t, root, policyFixture("[lint]\nignore = [\"AR740\"]\n[lint.severity]\nAR745 = \"off\"\nAR744 = \"info\"\n"))
	gitAdd(t, root)
	out := &config.PolicyOutcome{Violations: []config.PolicyViolation{
		{Code: CodePolicyLoosened, Key: "lint.severity.AR008", Line: 3, Message: "below the floor"},
		{Code: CodeSourceNotAllowed, Key: "sources.allowed_hosts", Message: "not covered"},
		{Code: CodePolicyRequiredMissing, Key: "lint.ignore", Message: "required"},
	}}
	// Act
	findings := lintWithOutcome(t, root, out)
	// Assert
	for _, code := range []string{CodePolicyLoosened, CodeSourceNotAllowed, CodePolicyRequiredMissing} {
		if got := severityOf(findings, code); got != SeverityError {
			t.Errorf("%s severity = %q, want error: %v", code, got, findings)
		}
	}
}

func TestPolicyFloorAndRequiredCodesHoldAgainstConfig(t *testing.T) {
	tests := []struct {
		name  string
		table string
		out   *config.PolicyOutcome
		want  Severity
	}{
		{"no policy keeps the repository's choice", "[lint]\nignore = [\"AR001\"]\n", nil, ""},
		{"a required code ignored in config still reports", "[lint]\nignore = [\"AR001\"]\n", &config.PolicyOutcome{RequiredCodes: []string{"AR001"}}, SeverityError},
		{"a required code switched off returns at its default", "[lint.severity]\nAR001 = \"off\"\n", &config.PolicyOutcome{RequiredCodes: []string{"AR001"}}, SeverityError},
		{"a floor raises a demoted rule", "[lint.severity]\nAR001 = \"info\"\n", &config.PolicyOutcome{SeverityFloor: map[string]string{"AR001": "error"}}, SeverityError},
		{"a floor never lowers", "", &config.PolicyOutcome{SeverityFloor: map[string]string{"AR001": "info"}}, SeverityError},
		{"a floored code cannot be ignored", "[lint]\nignore = [\"AR001\"]\n", &config.PolicyOutcome{SeverityFloor: map[string]string{"AR001": "warning"}}, SeverityError},
		{"a permissive profile cannot demote below the floor", "[lint]\nprofile = \"permissive\"\n", &config.PolicyOutcome{SeverityFloor: map[string]string{"AR001": "error"}}, SeverityError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, policyFixture(tt.table))
			gitAdd(t, root)
			// Act
			findings := lintWithOutcome(t, root, tt.out)
			// Assert
			if got := severityOf(findings, CodeSecretDetected); got != tt.want {
				t.Fatalf("AR001 severity = %q, want %q: %v", got, tt.want, findings)
			}
		})
	}
}

func TestPolicyCodesAreRegisteredAndClassified(t *testing.T) {
	for _, code := range policyCodes {
		if _, ok := lookupRule(code); !ok {
			t.Errorf("%s is not registered", code)
		}
		if got := AnalyzerFor(code).Name; got != AnalyzerConfig {
			t.Errorf("%s analyzer = %s", code, got)
		}
		if !IsPolicyCode(code) {
			t.Errorf("IsPolicyCode(%s) = false", code)
		}
	}
	if IsPolicyCode("AR001") {
		t.Error("AR001 is not a policy code")
	}
	if code, ok := ResolveCode("secret-detected"); !ok || code != CodeSecretDetected {
		t.Errorf("ResolveCode(name) = %q, %v", code, ok)
	}
}
