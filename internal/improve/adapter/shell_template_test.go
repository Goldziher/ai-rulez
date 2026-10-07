package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellTemplate_RefusesARequestWithoutASkillDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the template is a POSIX sh script")
	}
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("jq not available")
	}
	sh, err := exec.LookPath("sh")
	require.NoError(t, err)
	tests := []struct {
		name     string
		request  string
		wantOK   bool
		wantTool string // the arguments the tool was called with, "" when it must not run
		wantErr  string
	}{
		{"a skill dir", `{"version":1,"skill":{"dir":"deploy"},"train_cases":[]}`, true, "rewrite --skill deploy/SKILL.md --cases-from-stdin", ""},
		{"no skill key", `{"version":1,"train_cases":[]}`, false, "", "no skill.dir string"},
		{"a null dir", `{"version":1,"skill":{"dir":null}}`, false, "", "no skill.dir string"},
		{"a non-string dir", `{"version":1,"skill":{"dir":7}}`, false, "", "no skill.dir string"},
	}
	text, ok := Template(Shell)
	require.True(t, ok)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the template with a fake your-tool, first on PATH, that records its arguments.
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			require.NoError(t, os.MkdirAll(bin, 0o750))
			log := filepath.Join(dir, "tool.log")
			fake := "#!/bin/sh\necho \"$*\" > '" + log + "'\ncat > /dev/null\n"
			require.NoError(t, os.WriteFile(filepath.Join(bin, "your-tool"), []byte(fake), 0o700)) //nolint:gosec // a test script
			script := filepath.Join(dir, "optimize.sh")
			require.NoError(t, os.WriteFile(script, []byte(text), 0o600))
			cmd := exec.Command(sh, script) //nolint:gosec // the bundled template
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + filepath.Dir(jq) + ":/usr/bin:/bin"}
			cmd.Stdin = strings.NewReader(tt.request)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			resolved, lerr := exec.LookPath(filepath.Join(bin, "your-tool"))
			require.NoError(t, lerr)
			require.Equal(t, filepath.Join(bin, "your-tool"), resolved, "the fake tool resolves first")

			// Act
			runErr := cmd.Run()

			// Assert
			called, _ := os.ReadFile(log) //nolint:gosec // test file
			if tt.wantOK {
				require.NoError(t, runErr, stderr.String())
				assert.Equal(t, tt.wantTool, strings.TrimSpace(string(called)))
				assert.Contains(t, stdout.String(), `"version":1`)
				return
			}
			require.Error(t, runErr)
			assert.Contains(t, stderr.String(), tt.wantErr)
			assert.Empty(t, called, "the tool never ran on null/SKILL.md")
			assert.Empty(t, stdout.String())
		})
	}
}
