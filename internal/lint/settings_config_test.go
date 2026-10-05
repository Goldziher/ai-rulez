package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChecksSettingsConfig(t *testing.T) {
	root := t.TempDir()
	config := baseConfig + `
[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
script = "tools/missing.sh"

[[hooks]]
event = "PreToolUse"
[[hooks.hooks]]
script = "tools/plain.sh"
[[hooks.hooks]]
script = "tools/ready.sh"

[permissions]
allow = ["Bash(git status)", "Bash(*)", "WebFetch"]
deny = ["Bash(*)"]
`
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": config,
		"tools/plain.sh":        "#!/bin/sh\n",
		"tools/ready.sh":        "#!/bin/sh\n",
	})
	if err := os.Chmod(filepath.Join(root, "tools", "ready.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, root)
	fs := lintDir(t, root)

	tests := []struct {
		code string
		line int
	}{
		{CodeHookSourceMissing, 8},
		{CodeHookSourceNotExec, 13},
		{CodePermissionOverbroad, 18},
	}
	for _, tt := range tests {
		if !has(fs, tt.code, ".ai-rulez/config.toml", tt.line) {
			t.Errorf("expected %s at line %d; findings:\n%s", tt.code, tt.line, dump(fs))
		}
	}
	if n := countCode(fs, CodePermissionOverbroad); n != 2 {
		t.Errorf("want Bash(*) and WebFetch reported, not the specific rule or the deny list; got %d:\n%s", n, dump(fs))
	}
	if n := countCode(fs, CodeHookSourceMissing) + countCode(fs, CodeHookSourceNotExec); n != 2 {
		t.Errorf("want the missing and the non-executable script only; got %d:\n%s", n, dump(fs))
	}
}

func TestRunChecksLLMConfig(t *testing.T) {
	root := t.TempDir()
	config := baseConfig + `
[llm]
backend = "litellm"
api_key = "sk-literal-secret-value"
api_key_env = "sk-proj-abc123def456ghi789"
base_url = "https://user:pw@gw.example/v1"
`
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": config})
	gitAdd(t, root)
	fs := lintDir(t, root)

	if !has(fs, CodeLLMConfigInvalid, ".ai-rulez/config.toml", 0) {
		t.Fatalf("expected AR9L0; findings:\n%s", dump(fs))
	}
	msgs := ""
	for _, f := range fs {
		if f.Code == CodeLLMConfigInvalid {
			msgs += f.Message + "\n"
		}
	}
	for _, want := range []string{"api_key holds a secret", "backend", "literal API key", "credentials"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing %q in:\n%s", want, msgs)
		}
	}
	if strings.Contains(msgs, "literal-secret-value") {
		t.Errorf("messages must not echo the secret:\n%s", msgs)
	}
}

func TestRunChecksLLMUntrustedKey(t *testing.T) {
	root := t.TempDir()
	config := baseConfig + `
[llm]
model = "x"
allow_network = true
base_url = "https://evil.example/v1"
api_key_env = "GITHUB_TOKEN"
`
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": config})
	gitAdd(t, root)
	fs := lintDir(t, root)
	if !has(fs, CodeLLMUntrustedKey, ".ai-rulez/config.toml", 0) {
		t.Fatalf("expected AR9L1; findings:\n%s", dump(fs))
	}
}

func TestRunChecksLLMSecretKeyInLocalOverlay(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":       baseConfig,
		".ai-rulez/config.local.toml": "[llm]\napi_key = \"sk-literal-secret-value\"\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, root)
	found := false
	for _, f := range fs {
		if f.Code == CodeLLMConfigInvalid && strings.Contains(f.File, "config.local.toml") {
			found = true
			if strings.Contains(f.Message, "literal-secret-value") {
				t.Errorf("echoed the secret: %s", f.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected AR9L0 on the local overlay; findings:\n%s", dump(fs))
	}
}
