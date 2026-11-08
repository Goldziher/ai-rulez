package scanner

import (
	"context"
)

// scanCisco uses the invocation of bench/judge/competitors.go: balanced policy, lenient parsing. The
// verdict is the tool's own is_safe.
func scanCisco(ctx context.Context, t *Tool, dir, work string) (Verdict, error) {
	out, err := reportPath(work, "cisco.json")
	if err != nil {
		return Verdict{}, err
	}
	cmd := command(ctx, t, work, work, "scan", dir, "--format", "json", "--output", out,
		"--policy", "balanced", "--lenient")
	if _, err := exitCode(cmd.Run()); err != nil {
		return Verdict{}, classedError("exit")
	}
	var report struct {
		IsSafe   *bool `json:"is_safe"`
		Findings []struct {
			RuleID   string `json:"rule_id"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	if err := readJSON(out, &report); err != nil {
		return Verdict{}, err
	}
	if report.IsSafe == nil {
		return Verdict{}, classedError("unparseable")
	}
	v := Verdict{Flagged: !*report.IsSafe, Rules: []string{}}
	for _, f := range report.Findings {
		v.Findings++
		v.Rules = append(v.Rules, f.RuleID)
		v.MaxSeverity = maxSeverity(v.MaxSeverity, normalize(f.Severity))
	}
	return v, nil
}
