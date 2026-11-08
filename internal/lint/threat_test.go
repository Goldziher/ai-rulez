package lint

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsThreat(t *testing.T) {
	cases := []struct {
		name string
		f    Finding
		want bool
	}{
		{"attack shape", Finding{Code: CodeShellExec, Severity: SeverityError}, true},
		{"instruction at warning", Finding{Code: CodeAntiRefusal, Severity: SeverityWarning}, true},
		{"demoted to info by config", Finding{Code: CodeShellExec, Severity: SeverityInfo}, false},
		{"capability is not a threat", Finding{Code: CodeToolBreadth, Severity: SeverityWarning}, false},
		{"hygiene at error is not a threat", Finding{Code: CodeToolBreadth, Severity: SeverityError}, false},
		{"secret in a body", Finding{Code: CodeExfilCommand, Severity: SeverityError}, true},
		{"secret in an auth header", Finding{Code: CodeExfilCommand, Severity: SeverityWarning}, false},
		{"another scanner's error", Finding{Code: CodeExternalFinding, Severity: SeverityError}, true},
		{"another scanner's warning", Finding{Code: CodeExternalFinding, Severity: SeverityWarning}, false},
	}
	for _, tt := range cases {
		if got := tt.f.IsThreat(); got != tt.want {
			t.Errorf("%s: IsThreat = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestThreatCodesAreRegistered counts the threat codes found among the registered rules, so a code
// named in threatCode but never registered, or one dropped from the switch, changes the count.
func TestThreatCodesAreRegistered(t *testing.T) {
	n := 0
	for _, r := range Rules() {
		if threatCode(r.Code) {
			n++
		}
	}
	if n != 26 {
		t.Errorf("%d registered rules are threat codes, want 26: update this count with the switch in threat.go", n)
	}
}

func TestFailedAtTheThreatGate(t *testing.T) {
	hygiene := Finding{Code: CodeToolBreadth, Severity: SeverityWarning}
	threat := Finding{Code: CodeShellExec, Severity: SeverityError}
	if Failed([]Finding{hygiene}, FailOnThreat) {
		t.Fatal("hygiene alone must not fail the threat gate")
	}
	if !Failed([]Finding{hygiene, threat}, FailOnThreat) {
		t.Fatal("a threat must fail the gate")
	}
	if !Failed([]Finding{hygiene}, "warning") {
		t.Fatal("--fail-on warning must keep its meaning")
	}
}

func TestJUnitFollowsTheThreatGate(t *testing.T) {
	c := Combined{Roots: []string{"."}, Findings: []Finding{
		{Code: CodeToolBreadth, Severity: SeverityWarning, Root: ".", Message: "m"},
		{Code: CodeShellExec, Severity: SeverityError, Root: ".", Message: "m"},
	}}
	var out bytes.Buffer
	if err := WriteJUnit(&out, c, FailOnThreat); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `failures="1"`) {
		t.Fatalf("got %s", out.String())
	}
}
