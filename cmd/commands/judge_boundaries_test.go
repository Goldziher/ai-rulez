package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestJudgeRepositoryRubricsStayInsideProject(t *testing.T) {
	root := gitutil.Resolve(t.TempDir())
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("## C1 - Private\nPrivate criterion.\n"), 0o600))
	w := &judgeWorkspace{root: root, cwd: root}
	repo := &config.JudgeConfig{Skills: &config.JudgeSkills{Load: []string{"path:" + outside}}}
	err := (judgeDeps{}).loadSkills(&judgeFlags{}, w, repo, &judgeSettings{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the project")
	// An explicit CLI path is the user's choice and may name an external rubric.
	_, err = w.loadRubric("path:"+outside, "", false)
	require.NoError(t, err)
}

func TestJudgeRepositoryTrapsStayInsideProject(t *testing.T) {
	root := gitutil.Resolve(t.TempDir())
	outside := t.TempDir()
	file := filepath.Join(outside, "traps.yaml")
	require.NoError(t, os.WriteFile(file, []byte("- id: private\n  text: Private text.\n"), 0o600))
	rel, err := filepath.Rel(root, file)
	require.NoError(t, err)
	w := &judgeWorkspace{root: root, cwd: root}
	for _, name := range []string{file, rel} {
		t.Run(name, func(t *testing.T) {
			err := loadTraps(&judge.PromptSet{}, w, &config.JudgeConfig{TrapsFile: name})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "inside the project")
		})
	}
}

func TestJudgeRepositoryRubricPathResolvesFromProjectRoot(t *testing.T) {
	root := gitutil.Resolve(t.TempDir())
	cwd := filepath.Join(root, "nested")
	rubricDir := filepath.Join(root, "rubrics", "review")
	require.NoError(t, os.MkdirAll(cwd, 0o700))
	require.NoError(t, os.MkdirAll(rubricDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(rubricDir, "SKILL.md"), []byte("## C1 - Root criterion\nUse the root rubric.\n"), 0o600))
	w := &judgeWorkspace{root: root, cwd: cwd}
	repo := &config.JudgeConfig{Skills: &config.JudgeSkills{Load: []string{"path:rubrics/review"}}}
	settings := &judgeSettings{}
	require.NoError(t, (judgeDeps{}).loadSkills(&judgeFlags{}, w, repo, settings))
	require.Len(t, settings.opts.Rubrics[judge.CategoryGovernance], 1)
	assert.Equal(t, "Root criterion", settings.opts.Rubrics[judge.CategoryGovernance][0].Criteria[0].Title)
}

func TestJudgeRepositoryPromptInputsRefuseSymlinkedParents(t *testing.T) {
	root := gitutil.Resolve(t.TempDir())
	real := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(real, 0o700))
	for name, text := range map[string]string{
		"SKILL.md":   "## C1 - Criterion\nUse a heading.\n",
		"traps.yaml": "- id: safe\n  text: An example.\n",
		"prompt.md":  "Review carefully.\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(real, name), []byte(text), 0o600))
	}
	testutil.SymlinkOrSkip(t, real, filepath.Join(root, "linked"))
	w := &judgeWorkspace{root: root, cwd: root}
	t.Run("rubric", func(t *testing.T) {
		repo := &config.JudgeConfig{Skills: &config.JudgeSkills{Load: []string{"path:linked"}}}
		require.Error(t, (judgeDeps{}).loadSkills(&judgeFlags{}, w, repo, &judgeSettings{}))
	})
	t.Run("traps", func(t *testing.T) {
		require.Error(t, loadTraps(&judge.PromptSet{}, w, &config.JudgeConfig{TrapsFile: "linked/traps.yaml"}))
	})
	t.Run("prompt", func(t *testing.T) {
		_, err := readPromptFile(w, "security", config.JudgePrompt{File: "linked/prompt.md"}, true, "")
		require.Error(t, err)
	})
}
