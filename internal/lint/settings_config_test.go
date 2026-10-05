package lint

import (
	"os"
	"path/filepath"
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
