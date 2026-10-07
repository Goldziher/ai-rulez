package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	testutil.SymlinkOrSkip(t, target, link)
}

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestProjectScanner_SymlinkPolicy(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, project, outside string)
		wantRules    []string
		wantProblems int
	}{
		{
			name: "file symlink inside project is followed",
			setup: func(t *testing.T, project, _ string) {
				write(t, filepath.Join(project, "docs", "shared.md"), "# shared\n")
				symlinkOrSkip(t, filepath.Join(project, "docs", "shared.md"), filepath.Join(project, ".ai-rulez", "rules", "shared.md"))
			},
			wantRules: []string{"shared"},
		},
		{
			name: "file symlink outside project is refused",
			setup: func(t *testing.T, project, outside string) {
				write(t, filepath.Join(outside, "hosts.md"), "SECRET")
				symlinkOrSkip(t, filepath.Join(outside, "hosts.md"), filepath.Join(project, ".ai-rulez", "rules", "leak.md"))
			},
			wantProblems: 1,
		},
		{
			name: "rules directory symlinked inside project is followed",
			setup: func(t *testing.T, project, _ string) {
				write(t, filepath.Join(project, "shared-rules", "a.md"), "# a\n")
				symlinkOrSkip(t, filepath.Join(project, "shared-rules"), filepath.Join(project, ".ai-rulez", "rules"))
			},
			wantRules: []string{"a"},
		},
		{
			name: "rules directory symlinked outside project is refused",
			setup: func(t *testing.T, project, outside string) {
				write(t, filepath.Join(outside, "rules", "a.md"), "# a\n")
				symlinkOrSkip(t, filepath.Join(outside, "rules"), filepath.Join(project, ".ai-rulez", "rules"))
			},
			wantProblems: 1,
		},
		{
			name: "dangling symlink is refused",
			setup: func(t *testing.T, project, _ string) {
				write(t, filepath.Join(project, ".ai-rulez", "rules", "keep.md"), "# k\n")
				symlinkOrSkip(t, filepath.Join(project, "missing.md"), filepath.Join(project, ".ai-rulez", "rules", "gone.md"))
			},
			wantRules:    []string{"keep"},
			wantProblems: 1,
		},
		{
			name: "domain symlinked outside project is refused",
			setup: func(t *testing.T, project, outside string) {
				write(t, filepath.Join(outside, "dom", "rules", "a.md"), "# a\n")
				write(t, filepath.Join(project, ".ai-rulez", "domains", ".keep"), "")
				symlinkOrSkip(t, filepath.Join(outside, "dom"), filepath.Join(project, ".ai-rulez", "domains", "evil"))
			},
			wantProblems: 1,
		},
		{
			name: "whole config dir symlinked outside project is refused",
			setup: func(t *testing.T, project, outside string) {
				write(t, filepath.Join(outside, "cfg", "rules", "a.md"), "# a\n")
				symlinkOrSkip(t, filepath.Join(outside, "cfg"), filepath.Join(project, ".ai-rulez"))
			},
			wantProblems: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			outside := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez", "rules"), 0o755))
			if tt.name == "rules directory symlinked inside project is followed" ||
				tt.name == "rules directory symlinked outside project is refused" {
				require.NoError(t, os.Remove(filepath.Join(project, ".ai-rulez", "rules")))
			}
			if tt.name == "whole config dir symlinked outside project is refused" {
				require.NoError(t, os.RemoveAll(filepath.Join(project, ".ai-rulez")))
			}
			tt.setup(t, project, outside)
			warned := &testutil.LogRecorder{}
			s := newProjectScanner(osView(project))
			s.log = warned

			// Act
			tree, err := scanContentTree(s, filepath.Join(project, ".ai-rulez"), nil)

			// Assert
			require.NoError(t, err)
			var names []string
			for _, r := range tree.Rules {
				names = append(names, r.Name)
			}
			assert.ElementsMatch(t, tt.wantRules, names)
			assert.Len(t, s.problems, tt.wantProblems)
			if tt.wantProblems > 0 {
				assert.Contains(t, warned.String(), "refusing symlinked content")
				assert.NotContains(t, warned.String(), "SECRET")
			}
		})
	}
}

func TestIncludeScanner_NeverFollowsSymlinkedDirectories(t *testing.T) {
	// Arrange
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "rules")
	write(t, filepath.Join(target, "a.md"), "# a\n")
	symlinkOrSkip(t, target, filepath.Join(root, "rules"))
	write(t, filepath.Join(root, "agents", "b.md"), "# b\n")

	// Act
	tree, err := ScanContentTree(root)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, tree.Rules)
	assert.Len(t, tree.Agents, 1)
}

func TestConfigValidate_ReportsContentProblems(t *testing.T) {
	// Arrange
	cfg := &Config{ContentProblems: []ContentProblem{{Path: "/p/.ai-rulez/rules/x.md", Reason: "outside"}}}

	// Act
	err := cfg.validateContentProblems()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/p/.ai-rulez/rules/x.md")
	assert.NoError(t, (&Config{}).validateContentProblems())
}
