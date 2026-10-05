package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaLLMTable(t *testing.T) {
	write := func(body string) string {
		p := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(p, []byte("version = \"4.0\"\nname = \"t\"\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok := write("[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o-mini\"\nbackend = \"auto\"\nbase_url = \"https://gw/v1\"\napi_key_env = \"OPENAI_API_KEY\"\nmax_cost_usd = 2.5\nmax_tokens = 100000\ncache = true\nallow_network = false\n")
	if err := ValidateFile(ok); err != nil {
		t.Fatalf("valid [llm] rejected: %v", err)
	}
	for name, body := range map[string]string{
		"literal key":     "[llm]\napi_key = \"sk-abc\"\n",
		"unknown backend": "[llm]\nbackend = \"litellm\"\n",
		"key as env name": "[llm]\napi_key_env = \"sk-proj-abc123\"\n",
		"negative cap":    "[llm]\nmax_cost_usd = -1\n",
	} {
		if err := ValidateFile(write(body)); err == nil {
			t.Errorf("%s: schema accepted it", name)
		}
	}
}
