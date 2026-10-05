package doctor

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/okf"
	"github.com/Goldziher/ai-rulez/internal/okfbridge"
)

// CheckOKF is the name of the OKF bundle check in reports.
const CheckOKF = "okf"

// checkOKF lints the configured OKF bundle for conformance (AR9B0-AR9B4,
// AR9B6-AR9B8). Drift against the sources is already covered by the drift
// check, which renders the okf preset like any other.
func checkOKF(_ context.Context, s *state) []Finding {
	if s.cfg == nil || !okfbridge.Configured(s.cfg) {
		return nil
	}
	res, err := okfbridge.CheckProject(s.cfg, nil)
	if err != nil {
		return errorFinding(CheckOKF, SeverityError, err, "")
	}
	if res == nil {
		return nil
	}
	var out []Finding
	for i := range res.Findings {
		f := &res.Findings[i]
		out = append(out, Finding{
			Check: CheckOKF, Severity: doctorSeverity(f.Severity), Path: s.cfg.OKFDir() + "/" + f.Path,
			Message: f.Code + " " + f.Message,
		})
	}
	return out
}

func doctorSeverity(s okf.Severity) Severity {
	switch s {
	case okf.SeverityError:
		return SeverityError
	case okf.SeverityWarning:
		return SeverityWarning
	}
	return SeverityInfo
}
