package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// TestRootForNamedPaths covers the tree root of a run whose --path names a file
// above the configuration's directory in a tree git does not track (#470).
func TestRootForNamedPaths(t *testing.T) {
	tests := []struct {
		name     string
		git      bool
		repoRoot bool
		cwd      string // relative to the tree
		paths    []string
		want     string // relative to the tree; "" keeps the default lookup
	}{
		{name: "path above the configuration, run from the tree", cwd: ".", paths: []string{"docs/AGENTS.md"}, want: "."},
		{name: "absolute path above the configuration", cwd: ".", paths: []string{"<abs>docs/AGENTS.md"}, want: "."},
		{name: "path inside the configuration's directory", cwd: ".", paths: []string{"sub/NOTES.md"}},
		{name: "configuration outside the working directory", cwd: "docs", paths: []string{"AGENTS.md"}},
		{name: "--repo-root keeps its root", repoRoot: true, cwd: ".", paths: []string{"docs/AGENTS.md"}},
		{name: "git work tree keeps its root", git: true, cwd: ".", paths: []string{"docs/AGENTS.md"}},
		{name: "no --path", cwd: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := t.TempDir()
			writeFile(t, filepath.Join(tree, "sub", ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"t\"\n")
			writeFile(t, filepath.Join(tree, "sub", "NOTES.md"), "# Notes\n")
			writeFile(t, filepath.Join(tree, "docs", "AGENTS.md"), "# Agents\n")
			if tt.git {
				crossGit(t, tree, "init", "-q")
			}
			saved := strictTreeCache
			t.Cleanup(func() { strictTreeCache = saved })
			strictTreeCache = &lint.Loader{}
			if tt.repoRoot {
				strictTreeCache = &lint.Loader{Root: filepath.Join(tree, "sub")}
			}
			var paths []string
			for _, p := range tt.paths {
				if rest, ok := strings.CutPrefix(p, "<abs>"); ok {
					p = filepath.Join(tree, filepath.FromSlash(rest))
				}
				paths = append(paths, filepath.FromSlash(p))
			}
			setTargetFlags(t, paths, nil)
			t.Chdir(filepath.Join(tree, tt.cwd))

			rootForNamedPaths(t.Context(), filepath.Join(tree, "sub"))

			want := ""
			switch {
			case tt.repoRoot:
				want = filepath.Join(tree, "sub")
			case tt.want != "":
				want = gitutil.Resolve(filepath.Join(tree, tt.want))
			}
			assert.Equal(t, want, strictTreeCache.Root)
		})
	}
}
