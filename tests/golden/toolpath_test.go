package golden

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBaseEnv_PathIsTheSameOnEveryMachine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("goldens record POSIX modes and paths")
	}
	// Arrange
	binary(t)

	// Act
	var path string
	for _, kv := range baseEnv(t.TempDir(), nil) {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}

	// Assert: the stub directory, then only the system directories git and sh live in.
	dirs := filepath.SplitList(path)
	if len(dirs) == 0 {
		t.Fatal("baseEnv sets no PATH")
	}
	if want := append([]string{dirs[0]}, systemPath...); !slices.Equal(dirs, want) {
		t.Fatalf("PATH = %q, want %q", path, strings.Join(want, string(os.PathListSeparator)))
	}
	entries, err := os.ReadDir(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	tests := []struct {
		tool    string
		present bool
	}{
		{"claude", true}, {"codex", true}, {"copilot", true}, {"opencode", true}, {"npx", true}, {"git", true},
		{"cursor", false}, {"cursor-agent", false}, {"gemini", false}, {"personal-mcp", false},
	}
	for _, tc := range tests {
		if got := slices.Contains(names, tc.tool); got != tc.present {
			t.Errorf("%s on the golden PATH = %v, want %v", tc.tool, got, tc.present)
		}
	}
}
