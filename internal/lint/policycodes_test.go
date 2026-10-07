package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func policyFixture(lintTable string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + lintTable,
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

func TestPolicyCodesCannotBeSuppressedByPathOrInlineIgnores(t *testing.T) {
	protected := []*config.PolicyOutcome{
		{RequiredCodes: []string{"AR001"}},
		{SeverityFloor: map[string]string{"AR001": "error"}},
	}
	tests := []struct {
		name      string
		table     string
		rule      string
		attempted string // route named by the AR740 finding; "" when no suppression is attempted
	}{
		{"ignore_paths", "[lint]\nignore_paths = [\".ai-rulez/rules/r.md\", \"rules/r.md\"]\n",
			"---\nname: r\n---\nkey AKIAIOSFODNN7EXAMPLE\n", "ignore_paths"},
		{"inline code", "", "---\nname: r\n---\n<!-- ai-rulez-lint-ignore: AR001 -->\nkey AKIAIOSFODNN7EXAMPLE\n", "ai-rulez-lint-ignore"},
		{"inline blanket", "", "---\nname: r\n---\n<!-- ai-rulez-lint-ignore -->\nkey AKIAIOSFODNN7EXAMPLE\n", "ai-rulez-lint-ignore"},
		{"nothing attempted", "", "---\nname: r\n---\nkey AKIAIOSFODNN7EXAMPLE\n", ""},
	}
	for _, out := range protected {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Arrange
				root := t.TempDir()
				files := policyFixture(tt.table)
				files[".ai-rulez/rules/r.md"] = tt.rule
				writeFiles(t, root, files)
				gitAdd(t, root)
				// Act
				findings := lintWithOutcome(t, root, out)
				// Assert
				if countCode(findings, CodeSecretDetected) != 1 {
					t.Fatalf("AR001 must still be reported once: %v", findings)
				}
				wantAttempts := 0
				if tt.attempted != "" {
					wantAttempts = 1
				}
				attempts := 0
				for _, f := range findings {
					if f.Code == CodePolicyLoosened && strings.Contains(f.Message, tt.attempted) && tt.attempted != "" {
						attempts++
					}
				}
				if attempts != wantAttempts {
					t.Fatalf("suppression attempts via %q = %d, want %d: %v", tt.attempted, attempts, wantAttempts, findings)
				}
			})
		}
	}
}

func TestUnprotectedCodesStillHonorIgnores(t *testing.T) {
	// Arrange: the policy protects another code, so AR001 keeps its ignore.
	root := t.TempDir()
	writeFiles(t, root, policyFixture("[lint]\nignore_paths = [\".ai-rulez/rules/r.md\", \"rules/r.md\"]\n"))
	gitAdd(t, root)
	// Act
	findings := lintWithOutcome(t, root, &config.PolicyOutcome{RequiredCodes: []string{"AR008"}})
	// Assert
	if countCode(findings, CodeSecretDetected) != 0 || countCode(findings, CodePolicyLoosened) != 0 {
		t.Fatalf("an unprotected code keeps its ignore and nothing is reported: %v", findings)
	}
}

func TestPolicyFindingsIgnoreIgnorePaths(t *testing.T) {
	// Arrange: ignore_paths covers the config file the policy findings point at.
	root := t.TempDir()
	writeFiles(t, root, policyFixture("[lint]\nignore_paths = [\".ai-rulez/config.toml\", \"config.toml\"]\n"))
	gitAdd(t, root)
	out := &config.PolicyOutcome{Violations: []config.PolicyViolation{
		{Code: CodePolicyLoosened, Key: "lint.severity.AR008", Line: 3, Message: "below the floor"},
	}}
	// Act
	findings := lintWithOutcome(t, root, out)
	// Assert
	if countCode(findings, CodePolicyLoosened) == 0 {
		t.Fatalf("AR740 must survive ignore_paths: %v", findings)
	}
}
