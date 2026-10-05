package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func llmProject(t *testing.T, llmTable string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + llmTable
	if err := os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(strings.Repeat("hello world ", 500)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("LLM_CMD_TEST_KEY", "sk-never-printed-value-123")
}

func TestLLMDoctorPrintsResolvedSetupWithoutSecrets(t *testing.T) {
	llmProject(t, "\n[llm]\nprovider = \"openai\"\nmodel = \"gpt-4o-mini\"\nbase_url = \"https://gateway.internal/v1\"\napi_key_env = \"LLM_CMD_TEST_KEY\"\n")
	llmJSON, llmPing = false, true
	t.Cleanup(func() { llmPing = false })
	var out bytes.Buffer
	err := runLLMDoctor(context.Background(), nil, &out)
	text := out.String()
	if strings.Contains(text, "never-printed") || !strings.Contains(text, "gateway.internal") || !strings.Contains(text, "network allowed: false") {
		t.Fatalf("doctor output:\n%s", text)
	}
	// --ping with allow_network off must fail with the gate message, not reach the network
	if err == nil || !strings.Contains(text, "ping:            failed") || !strings.Contains(text, "allow_network") {
		t.Fatalf("ping must be refused: err=%v\n%s", err, text)
	}
}

func TestLLMEstimate(t *testing.T) {
	llmProject(t, "\n[llm]\nmodel = \"gpt-4o-mini\"\n")
	llmJSON, llmMaxOutput = false, 100
	var out bytes.Buffer
	if err := runLLMEstimate(context.Background(), "prompt.txt", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gpt-4o-mini") || !strings.Contains(out.String(), "nothing was sent") || strings.Contains(out.String(), "unknown") {
		t.Fatalf("estimate output:\n%s", out.String())
	}
}
