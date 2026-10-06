package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The helper process tells its two roles apart by the protocol variable each
// caller sets for its child.
const (
	helperOptimizerEnv = "AI_RULEZ_IMPROVE_PROTOCOL"
	helperEvalEnv      = "AI_RULEZ_EVAL_PROTOCOL"
)

// TestImproveHelperProcess is not a test: the fake optimizer and the fake eval
// runner re-execute the test binary through it.
func TestImproveHelperProcess(t *testing.T) {
	switch {
	case os.Getenv(helperOptimizerEnv) != "":
		var req improve.OptimizerRequest
		if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
			os.Exit(3)
		}
		path := filepath.Join(req.Skill.Dir, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			os.Exit(4)
		}
		if err := os.WriteFile(path, append(data, []byte("\nGOOD advice.\n")...), 0o600); err != nil {
			os.Exit(5)
		}
		_, _ = os.Stdout.WriteString(`{"version":1,"summary":"added advice","changed":["SKILL.md"],"cost_usd":0.01}`)
		os.Exit(0)
	case os.Getenv(helperEvalEnv) != "":
		var req evals.Request
		if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
			os.Exit(3)
		}
		data, _ := os.ReadFile(filepath.Join(req.Skill.Dir, "SKILL.md")) //nolint:errcheck // a missing file fails the cases
		good := strings.Contains(string(data), "GOOD")
		resp := evals.Response{Version: 1}
		for i := range req.Cases {
			c := &req.Cases[i]
			trig, pass := c.Expects(), good || !c.Expects()
			resp.Results = append(resp.Results, evals.Result{Case: c.ID, Arm: evals.ArmWith, Triggered: &trig, Passed: &pass, CostUSD: 0.01})
		}
		out, _ := json.Marshal(resp)
		_, _ = os.Stdout.Write(out)
		os.Exit(0)
	}
}

func resetImproveFlags(t *testing.T) {
	t.Helper()
	prevDir := configDir
	reset := func() {
		improveFlags.with, improveFlags.holdoutTag, improveFlags.holdoutFraction = "", improve.DefaultHoldoutTag, improve.DefaultHoldoutFraction
		improveFlags.minGain, improveFlags.maxRegressions = improve.DefaultMinGain, 0
		improveFlags.maxRounds, improveFlags.maxHoldoutEvals, improveFlags.maxCost, improveFlags.runs = improve.DefaultMaxRounds, improve.DefaultMaxHoldoutEvals, 0, improve.DefaultRuns
		improveFlags.timeout, improveFlags.evalTimeout = improve.DefaultTimeout, 30*time.Minute
		improveFlags.harness, improveFlags.model, improveFlags.runnerCommand, improveFlags.claudeBin = "claude", "", "", "claude"
		improveFlags.allowFrontmatter, improveFlags.allowScripts, improveFlags.envPass, improveFlags.egress = false, false, nil, nil
		improveFlags.yes, improveFlags.dryRun, improveFlags.stopAtFirstAccept, improveFlags.format = false, false, false, formatText
		improveFlags.date, improveFlags.priceIn, improveFlags.priceOut = "", 0, 0
		configDir = prevDir
	}
	reset()
	t.Cleanup(reset)
}

const improveSkillBody = "---\nname: deploy\ndescription: Deploy things. Use when deploying.\n---\n\nRun the deploy.\n"

func improveProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml":            "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n",
		".ai-rulez/skills/deploy/SKILL.md": improveSkillBody,
		".ai-rulez/skills/deploy/evals/a.eval.yaml": `cases:
  - id: train-a
    prompt: deploy it
    expect_trigger: true
    assertions: [{type: contains, value: ok}]
  - id: train-b
    prompt: ship it
    expect_trigger: true
    assertions: [{type: contains, value: ok}]
  - id: held-a
    prompt: roll it out
    expect_trigger: true
    assertions: [{type: contains, value: ok}]
    tags: [holdout]
  - id: held-b
    prompt: push it
    expect_trigger: true
    assertions: [{type: contains, value: ok}]
    tags: [holdout]
  - id: held-n
    prompt: what is a deploy
    expect_trigger: false
    tags: [holdout]
`,
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	t.Chdir(root)
	return root
}

func helperCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe + " -test.run=TestImproveHelperProcess"
}

func setupImproveRun(t *testing.T) string {
	t.Helper()
	resetImproveFlags(t)
	root := improveProject(t)
	exe, err := os.Executable()
	require.NoError(t, err)
	argv, err := json.Marshal([]string{exe, "-test.run=TestImproveHelperProcess"})
	require.NoError(t, err)
	improveFlags.with = string(argv)
	improveFlags.runnerCommand = helperCommand(t)
	improveFlags.maxCost, improveFlags.runs, improveFlags.maxRounds = 5, 1, 1
	improveFlags.yes = true
	improveFlags.format = formatJSON
	improveFlags.date = "2026-10-06"
	return root
}

func TestImprove_EndToEndAcceptsAndApplies(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // the report MAC key stays out of the real home
	root := setupImproveRun(t)
	var out, errOut bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&errOut)

	// Act
	noCandidate, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.NoError(t, err)
	assert.False(t, noCandidate)
	assert.Contains(t, errOut.String(), "experimental")
	var report improve.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, improve.StatusAccepted, report.Status)
	assert.Equal(t, "2026-10-06", report.Date)
	assert.Empty(t, report.EnvPass)
	skillPath := filepath.Join(root, ".ai-rulez/skills/deploy/SKILL.md")
	data, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Equal(t, improveSkillBody, string(data), "the source is untouched until apply")
	assert.FileExists(t, filepath.Join(root, ".ai-rulez/local/improve", report.RunID, "diff.patch"))

	// Act: apply
	var applyOut bytes.Buffer
	improveApplyCmd.SetOut(&applyOut)
	improveApplyCmd.SetErr(&errOut)
	require.NoError(t, improveApplyCmd.RunE(improveApplyCmd, []string{report.RunID}))

	// Assert
	data, err = os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "GOOD advice.")
	assert.Contains(t, errOut.String(), "Nothing was committed", "the narrative goes to stderr in JSON mode")
	var applied improve.ApplyResult
	require.NoError(t, json.Unmarshal(applyOut.Bytes(), &applied), "stdout is the result document only")
	assert.Equal(t, "deploy", applied.Skill)
	assert.Contains(t, applied.Written, "SKILL.md")
	require.Error(t, improveApplyCmd.RunE(improveApplyCmd, []string{report.RunID}), "a second apply is refused")
}

func TestImprove_DryRunWritesNothingAndRunsNothing(t *testing.T) {
	// Arrange
	root := setupImproveRun(t)
	improveFlags.dryRun = true
	improveFlags.with = "/nonexistent/optimizer"
	improveFlags.format = formatText
	var out bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&bytes.Buffer{})

	// Act
	noCandidate, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.NoError(t, err)
	assert.False(t, noCandidate)
	assert.Contains(t, out.String(), "Dry run")
	assert.Contains(t, out.String(), "2 train case(s), 3 held-out case(s)")
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez/local"))
}

func TestImprove_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func()
		text   string
	}{
		{"no optimizer", func() { improveFlags.with = "" }, "optimizer command is empty"},
		{"no max cost", func() { improveFlags.maxCost = 0 }, "--max-cost"},
		{"bad format", func() { improveFlags.format = "xml" }, "--format"},
		{"credential without egress", func() { improveFlags.envPass = []string{"MY_API_KEY"} }, "--egress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			setupImproveRun(t)
			tt.mutate()
			improveRunCmd.SetOut(&bytes.Buffer{})
			improveRunCmd.SetErr(&bytes.Buffer{})

			// Act
			_, err := runImprove(improveRunCmd, "deploy")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.text)
		})
	}
}

func TestImprove_IsMarkedExperimentalInHelp(t *testing.T) {
	assert.True(t, strings.HasPrefix(ImproveCmd.Short, "(experimental)"))
	assert.True(t, strings.HasPrefix(improveRunCmd.Short, "(experimental)"))
	assert.True(t, strings.HasPrefix(improveApplyCmd.Short, "(experimental)"))
	assert.Contains(t, improveRunCmd.Long, "EXPERIMENTAL")
}
