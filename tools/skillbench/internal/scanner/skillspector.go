package scanner

import (
	"context"
	"strings"
)

// scanSkillSpector uses the offline mode. The verdict is the tool's own recommendation.
func scanSkillSpector(ctx context.Context, t *Tool, dir, work string) (Verdict, error) {
	out, err := reportPath(work, "skillspector.json")
	if err != nil {
		return Verdict{}, err
	}
	cmd := command(ctx, t, work, work, "scan", dir, "--no-llm", "--format", "json", "--output", out)
	if _, err := exitCode(cmd.Run()); err != nil {
		return Verdict{}, classedError("exit")
	}
	var report struct {
		Risk struct {
			Recommendation string `json:"recommendation"`
		} `json:"risk_assessment"`
		Issues []struct {
			ID       string `json:"id"`
			Pattern  string `json:"pattern"`
			Severity string `json:"severity"`
		} `json:"issues"`
	}
	if err := readJSON(out, &report); err != nil {
		return Verdict{}, err
	}
	if report.Risk.Recommendation == "" {
		return Verdict{}, classedError("unparseable")
	}
	v := Verdict{Flagged: !strings.EqualFold(report.Risk.Recommendation, "SAFE"), Rules: []string{}}
	for _, f := range report.Issues {
		v.Findings++
		rule := f.Pattern
		if rule == "" {
			rule = f.ID
		}
		v.Rules = append(v.Rules, rule)
		v.MaxSeverity = maxSeverity(v.MaxSeverity, normalize(f.Severity))
	}
	return v, nil
}
