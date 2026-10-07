package airulez_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// withOverlay is the shared sources plus a machine-local overlay and local
// content, which make Check and DryRun render the shared baseline again.
func withOverlay() map[string]string {
	files := map[string]string{}
	for k, v := range sources {
		files[k] = v
	}
	files[".ai-rulez/config.local.toml"] = "name = \"overlaid\"\n"
	files[".ai-rulez/local/rules/mine.md"] = "# Mine\n"
	return files
}

// TestLocalOverlayReloadStaysInTheWorkspace is RV-ENGINE-3: the shared
// baseline Check and DryRun render with a machine-local overlay is a second
// load of the project, which must read the same workspace as the first one. It
// re-read the real disk, so a project in memory failed with "stat config path".
func TestLocalOverlayReloadStaysInTheWorkspace(t *testing.T) {
	tests := []struct {
		name string
		mode airulez.Mode
	}{
		{name: "check", mode: airulez.Check},
		{name: "dry run", mode: airulez.DryRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ws := memWith(withOverlay())
			p, err := airulez.Load(t.Context(), airulez.Options{Workspace: ws, WithLocal: true})
			require.NoError(t, err)
			require.Equal(t, "overlaid", p.Name())

			// Act
			_, err = p.Generate(t.Context(), airulez.GenerateOptions{Mode: tt.mode})

			// Assert
			require.NoError(t, err)
		})
	}
}

// TestLocalOverlayReloadStartsNoProcess is RV-ENGINE-3 on disk: the baseline
// reload must start its commands through Options.Runner, so with a denying
// runner the real git is never run. A git on PATH that records its calls
// proves it.
func TestLocalOverlayReloadStartsNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the recording git is a shell script")
	}
	// Arrange
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho \"$@\" >> " + calls + "\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755)) //nolint:gosec // test fake
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	resolved, err := exec.LookPath("git")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(bin, "git"), resolved, "the recording git must resolve first")
	dir := t.TempDir()
	for name, content := range withOverlay() {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	ws, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	p, err := airulez.Load(t.Context(), airulez.Options{Workspace: ws, WithLocal: true})
	require.NoError(t, err)

	// Act
	_, err = p.Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.DryRun})

	// Assert
	require.NoError(t, err)
	_, statErr := os.Stat(calls)
	assert.True(t, os.IsNotExist(statErr), "git ran outside Options.Runner: %s", readOrEmpty(calls))
}

func readOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
