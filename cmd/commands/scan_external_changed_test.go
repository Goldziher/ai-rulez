package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestChangedHandsOnlyTheChangedSkillsToTheScanner(t *testing.T) {
	// Arrange: two skills and a scanner that logs the directories it is handed.
	resetStrictFlags(t)
	testutil.IsolateHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	logFile := filepath.Join(t.TempDir(), "runs.log")
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Use when running " + name + " tasks in this repository.\n---\n# " + name + "\n\nSteps.\n"
	}
	root, cfg := strictProject(t, "\n[lint.scanner_policy]\nisolation = \"none\"\n"+scannerEntry(t, "inputs = [\"skills\"]\n", "--log", logFile, "{skill_dirs}"), map[string]string{
		".ai-rulez/skills/alpha/SKILL.md": skill("alpha"),
		".ai-rulez/skills/beta/SKILL.md":  skill("beta"),
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "init")
	oldExt := validateExtern
	t.Cleanup(func() { validateExtern = oldExt })
	validateExtern, validateChanged = true, "HEAD"
	scans := func() string {
		data, err := os.ReadFile(logFile)
		if os.IsNotExist(err) {
			return ""
		}
		require.NoError(t, err)
		return string(data)
	}

	// Act / Assert: nothing changed since HEAD, so the scanner is not started.
	lintProject(t, cfg)
	require.Empty(t, scans())

	// Editing beta hands the scanner beta alone.
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "beta", "SKILL.md"), skill("beta")+"\nEdited.\n")
	report := lintProject(t, loadStrictProject(t, root))
	log := scans()
	require.Equal(t, 1, strings.Count(log, "start "), log)
	require.Contains(t, log, "beta")
	require.NotContains(t, log, "alpha")
	var files []string
	for i := range report.Findings {
		if report.Findings[i].Code == lint.CodeExternalFinding {
			files = append(files, report.Findings[i].RepoPath())
		}
	}
	require.Equal(t, []string{".ai-rulez/skills/beta/SKILL.md"}, files)
}
