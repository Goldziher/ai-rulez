package doctor

import (
	"strings"
	"testing"
)

func TestCheckLLM(t *testing.T) {
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "")
	t.Setenv("DOCTOR_LLM_KEY", "")
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + `
[llm]
provider = "openai"
model = "gpt-4o-mini"
base_url = "https://gateway.internal/v1"
api_key_env = "DOCTOR_LLM_KEY"
allow_network = true
`})
	got := byCheck(run(t, dir), CheckLLM)
	var warn, info bool
	for _, f := range got {
		warn = warn || (f.Severity == SeverityWarning && strings.Contains(f.Message, "DOCTOR_LLM_KEY is not set"))
		info = info || (f.Severity == SeverityInfo && strings.Contains(f.Message, "gateway.internal") && strings.Contains(f.Message, "network allowed: true"))
		if strings.Contains(f.Message, "/v1") {
			t.Errorf("only the host may be shown: %q", f.Message)
		}
	}
	if !warn || !info {
		t.Fatalf("want an unset-key warning and a summary, got %+v", got)
	}

	bad := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + "\n[llm]\nbackend = \"nope\"\n"})
	if f := byCheck(run(t, bad), CheckLLM); len(f) == 0 || f[0].Severity != SeverityError || !strings.Contains(f[0].Message, "AR9C0") {
		t.Fatalf("invalid [llm] must be an AR9C0 error, got %+v", f)
	}

	none := project(t, map[string]string{".ai-rulez/config.toml": baseConfig})
	if f := byCheck(run(t, none), CheckLLM); len(f) != 0 {
		t.Fatalf("no [llm] table, no finding: %+v", f)
	}
}
