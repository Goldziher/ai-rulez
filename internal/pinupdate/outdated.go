package pinupdate

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// Outdated reports which sources of kind and names have newer tags than their
// pins (`lock --outdated`). It reads the remotes' tags and writes nothing.
func Outdated(ctx context.Context, env Env, kind string, names []string) (*tagresolve.Report, error) {
	if err := RequireOnline("lock --outdated", env.Policy.Offline); err != nil {
		return nil, err
	}
	p, err := Prepare(ctx, env, kind, names)
	if err != nil {
		return nil, err
	}
	report := tagresolve.NewReport(p.Rows)
	report.MarkOutdated(LintSeverityOf(p.Cfg, tagresolve.CodeOutdated, "source-outdated"))
	return report, nil
}

// LintSeverityOf is the [lint.severity] value for a rule given by code or name,
// lower case; "" when the table does not name it.
func LintSeverityOf(cfg *config.Config, code, name string) string {
	if cfg == nil || cfg.Lint == nil {
		return ""
	}
	for key, val := range cfg.Lint.Severity {
		if strings.EqualFold(key, code) || strings.EqualFold(key, name) {
			return strings.ToLower(strings.TrimSpace(val))
		}
	}
	return ""
}
