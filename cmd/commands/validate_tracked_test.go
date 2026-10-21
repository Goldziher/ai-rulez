package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const (
	trackedSkill = "---\nname: sa\ndescription: a skill that does a thing\n---\n# sa\n\nBody.\n"
	pycRel       = ".ai-rulez/skills/sa/scripts/__pycache__/run.cpython-311.pyc"
)

// trackedRepo commits a clean skill, then leaves an untracked, not-ignored
// bytecode file behind, as `python -m py_compile` does.
func trackedRepo(t *testing.T, configExtra string) (string, *config.Config) {
	t.Helper()
	root, _ := strictProject(t, configExtra, map[string]string{
		".ai-rulez/skills/sa/SKILL.md":       trackedSkill,
		".ai-rulez/skills/sa/scripts/run.py": "print('hi')\n",
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "init")
	writeFile(t, filepath.Join(root, filepath.FromSlash(pycRel)), "\x00\x00\x00\x00pyc")
	return root, loadStrictProject(t, root)
}

func reportFiles(r *lint.Report) map[string]bool {
	files := map[string]bool{}
	for i := range r.Findings {
		files[r.Findings[i].RepoPath()] = true
	}
	return files
}

func TestTrackedOnlyLintsTrackedFilesOnly(t *testing.T) {
	tests := []struct {
		name        string
		configExtra string
		flag        bool
		changed     string
		wantPyc     bool
	}{
		{"default lints the untracked file", "", false, "", true},
		{"flag drops it", "", true, "", false},
		{"config key drops it", "\n[lint]\ntracked_only = true\n", false, "", false},
		{"changed adds it back", "", true, "HEAD", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetStrictFlags(t)
			t.Cleanup(func() { validateTrackedOnly = false })
			_, cfg := trackedRepo(t, tt.configExtra)
			validateTrackedOnly, validateChanged = tt.flag, tt.changed

			report := lintProject(t, cfg)

			assert.Equal(t, tt.wantPyc, reportFiles(report)[pycRel], "%v", reportFiles(report))
		})
	}
}

func TestTrackedOnlyOutsideGitLintsEverything(t *testing.T) {
	resetStrictFlags(t)
	t.Cleanup(func() { validateTrackedOnly = false })
	_, cfg := strictProject(t, "", map[string]string{
		".ai-rulez/skills/sa/SKILL.md":                  trackedSkill,
		".ai-rulez/skills/sa/scripts/__pycache__/x.pyc": "\x00\x00\x00\x00pyc",
	})
	validateTrackedOnly = true

	report := lintProject(t, cfg)

	assert.True(t, reportFiles(report)[".ai-rulez/skills/sa/scripts/__pycache__/x.pyc"])
}

func TestUpdateBaselineSkipsUntrackedFiles(t *testing.T) {
	resetStrictFlags(t)
	root, cfg := trackedRepo(t, "")
	validateUpdateBaseline, validateBaselineReason = true, "r"
	report := lintProject(t, cfg)
	require.True(t, reportFiles(report)[pycRel], "the untracked file is linted without --tracked-only")

	require.NoError(t, updateBaselines([]*lint.Report{report}, []*config.Config{cfg}))

	b, err := lint.LoadBaseline(filepath.Join(root, ".ai-rulez", lint.BaselineFile))
	require.NoError(t, err)
	for _, e := range b.Entries {
		assert.NotEqual(t, pycRel, e.File, "an untracked file is not accepted into the baseline")
	}
}
