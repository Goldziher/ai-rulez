package improve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const (
	rollbackSkill = "---\nname: rollback\ndescription: Roll back a bad release. Use when asked to roll back, revert or undo a release.\n---\n\n# Rollback\n\nRevert the release.\n"
	rollbackCases = `cases:
  - id: rb-one
    prompt: roll back the checkout release
    expect_trigger: true
  - id: rb-two
    prompt: revert the last release and undo it
    expect_trigger: true
`
)

// withSibling adds a second skill, rollback, with trigger cases to the project.
func withSibling(t *testing.T, configDir string) {
	t.Helper()
	pad := strings.Repeat("Background paragraph about deployments, kept so the description has room to grow. ", 12)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "skills", "deploy", "SKILL.md"), []byte(skillBody+"\n"+pad+"\n"), 0o600))
	for name, body := range map[string]string{
		"skills/rollback/SKILL.md":                rollbackSkill,
		"skills/rollback/evals/trigger.eval.yaml": rollbackCases,
	} {
		p := filepath.Join(configDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
}

// setDescription rewrites the description line of the SKILL.md in dir.
func setDescription(t *testing.T, dir, desc string) {
	t.Helper()
	p := filepath.Join(dir, "SKILL.md")
	lines := strings.Split(readFileString(t, p), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "description:") {
			lines[i] = "description: " + desc + " GOOD"
		}
	}
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600))
}

func TestExecute_SiblingGuardRejectsADescriptionThatStealsASiblingsPrompts(t *testing.T) {
	// Arrange: the candidate passes the held-out gate but its description now wins rollback's prompts.
	root, configDir := project(t)
	withSibling(t, configDir)
	ev := goodEval()
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		setDescription(t, dir, "Deploy, roll back, revert, undo and release. Roll back the checkout release, revert the last release and undo it.")
	})
	o := baseOptions(root, configDir, ev, opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	plan := mustPrepare(t, &o)
	callsBefore := ev.callCount()

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, StatusNoCandidate, report.Status)
	require.Len(t, report.Rounds, 1)
	rd := report.Rounds[0]
	assert.Equal(t, "rejected: regression", rd.Decision)
	require.NotNil(t, rd.Siblings)
	regressed := rd.Siblings.Regressions()
	require.Len(t, regressed, 1)
	assert.Equal(t, "rollback", regressed[0].Skill)
	assert.NotEmpty(t, regressed[0].Stolen)
	assert.Contains(t, strings.Join(rd.Reasons, " "), CodeSiblingRegression)
	assertReportSchema(t, report)
	assert.Nil(t, rd.Held, "no held-out evaluation was spent on a candidate the guard rejects")
	assert.Equal(t, 2, ev.callCount()-callsBefore, "only the two baseline measurements ran (held-out and train)")
	assert.Equal(t, plan.orig.Files[skillFile].Data, []byte(readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md"))), "the authored skill is untouched")
}

func TestExecute_SiblingGuardPassesAnEditThatLeavesTheSiblingAlone(t *testing.T) {
	// Arrange
	root, configDir := project(t)
	withSibling(t, configDir)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		setDescription(t, dir, "Deploy services to staging. Use when asked to deploy, ship or push a service to staging.")
	})
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, StatusAccepted, report.Status)
	sib := report.Rounds[0].Siblings
	require.NotNil(t, sib)
	assert.Empty(t, sib.Regressions())
	require.Len(t, sib.Results, 1)
	assert.Equal(t, "rollback", sib.Results[0].Skill)
	assert.Equal(t, 2, sib.Results[0].Positives)
	assert.Empty(t, sib.Skipped)
}

func TestExecute_SiblingGuardIsSkippedForABodyOnlyEdit(t *testing.T) {
	// Arrange
	root, configDir := project(t)
	withSibling(t, configDir)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) { appendSkill(t, dir, "\nGOOD advice.\n") })
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, StatusAccepted, report.Status)
	require.NotNil(t, report.Rounds[0].Siblings)
	assert.Contains(t, report.Rounds[0].Siblings.Skipped, "ranker reads")
}

func TestExecute_SiblingGuardNeverCopiesTheTargetsEvalCases(t *testing.T) {
	// Arrange: the scratch tree the guard builds must not hold held-out content.
	root, configDir := project(t)
	withSibling(t, configDir)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		setDescription(t, dir, "Deploy services to staging. Use when asked to deploy, ship or push a service to staging.")
	})
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxSkillGrowth = 1, 2
	plan := mustPrepare(t, &o)

	// Act
	_, err := plan.Execute(context.Background())

	// Assert: the scratch directory is removed, and a direct build holds no CANARY text.
	require.NoError(t, err)
	left, _ := filepath.Glob(filepath.Join(tmp, "ai-rulez-improve-siblings-*"))
	assert.Empty(t, left, "the scratch tree is removed")
	scratch := t.TempDir()
	all := mustSkills(t, configDir)
	_, _, err = plan.siblingActivation(context.Background(), scratch, all, plan.orig, []string{"rollback"}, nil, 0)
	require.NoError(t, err)
	require.NoError(t, filepath.WalkDir(scratch, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}
		data, rerr := os.ReadFile(p)
		require.NoError(t, rerr)
		for _, c := range canaries {
			assert.NotContains(t, string(data), c, p)
		}
		return nil
	}))
}

func TestExecute_SiblingGuardSkipsASiblingItCannotCopyAndSaysSo(t *testing.T) {
	tests := []struct {
		name    string
		breakIt func(t *testing.T, dir string)
		why     string
	}{
		{"oversized SKILL.md", func(t *testing.T, dir string) {
			big := rollbackSkill + strings.Repeat("padding line to push the file past the copy bound\n", 25000)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(big), 0o600))
		}, "larger than"},
		{"symlinked SKILL.md", func(t *testing.T, dir string) {
			real := filepath.Join(t.TempDir(), "real.md")
			require.NoError(t, os.WriteFile(real, []byte(rollbackSkill), 0o600))
			require.NoError(t, os.Remove(filepath.Join(dir, "SKILL.md")))
			testutil.SymlinkOrSkip(t, real, filepath.Join(dir, "SKILL.md"))
		}, "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: deploy has one healthy sibling (rollback) and one that cannot be copied (broken).
			root, configDir := project(t)
			withSibling(t, configDir)
			broken := filepath.Join(configDir, "skills", "broken")
			require.NoError(t, os.MkdirAll(filepath.Join(broken, "evals"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(broken, "SKILL.md"), []byte(rollbackSkill), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(broken, "evals", "t.eval.yaml"), []byte(strings.ReplaceAll(rollbackCases, "rb-", "bk-")), 0o600))
			tt.breakIt(t, broken)
			opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
				setDescription(t, dir, "Deploy services to staging. Use when asked to deploy, ship or push a service to staging.")
			})
			o := baseOptions(root, configDir, goodEval(), opt)
			o.MaxRounds, o.MaxSkillGrowth = 1, 2
			plan := mustPrepare(t, &o)

			// Act
			report, err := plan.Execute(context.Background())

			// Assert: the round is judged, not rejected as "guard failed".
			require.NoError(t, err)
			rd := report.Rounds[0]
			assert.Equal(t, "accepted", rd.Decision, "%v", rd.Reasons)
			require.NotNil(t, rd.Siblings)
			require.Len(t, rd.Siblings.Unmeasured, 1)
			assert.Equal(t, "broken", rd.Siblings.Unmeasured[0].Skill)
			assert.Contains(t, rd.Siblings.Unmeasured[0].Reason, tt.why)
			require.Len(t, rd.Siblings.Results, 1)
			assert.Equal(t, "rollback", rd.Siblings.Results[0].Skill)
			assert.Contains(t, strings.Join(rd.Warnings, ";"), "left out broken")
			assertReportSchema(t, report)
		})
	}
}
