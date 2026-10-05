package doctor

import (
	"context"
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// CheckLLM names the [llm] section of the report.
const CheckLLM = "llm"

// checkLLM reports the resolved LLM setup. It is offline: it reads only whether
// the key variable is set, never its value, and never calls a model (use
// `ai-rulez llm doctor --ping` for a live health call). Projects without an
// [llm] table get no finding.
func checkLLM(_ context.Context, s *state) []Finding {
	if s.cfg == nil || s.cfg.LLM == nil {
		return nil
	}
	res, err := s.cfg.ResolveLLM(nil)
	if err != nil {
		return []Finding{{Check: CheckLLM, Severity: SeverityError, Message: err.Error()}}
	}
	d := llm.Diagnose(res.Config, llm.Options{ConfigDir: s.cfg.ConfigDir})
	var out []Finding
	if len(res.Ignored) > 0 {
		out = append(out, Finding{Check: CheckLLM, Severity: SeverityWarning, Message: llm.IgnoredKeysMessage(res.Ignored), Hint: "see docs/llm.md, \"Trust rule\""})
	}
	for _, p := range d.Problems {
		out = append(out, Finding{Check: CheckLLM, Severity: SeverityError, Message: llm.CodeConfigInvalid + " " + p, Hint: "see docs/llm.md"})
	}
	if d.AllowNetwork && d.APIKeyEnv != "" && !d.APIKeySet {
		out = append(out, Finding{Check: CheckLLM, Severity: SeverityWarning,
			Message: fmt.Sprintf("allow_network is true but %s is not set", d.APIKeyEnv), Hint: "export the variable named by llm.api_key_env"})
	}
	host := d.BaseURLHost
	if host == "" {
		host = "provider default"
	}
	out = append(out, Finding{Check: CheckLLM, Severity: SeverityInfo,
		Message: fmt.Sprintf("backend %s, model %s, endpoint %s, network allowed: %v", d.Backend, orDash(d.Model), host, d.AllowNetwork),
		Hint:    "`ai-rulez llm doctor --ping` makes one live call"})
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
