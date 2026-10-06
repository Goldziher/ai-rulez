package lint

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const secretRule = "---\nname: r\n---\nkey AKIAIOSFODNN7EXAMPLE\n"

func TestMaxFindingsCeiling(t *testing.T) {
	tests := []struct {
		name     string
		ceiling  map[string]int
		rule     string
		wantOver int
	}{
		{"a ceiling of zero allows no finding", map[string]int{"AR001": 0}, secretRule, 1},
		{"a ceiling above the count is fine", map[string]int{"AR001": 1}, secretRule, 0},
		{"no finding is within any ceiling", map[string]int{"AR001": 0}, "---\nname: r\n---\nclean\n", 0},
		{"two findings are over a ceiling of one", map[string]int{"AR001": 1}, "---\nname: r\n---\nkey AKIAIOSFODNN7EXAMPLE\nkey2 AKIAIOSFODNN7EXAMPLF\n", 1},
		{"codes without a ceiling are not counted", map[string]int{"AR005": 0}, secretRule, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := policyFixture("")
			files[".ai-rulez/rules/r.md"] = tt.rule
			writeFiles(t, root, files)
			gitAdd(t, root)
			// Act
			findings := lintWithOutcome(t, root, &config.PolicyOutcome{MaxFindings: tt.ceiling})
			// Assert
			if got := countCode(findings, CodePolicyBudgetExceeded); got != tt.wantOver {
				t.Fatalf("AR749 findings = %d, want %d: %v", got, tt.wantOver, findings)
			}
			if tt.wantOver > 0 && severityOf(findings, CodePolicyBudgetExceeded) != SeverityError {
				t.Fatalf("AR749 must be an error: %v", findings)
			}
		})
	}
}

func TestMaxFindingsCannotBeSilencedByTheRepository(t *testing.T) {
	// Arrange: the repository ignores the code and switches AR749 off.
	root := t.TempDir()
	writeFiles(t, root, policyFixture("[lint]\nignore = [\"AR749\"]\n[lint.severity]\nAR749 = \"off\"\n"))
	gitAdd(t, root)
	// Act
	findings := lintWithOutcome(t, root, &config.PolicyOutcome{MaxFindings: map[string]int{"AR001": 0}})
	// Assert
	if countCode(findings, CodePolicyBudgetExceeded) != 1 {
		t.Fatalf("AR749 must be reported once: %v", findings)
	}
}

func TestProtectedCodesIncludeCeilings(t *testing.T) {
	// Act
	got := ProtectedCodes(&config.PolicyOutcome{MaxFindings: map[string]int{"secret-detected": 0}})
	// Assert
	if !got["AR001"] || !got[CodePolicyBudgetExceeded] {
		t.Fatalf("a code with a ceiling is protected from baselines and tolerate: %v", got)
	}
}

func TestNoInlineIgnoreRefusesTheComment(t *testing.T) {
	tests := []struct {
		name         string
		noInline     []string
		wantSecret   int
		wantAttempts int
	}{
		{"listed code ignores the comment", []string{"AR001"}, 1, 1},
		{"another code keeps the comment", []string{"AR008"}, 0, 0},
		{"no policy list keeps the comment", nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := policyFixture("")
			files[".ai-rulez/rules/r.md"] = "---\nname: r\n---\n<!-- ai-rulez-lint-ignore: AR001 -->\nkey AKIAIOSFODNN7EXAMPLE\n"
			writeFiles(t, root, files)
			gitAdd(t, root)
			// Act
			findings := lintWithOutcome(t, root, &config.PolicyOutcome{NoInlineIgnore: tt.noInline})
			// Assert
			if got := countCode(findings, CodeSecretDetected); got != tt.wantSecret {
				t.Fatalf("AR001 findings = %d, want %d: %v", got, tt.wantSecret, findings)
			}
			attempts := 0
			for _, f := range findings {
				if f.Code == CodePolicyLoosened && strings.Contains(f.Message, "ai-rulez-lint-ignore") {
					attempts++
				}
			}
			if attempts != tt.wantAttempts {
				t.Fatalf("inline attempts = %d, want %d: %v", attempts, tt.wantAttempts, findings)
			}
		})
	}
}

func TestNoInlineIgnoreLeavesPathIgnoresAlone(t *testing.T) {
	// Arrange: only inline comments are refused; ignore_paths is another route.
	root := t.TempDir()
	writeFiles(t, root, policyFixture("[lint]\nignore_paths = [\".ai-rulez/rules/r.md\", \"rules/r.md\"]\n"))
	gitAdd(t, root)
	// Act
	findings := lintWithOutcome(t, root, &config.PolicyOutcome{NoInlineIgnore: []string{"AR001"}})
	// Assert
	if countCode(findings, CodeSecretDetected) != 0 {
		t.Fatalf("no_inline_ignore must not touch ignore_paths: %v", findings)
	}
}
