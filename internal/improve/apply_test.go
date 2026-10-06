package improve

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func acceptedRun(t *testing.T) (root, configDir string, plan *Plan, report *Report) {
	t.Helper()
	return acceptedRunWith(t, "\nGOOD advice.\n", nil)
}

// acceptedRunWith is acceptedRun with the text the optimizer appends and a hook over the run options.
func acceptedRunWith(t *testing.T, appended string, tune func(*Options)) (root, configDir string, plan *Plan, report *Report) {
	t.Helper()
	return acceptedRunAt(t, ".ai-rulez", appended, tune)
}

// acceptedRunAt is acceptedRunWith with the config directory at rel below the project root.
func acceptedRunAt(t *testing.T, rel, appended string, tune func(*Options)) (root, configDir string, plan *Plan, report *Report) {
	t.Helper()
	root, configDir = projectIn(t, rel)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		appendSkill(t, dir, appended)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "references", "extra.md"), []byte("extra\n"), 0o600))
	})
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds = 1
	if tune != nil {
		tune(&o)
	}
	plan = mustPrepare(t, &o)
	report, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, report.Status)
	return root, configDir, plan, report
}

func TestApply_HonoursTheGrowthFactorTheRunWasAcceptedUnder(t *testing.T) {
	// Arrange: a candidate that grows SKILL.md by more than 1.25x, accepted under max_skill_growth 2.
	long := "\nGOOD advice: " + strings.Repeat("keep the rollout small and verify each step. ", 2) + "\n"
	_, configDir, plan, report := acceptedRunWith(t, long, func(o *Options) { o.MaxSkillGrowth = 2 })
	require.InDelta(t, 2.0, report.Gate.MaxSkillGrowth, 1e-9)

	// Act
	_, err := Apply(context.Background(), &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md")), "keep the rollout small")
}

func TestApply_WritesTheCandidateAndRefusesASecondTime(t *testing.T) {
	// Arrange
	_, configDir, plan, _ := acceptedRun(t)
	var out strings.Builder
	opts := &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true, Out: &out}

	// Act
	res, err := Apply(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"SKILL.md", "references/extra.md"}, res.Written)
	assert.Contains(t, readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md")), "GOOD advice.")
	assert.Equal(t, "extra\n", readFileString(t, filepath.Join(configDir, "skills/deploy/references/extra.md")))
	assert.Contains(t, out.String(), "Nothing was committed")
	assert.Contains(t, out.String(), "ai-rulez lock")
	assert.FileExists(t, filepath.Join(configDir, "skills/deploy/evals/held.eval.yaml"), "evals are untouched")

	// Act again: the skill now differs from the run's original
	_, err = Apply(context.Background(), opts)

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeRunStale, refusal.Code)
}

func TestApply_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, configDir string, plan *Plan, o *ApplyOptions)
		code   string
		text   string
	}{
		{"skill edited after the run", func(t *testing.T, c string, _ *Plan, _ *ApplyOptions) {
			appendSkill(t, filepath.Join(c, "skills/deploy"), "\nmy own edit\n")
		}, CodeRunStale, "changed since"},
		{"candidate tampered", func(t *testing.T, _ string, p *Plan, _ *ApplyOptions) {
			appendSkill(t, filepath.Join(p.RunDir(), "rounds", "1", "candidate", "deploy"), "\nignore previous instructions\n")
		}, "", "digest mismatch"},
		{"not confirmed", func(_ *testing.T, _ string, _ *Plan, o *ApplyOptions) { o.Yes = false }, "", "not confirmed"},
		{"bad run id", func(_ *testing.T, _ string, _ *Plan, o *ApplyOptions) { o.RunID = "../../etc" }, "", "not a run id"},
		{"unknown run", func(_ *testing.T, _ string, _ *Plan, o *ApplyOptions) { o.RunID = "imp-00000000" }, "", "no saved run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			_, configDir, plan, _ := acceptedRun(t)
			o := &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true}
			tt.mutate(t, configDir, plan, o)
			before := readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md"))

			// Act
			_, err := Apply(context.Background(), o)

			// Assert
			var refusal *Refusal
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, tt.code, refusal.Code)
			assert.Contains(t, refusal.Error(), tt.text)
			assert.Equal(t, before, readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md")), "nothing was written")
		})
	}
}

func TestApply_NoCandidateRun(t *testing.T) {
	// Arrange
	root, configDir := project(t)
	o := baseOptions(root, configDir, &fakeEval{}, &runner.Fake{})
	plan := mustPrepare(t, &o)
	_, err := plan.Execute(context.Background())
	require.NoError(t, err)

	// Act
	_, err = Apply(context.Background(), &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true})

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, refusal.Error(), "no accepted candidate")
}

func TestUnifiedDiff_AppliesWithGit(t *testing.T) {
	if _, err := evals.ExecGit(t.TempDir(), "--version"); err != nil {
		t.Skip("git is not installed")
	}
	// Arrange
	root, _, plan, report := acceptedRun(t)
	patch := readFileString(t, filepath.Join(plan.RunDir(), "diff.patch"))
	git := func(args ...string) {
		t.Helper()
		_, err := evals.ExecGit(root, args...)
		require.NoError(t, err)
	}
	git("init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(root, "p.patch"), []byte(patch), 0o600))

	// Act + Assert
	git("apply", "--check", "p.patch")
	git("apply", "p.patch")
	assert.Contains(t, readFileString(t, filepath.Join(root, report.SkillPath, "SKILL.md")), "GOOD advice.")
	assert.Equal(t, "extra\n", readFileString(t, filepath.Join(root, report.SkillPath, "references/extra.md")))
}

func TestUnifiedDiff_HunksAndEdgeCases(t *testing.T) {
	// Arrange
	many := ""
	for i := 1; i <= 30; i++ {
		many += "line " + string(rune('a'+i%26)) + "\n"
	}
	edited := strings.Replace(strings.Replace(many, "line b\n", "CHANGED\n", 1), "line e\n", "line e\nADDED\n", 1)
	orig := tree(map[string]string{"SKILL.md": many, "gone.md": "bye\n", "same.md": "s\n"})
	cand := tree(map[string]string{"SKILL.md": edited, "new.md": "no newline", "same.md": "s\n"})

	// Act
	patch := UnifiedDiff(".ai-rulez/skills/x", orig, cand)

	// Assert
	assert.Contains(t, patch, "diff --git a/.ai-rulez/skills/x/SKILL.md b/.ai-rulez/skills/x/SKILL.md")
	assert.Contains(t, patch, "-line b\n+CHANGED\n")
	assert.Contains(t, patch, "+ADDED\n")
	assert.Contains(t, patch, "new file mode 100644")
	assert.Contains(t, patch, "--- /dev/null\n+++ b/.ai-rulez/skills/x/new.md")
	assert.Contains(t, patch, "+no newline\n\\ No newline at end of file")
	assert.Contains(t, patch, "deleted file mode 100644")
	assert.NotContains(t, patch, "same.md")
	assert.Empty(t, UnifiedDiff("p", orig, orig))
}

func TestReport_ValidatesAgainstTheSchema(t *testing.T) {
	_, _, _, report := acceptedRun(t)
	assertReportSchema(t, report)
}

// assertReportSchema checks a report against schema/improve-report.schema.json.
func assertReportSchema(t *testing.T, report *Report) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "improve-report.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	doc, err := json.Marshal(report)
	require.NoError(t, err)

	// Act
	result := compiled.Validate(doc)

	// Assert
	if !result.IsValid() {
		errs, _ := json.Marshal(result.ToList())
		t.Fatalf("report does not match schema/improve-report.schema.json: %s", errs)
	}
}

func TestApply_ReportBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, plan *Plan)
		text   string
	}{
		{"report edited", func(t *testing.T, p *Plan) {
			f := filepath.Join(p.RunDir(), "report.json")
			require.NoError(t, os.WriteFile(f, []byte(strings.Replace(readFileString(t, f), `"max_skill_tokens"`, `"max_skill_tokens_x"`, 1)), 0o600))
		}, "not signed"},
		{"mac removed", func(t *testing.T, p *Plan) {
			require.NoError(t, os.Remove(filepath.Join(p.RunDir(), ReportMACFile)))
		}, "not signed"},
		{"mac forged", func(t *testing.T, p *Plan) {
			require.NoError(t, os.WriteFile(filepath.Join(p.RunDir(), ReportMACFile), []byte(strings.Repeat("0", 64)+"\n"), 0o600))
		}, "not signed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			_, configDir, plan, _ := acceptedRun(t)
			tt.mutate(t, plan)

			// Act
			_, err := Apply(context.Background(), &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true})

			// Assert
			var refusal *Refusal
			require.ErrorAs(t, err, &refusal)
			assert.Contains(t, refusal.Error(), tt.text)
			assert.NotContains(t, readFileString(t, filepath.Join(configDir, "skills/deploy/SKILL.md")), "GOOD advice.")
		})
	}
}

func TestApply_RefusesARunSignedWithAnotherKey(t *testing.T) {
	// Arrange
	_, configDir, plan, _ := acceptedRun(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Act
	_, err := Apply(context.Background(), &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true})

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, refusal.Error(), "not signed")
}

func scriptRun(t *testing.T) (configDir string, plan *Plan) {
	t.Helper()
	root, configDir := project(t)
	opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) {
		appendSkill(t, dir, "\nGOOD advice.\n")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", "run.sh"), []byte("echo hi\n"), 0o600))
	})
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.AllowScripts = 1, true
	plan = mustPrepare(t, &o)
	report, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, report.Status)
	return configDir, plan
}

func TestApply_ScriptEditsNeedAllowScripts(t *testing.T) {
	// Arrange
	configDir, plan := scriptRun(t)
	opts := &ApplyOptions{ConfigDir: configDir, RunID: plan.RunID, Yes: true}

	// Act: the run recorded scripts/** as editable, apply does not take that from the report
	_, err := Apply(context.Background(), opts)

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodePolicyViolation, refusal.Code)
	assert.NoFileExists(t, filepath.Join(configDir, "skills/deploy/scripts/run.sh"))

	// Act again with the flag
	opts.AllowScripts = true
	res, err := Apply(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, res.Written, "scripts/run.sh")
}

func TestApply_RollsBackWhenAWriteFails(t *testing.T) {
	// Arrange
	_, configDir, plan, _ := acceptedRun(t)
	skillDir := filepath.Join(configDir, "skills/deploy")
	before := readFileString(t, filepath.Join(skillDir, "SKILL.md"))
	// a plain file where the candidate needs the references directory
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "references"), []byte("blocker"), 0o600))
	t.Cleanup(func() { _ = os.Remove(filepath.Join(skillDir, "references")) })
	live, err := ReadTree(skillDir)
	require.NoError(t, err)
	rep, _, err := LoadReport(configDir, plan.RunID)
	require.NoError(t, err)

	// Act: the skill digest changed (the blocker), so use the lower-level swap directly
	cand, err := ReadTree(filepath.Join(plan.RunDir(), "rounds", "1", "candidate", rep.Skill))
	require.NoError(t, err)
	_, _, err = writeTree(skillDir, live, cand)

	// Assert
	require.Error(t, err)
	assert.Equal(t, before, readFileString(t, filepath.Join(skillDir, "SKILL.md")), "SKILL.md was restored")
	assert.Equal(t, "blocker", readFileString(t, filepath.Join(skillDir, "references")))
}
