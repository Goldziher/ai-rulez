package doctor

import (
	"strings"
	"testing"
)

func TestCheckLLM(t *testing.T) {
	isolateLLM(t)
	t.Setenv("DOCTOR_LLM_KEY", "")
	// network, endpoint and key variable are user scope: set them through the environment
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "true")
	t.Setenv("AI_RULEZ_LLM_BASE_URL", "https://gateway.internal/v1")
	t.Setenv("AI_RULEZ_LLM_API_KEY_ENV", "DOCTOR_LLM_KEY")
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + `
[llm]
provider = "openai"
model = "gpt-4o-mini"
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

	bad := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + "\n[llm]\napi_key_env = \"my key\"\n"})
	if f := byCheck(run(t, bad), CheckLLM); len(f) == 0 || f[0].Severity != SeverityError || !strings.Contains(f[0].Message, "AR9L0") {
		t.Fatalf("invalid [llm] must be an AR9L0 error, got %+v", f)
	}

	none := project(t, map[string]string{".ai-rulez/config.toml": baseConfig})
	if f := byCheck(run(t, none), CheckLLM); len(f) != 0 {
		t.Fatalf("no [llm] table, no finding: %+v", f)
	}
}

// isolateLLM points user scope at an empty directory and clears AI_RULEZ_LLM_*.
func isolateLLM(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, k := range []string{"PROVIDER", "MODEL", "BASE_URL", "API_KEY_ENV", "EMBEDDING_MODEL", "MAX_COST_USD", "MAX_TOKENS", "MAX_CALLS", "TIMEOUT_SECONDS", "CACHE", "ALLOW_NETWORK"} {
		t.Setenv("AI_RULEZ_LLM_"+k, "")
	}
}

func TestCheckLLMRepoCannotEnableNetwork(t *testing.T) {
	isolateLLM(t)
	t.Setenv("HOME_SECRET", "topsecret123")
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + `
[llm]
model = "x"
api_key_env = "HOME_SECRET"
allow_network = true
base_url = "http://127.0.0.1:18765/v1"
`})
	var warned, offline bool
	for _, f := range byCheck(run(t, dir), CheckLLM) {
		warned = warned || (f.Severity == SeverityWarning && strings.Contains(f.Message, "ignored") && strings.Contains(f.Message, "allow_network"))
		offline = offline || strings.Contains(f.Message, "network allowed: false")
		if strings.Contains(f.Message, "topsecret") {
			t.Errorf("secret value leaked: %q", f.Message)
		}
	}
	if !warned || !offline {
		t.Fatalf("repo-set network must be ignored with a warning (warned=%v offline=%v)", warned, offline)
	}
}

func TestCheckLLMReportsThePlainHTTPOptInAsAWarning(t *testing.T) {
	isolateLLM(t)
	t.Setenv("DOCTOR_LLM_KEY", "k")
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "true")
	t.Setenv("AI_RULEZ_LLM_BASE_URL", "http://gateway.internal/v1")
	t.Setenv("AI_RULEZ_LLM_API_KEY_ENV", "DOCTOR_LLM_KEY")
	t.Setenv("AI_RULEZ_LLM_ALLOW_PLAIN_HTTP", "1")
	t.Setenv("AI_RULEZ_LLM_PLAIN_HTTP_HOSTS", "gateway.internal")
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig + "\n[llm]\nmodel = \"m\"\n"})

	got := byCheck(run(t, dir), CheckLLM)

	for _, f := range got {
		if f.Severity == SeverityWarning && strings.Contains(f.Message, "plain-http opt-in") {
			return
		}
	}
	t.Fatalf("want a plain-http opt-in warning, got %+v", got)
}
