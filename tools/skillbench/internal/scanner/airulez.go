package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
)

// The two ai-rulez rows run the same binary on the same samples and differ only in the gate. The threat
// gate fails on findings that describe an attack; the warning gate also fails on capability and hygiene
// findings, such as allowed-tools naming Bash. validate would add non-security findings and --external
// would absorb another scanner, so neither is used.
const (
	gateWarning = "warning"
	gateThreat  = "threat"
)

func scanAIRulez(ctx context.Context, t *Tool, dir, work string) (Verdict, error) {
	return scanAIRulezAt(ctx, t, dir, work, gateWarning)
}

func scanAIRulezThreat(ctx context.Context, t *Tool, dir, work string) (Verdict, error) {
	return scanAIRulezAt(ctx, t, dir, work, gateThreat)
}

// supportsThreatGate reports whether this ai-rulez release accepts --fail-on threat. Releases before it
// have no threat row; they still have the warning row.
func supportsThreatGate(ctx context.Context, t *Tool) bool {
	out, err := exec.CommandContext(ctx, t.Bin, "scan", "--help").CombinedOutput() //nolint:gosec // pinned tool
	return err == nil && bytes.Contains(out, []byte("threat"))
}

func scanAIRulezAt(ctx context.Context, t *Tool, dir, work, gate string) (Verdict, error) {
	cmd := command(ctx, t, dir, work, "scan", "--format", "json", "--fail-on", gate, "--no-local")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	code, err := exitCode(cmd.Run())
	if err != nil || (code != 0 && code != 2) {
		return Verdict{}, classedError("exit")
	}
	var report struct {
		Findings []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	raw := stdout.Bytes()
	if i := bytes.IndexByte(raw, '{'); i >= 0 {
		raw = raw[i:]
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return Verdict{}, classedError("unparseable")
	}
	v := Verdict{Flagged: code == 2, Rules: []string{}}
	for _, f := range report.Findings {
		v.Findings++
		v.Rules = append(v.Rules, f.Code)
		v.MaxSeverity = maxSeverity(v.MaxSeverity, normalize(f.Severity))
	}
	return v, nil
}
