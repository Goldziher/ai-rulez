package commands

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetEvalFlags(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // the eval-results key stays out of the real home
	prevDir := configDir
	reset := func() {
		evalFlags.harness, evalFlags.runner, evalFlags.runnerCommand = "claude", "", ""
		evalFlags.claudeBin, evalFlags.runnerArgs, evalFlags.runs = "claude", nil, 0
		evalFlags.codexBin, evalFlags.maxCostMode = "codex", ""
		evalFlags.ablation, evalFlags.dryRun, evalFlags.format = false, false, evals.FormatMarkdown
		evalFlags.out, evalFlags.maxCost, evalFlags.date = "", 0, ""
		evalFlags.changedOnly, evalFlags.base, evalFlags.force = false, "HEAD", false
		evalFlags.threshold, evalFlags.allowExec, evalFlags.noWrite = 1, false, false
		evalFlags.timeout = 30 * time.Minute
		evalFlags.estimate, evalFlags.mode, evalFlags.surface, evalFlags.scope = false, evals.ModeCases, "", evals.ScopeDomain
		evalFlags.descriptionFrom = ""
		if f := evalRunCmd.Flags().Lookup("threshold"); f != nil {
			f.Changed = false
		}
		evalFlags.results, evalFlags.priceIn, evalFlags.priceOut, evalFlags.model = "", 0, 0, ""
		configDir = prevDir
	}
	reset()
	t.Cleanup(reset)
}

const evalCLICases = `cases:
  - id: fires
    prompt: do it
    expect_trigger: true
    assertions:
      - {type: contains, value: ok}
  - id: quiet
    prompt: other
    expect_trigger: false
`

func evalProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml":                     "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n",
		".ai-rulez/skills/deploy/SKILL.md":          "---\nname: deploy\ndescription: Deploy things. Use when deploying.\n---\nbody\n",
		".ai-rulez/skills/deploy/evals/a.eval.yaml": evalCLICases,
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	t.Chdir(root)
	return root
}

func writeRunnerScript(t *testing.T, reply string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the runner is a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '"+reply+"'\n"), 0o700))
	return path
}

const goodReply = `{"version":1,"results":[{"case":"fires","arm":"with","triggered":true,"output":"ok","cost_usd":0.1},{"case":"quiet","arm":"with","triggered":false}]}`

func TestEvalRun_CommandRunnerRecordsResultsAndUsesCache(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	script := writeRunnerScript(t, goodReply)
	calls := filepath.Join(t.TempDir(), "calls")
	evalFlags.runnerCommand = script + " && echo x >> " + calls
	evalFlags.date = "2026-10-05"
	evalFlags.format = evals.FormatJSON

	cmd := evalRunCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	failed, err := runEval(cmd, nil)
	require.NoError(t, err)
	assert.False(t, failed)

	var report evals.RunReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.Len(t, report.Skills, 1)
	assert.Equal(t, evals.RunRan, report.Skills[0].Status)
	assert.Equal(t, "command", report.Runner)

	store, err := evals.LoadStore(filepath.Join(root, ".ai-rulez", evals.StoreFileName))
	require.NoError(t, err)
	record, ok := store.Get("deploy")
	require.True(t, ok)
	assert.Equal(t, "2026-10-05", record.Date)
	assert.True(t, record.Passing)

	// second run is served from the store: the runner is not started again
	out.Reset()
	failed, err = runEval(cmd, nil)
	require.NoError(t, err)
	assert.False(t, failed)
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, evals.RunCached, report.Skills[0].Status)
	logged, err := os.ReadFile(calls)
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(logged))
}

func TestEvalRun_FailingRunExitsNonZeroAndDryRunWritesNothing(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	evalFlags.runnerCommand = writeRunnerScript(t, `{"version":1,"results":[{"case":"fires","arm":"with","triggered":false},{"case":"quiet","arm":"with","triggered":false}]}`)
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	failed, err := runEval(evalRunCmd, []string{"deploy"})
	require.NoError(t, err)
	assert.True(t, failed)
	assert.Contains(t, out.String(), "deploy")

	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", evals.StoreFileName)))
	evalFlags.dryRun = true
	evalFlags.runnerCommand = "exit 9"
	out.Reset()
	failed, err = runEval(evalRunCmd, nil)
	require.NoError(t, err)
	assert.False(t, failed)
	assert.Contains(t, out.String(), "Dry run")
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}

func TestEvalRun_OutDirAndFormats(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	evalFlags.runnerCommand = writeRunnerScript(t, goodReply)
	evalFlags.format = evals.FormatJUnit
	evalFlags.out = filepath.Join(root, "reports")
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	_, err := runEval(evalRunCmd, nil)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "reports", "eval-report.xml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "<testsuites")
	assert.Contains(t, out.String(), "Wrote")

	evalFlags.format = "yaml"
	_, err = runEval(evalRunCmd, nil)
	assert.ErrorContains(t, err, "unknown --format")
}

func TestEvalRun_RunnerSelection(t *testing.T) {
	resetEvalFlags(t)
	evalProject(t)
	evalFlags.harness = "codex"
	_, err := runEval(evalRunCmd, nil)
	assert.ErrorContains(t, err, "no built-in runner")

	evalFlags.harness = "claude"
	evalFlags.runner = "nope"
	_, err = runEval(evalRunCmd, nil)
	assert.ErrorContains(t, err, "unknown runner")
	for _, name := range []string{evals.RunnerClaudePluginEval, evals.RunnerCommand, evals.RunnerClaudeNative, evals.RunnerCodexNative} {
		assert.ErrorContains(t, err, name, "the error names every runner")
	}

	evalFlags.runner = evals.RunnerClaudeNative
	_, err = runEval(evalRunCmd, nil)
	assert.ErrorContains(t, err, "measures activation")

	evalFlags.runner = evals.RunnerCommand
	_, err = runEval(evalRunCmd, nil)
	assert.ErrorContains(t, err, "--runner-command")

	// a dry run works without any runner
	evalFlags.runner, evalFlags.harness, evalFlags.dryRun = "", "codex", true
	_, err = runEval(evalRunCmd, nil)
	assert.NoError(t, err)
}

func TestEvalRun_MinPassRateFromConfigIsTheThreshold(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	cfg := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n[lint.evals]\nmin_pass_rate = 0.5\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte(cfg), 0o600))
	// one of two cases passes: 50%
	evalFlags.runnerCommand = writeRunnerScript(t, `{"version":1,"results":[{"case":"fires","arm":"with","triggered":true,"output":"ok"},{"case":"quiet","arm":"with","triggered":true}]}`)
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	failed, err := runEval(evalRunCmd, nil)
	require.NoError(t, err)
	assert.False(t, failed, out.String())

	require.NoError(t, evalRunCmd.Flags().Set("threshold", "0.9"))
	evalFlags.force = true
	failed, err = runEval(evalRunCmd, nil)
	require.NoError(t, err)
	assert.True(t, failed)
}

func TestEvalRun_RejectsBadFlagsBeforeAnyPaidWork(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	cases := map[string]func(){
		"format typo":       func() { evalFlags.format = "junitt" },
		"nan max cost":      func() { evalFlags.maxCost = math.NaN() },
		"negative max cost": func() { evalFlags.maxCost = -5 },
		"negative price":    func() { evalFlags.priceIn = -1 },
		"threshold above 1": func() { require.NoError(t, evalRunCmd.Flags().Set("threshold", "1.5")) },
		"negative runs":     func() { evalFlags.runs = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			resetEvalFlags(t)
			evalProject(t)
			evalFlags.runnerCommand = "touch " + marker
			mutate()
			var out bytes.Buffer
			evalRunCmd.SetOut(&out)
			_, err := runEval(evalRunCmd, nil)
			require.Error(t, err)
			assert.NoFileExists(t, marker, "the runner must not have been started")
			assert.Empty(t, out.String())
		})
	}
}

func TestEvalRun_ThresholdZeroRecordsWithoutGating(t *testing.T) {
	resetEvalFlags(t)
	evalProject(t)
	evalFlags.runnerCommand = writeRunnerScript(t, `{"version":1,"results":[{"case":"fires","arm":"with","triggered":false},{"case":"quiet","arm":"with","triggered":false}]}`)
	require.NoError(t, evalRunCmd.Flags().Set("threshold", "0"))
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	failed, err := runEval(evalRunCmd, nil)
	require.NoError(t, err)
	assert.False(t, failed, out.String())
}

func TestEvalRun_SavesResultsOfFinishedSkillsWhenALaterStepFails(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	// a second skill whose runner call fails after the first was stored
	second := filepath.Join(root, ".ai-rulez", "skills", "zeta", "evals")
	require.NoError(t, os.MkdirAll(second, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(second, "..", "SKILL.md"), []byte("---\nname: zeta\n---\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(second, "a.eval.yaml"), []byte(evalCLICases), 0o600))
	// the output directory cannot be created, so the report step fails after the run
	blocker := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	evalFlags.out = filepath.Join(blocker, "sub")
	evalFlags.runnerCommand = writeRunnerScript(t, goodReply)
	_, err := runEval(evalRunCmd, nil)
	require.Error(t, err)
	store, loadErr := evals.LoadStore(filepath.Join(root, ".ai-rulez", evals.StoreFileName))
	require.NoError(t, loadErr)
	_, ok := store.Get("deploy")
	assert.True(t, ok, "results are saved before the report is written")
}

func TestEvalRun_ClaudeRunnerGetsTheEffectiveRunsAndTimeout(t *testing.T) {
	tests := []struct {
		name         string
		flagRuns     int
		wantRuns     int
		wantEstimate int
	}{
		{"default is the estimate's assumed 3, passed explicitly", 0, 3, 3},
		{"explicit value is passed through", 5, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEvalFlags(t)
			evalFlags.runs = tt.flagRuns
			evalFlags.timeout = 7 * time.Minute

			runner, estimateRuns, err := buildEvalRunner(evalRunCmd)

			require.NoError(t, err)
			claude, ok := runner.(*evals.ClaudePluginEval)
			require.True(t, ok)
			assert.Equal(t, tt.wantRuns, claude.Runs)
			assert.Equal(t, tt.wantEstimate, estimateRuns)
			assert.Equal(t, 7*time.Minute, claude.Timeout)
		})
	}
}

const activationCLISkills = `cases:
  - id: fires
    prompt: Deploy the billing service to staging
    expect_trigger: true
  - id: quiet
    prompt: bake a sourdough loaf
    expect_trigger: false
`

func activationProject(t *testing.T) string {
	t.Helper()
	root := evalProject(t)
	files := map[string]string{
		".ai-rulez/skills/deploy/SKILL.md":          "---\nname: deploy\ndescription: Deploy a service to the staging environment\nkeywords: [staging]\n---\nbody\n",
		".ai-rulez/skills/deploy/evals/a.eval.yaml": activationCLISkills,
		".ai-rulez/skills/changelog/SKILL.md":       "---\nname: changelog\ndescription: Write release notes\n---\nbody\n",
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez", "skills", "changelog"), 0o750))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o600))
	}
	return root
}

func TestEvalRun_ActivationRetrievalReportsAndRecords(t *testing.T) {
	resetEvalFlags(t)
	root := activationProject(t)
	evalFlags.mode, evalFlags.surface = evals.ModeActivation, evals.SurfaceRetrieval
	evalFlags.format, evalFlags.date = evals.FormatJSON, "2026-10-06"
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	failed, err := runEval(evalRunCmd, []string{"deploy"})

	require.NoError(t, err)
	assert.False(t, failed)
	var report evals.ActivationReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, evals.SurfaceRetrieval, report.Surface)
	require.Len(t, report.Skills, 1)
	require.NotNil(t, report.Skills[0].Recall)
	assert.Equal(t, 1.0, report.Skills[0].Recall.Value)
	store, err := evals.LoadStore(filepath.Join(root, ".ai-rulez", evals.StoreFileName))
	require.NoError(t, err)
	record, ok := store.Get("deploy")
	require.True(t, ok)
	require.NotNil(t, record.Activation)
	assert.Equal(t, "2026-10-06", record.Activation.Date)
}

func TestEvalRun_ActivationDryRunWritesNothingAndNoRunnerIsNeeded(t *testing.T) {
	resetEvalFlags(t)
	root := activationProject(t)
	evalFlags.mode, evalFlags.surface, evalFlags.estimate = evals.ModeActivation, evals.SurfaceRetrieval, true
	evalFlags.runnerCommand = "exit 9" // never started: retrieval calls no runner
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	_, err := runEval(evalRunCmd, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "# Skill activation results")
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}

func TestEvalRun_ActivationFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		wantErr string
	}{
		{"unknown mode", func() { evalFlags.mode = "vibes" }, "unknown --mode"},
		{"surface without activation", func() { evalFlags.surface = evals.SurfaceRetrieval }, "--surface needs --mode activation"},
		{"scope without activation", func() { evalFlags.scope = evals.ScopeAll }, "--scope needs --mode activation"},
		{"activation needs a surface", func() { evalFlags.mode = evals.ModeActivation }, "needs --surface"},
		{"unknown surface", func() { evalFlags.mode, evalFlags.surface = evals.ModeActivation, "tarot" }, "unknown --surface"},
		{"unknown scope", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.scope = evals.ModeActivation, evals.SurfaceRetrieval, "galaxy"
		}, "unknown --scope"},
		{"junit has no activation form", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.format = evals.ModeActivation, evals.SurfaceRetrieval, evals.FormatJUnit
		}, "not junit"},
		{"a command runner that answers the probe without the capability is refused", func() {
			evalFlags.mode, evalFlags.surface = evals.ModeActivation, evals.SurfaceNative
			evalFlags.runnerCommand = `printf '{"version":1}'`
		}, "does not support activation mode"},
		{"a command runner that does not answer the probe is refused", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.runnerCommand = evals.ModeActivation, evals.SurfaceNative, "true"
		}, "capabilities handshake failed"},
		{"the plugin-eval runner does not declare the capability", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.runner = evals.ModeActivation, evals.SurfaceNative, evals.RunnerClaudePluginEval
		}, "does not support activation mode"},
		{"a surface the runner does not declare is refused", func() {
			evalFlags.mode, evalFlags.surface = evals.ModeActivation, evals.SurfaceNative
			evalFlags.runnerCommand = `printf '{"version":1,"capabilities":["activation"],"surfaces":["other"]}'`
		}, "does not support the native surface"},
		{"description-from without activation", func() { evalFlags.descriptionFrom = "d.txt" }, "--description-from needs --mode activation"},
		{"description-from needs exactly one skill", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.descriptionFrom = evals.ModeActivation, evals.SurfaceRetrieval, "d.txt"
		}, "exactly one skill"},
		{"unknown max-cost-mode", func() { evalFlags.maxCostMode = "wild" }, "unknown --max-cost-mode"},
		{"an unknown runner", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.runner = evals.ModeActivation, evals.SurfaceNative, "magic"
		}, "unknown runner"},
		{"no built-in runner for a harness", func() {
			evalFlags.mode, evalFlags.surface, evalFlags.harness = evals.ModeActivation, evals.SurfaceNative, "gemini"
		}, "no built-in native activation runner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEvalFlags(t)
			activationProject(t)
			tt.set()

			_, err := runEval(evalRunCmd, nil)

			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// fakeClaude is a claude executable that answers one stream-json run: it loads the
// skill when the prompt (stdin) mentions staging.
func fakeClaude(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "claude")
	body := `#!/bin/sh
prompt=$(cat)
case "$prompt" in
*staging*) echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"ai-rulez-activation:deploy"}}]}}' ;;
esac
echo '{"type":"result","subtype":"error_max_turns","is_error":true,"total_cost_usd":0.01,"usage":{"input_tokens":100,"output_tokens":10}}'
exit 1
`
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))
	return script
}

func TestEvalRun_ActivationNativeThroughTheClaudeAdapter(t *testing.T) {
	resetEvalFlags(t)
	root := activationProject(t)
	evalFlags.mode, evalFlags.surface, evalFlags.runs = evals.ModeActivation, evals.SurfaceNative, 3
	evalFlags.claudeBin, evalFlags.format, evalFlags.date = fakeClaude(t), evals.FormatJSON, "2026-10-06"
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	failed, err := runEval(evalRunCmd, []string{"deploy"})

	require.NoError(t, err)
	assert.False(t, failed)
	var report evals.ActivationReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, evals.SurfaceNative, report.Surface)
	assert.Equal(t, evals.RunnerClaudeNative, report.Runner)
	assert.Equal(t, 3, report.Runs)
	assert.InDelta(t, 0.06, report.Cost.ActualUSD, 1e-9, "two prompts, three runs, $0.01 each")
	require.Len(t, report.Skills, 1)
	assert.True(t, report.Skills[0].Passing)
	assert.Equal(t, map[string]int{"deploy": 3}, report.Skills[0].Prompts[0].FiredCounts)
	store, err := evals.LoadStore(filepath.Join(root, ".ai-rulez", evals.StoreFileName))
	require.NoError(t, err)
	record, ok := store.Get("deploy")
	require.True(t, ok)
	require.NotNil(t, record.Activation)
	assert.Equal(t, evals.SurfaceNative, record.Activation.Surface)
}

func TestEvalRun_ActivationNativeDryRunNeedsNoRunnerAndCallsNone(t *testing.T) {
	resetEvalFlags(t)
	root := activationProject(t)
	evalFlags.mode, evalFlags.surface, evalFlags.estimate = evals.ModeActivation, evals.SurfaceNative, true
	evalFlags.claudeBin = filepath.Join(t.TempDir(), "does-not-exist")
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	failed, err := runEval(evalRunCmd, nil)

	require.NoError(t, err)
	assert.False(t, failed)
	assert.Contains(t, out.String(), "dry run: ")
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}

func TestEvalRun_ActivationNativeMaxCostRefusesBeforeRunning(t *testing.T) {
	resetEvalFlags(t)
	activationProject(t)
	evalFlags.mode, evalFlags.surface, evalFlags.maxCost = evals.ModeActivation, evals.SurfaceNative, 0.0001
	evalFlags.claudeBin = filepath.Join(t.TempDir(), "never-started")

	_, err := runEval(evalRunCmd, []string{"deploy"})

	assert.ErrorContains(t, err, "exceeds --max-cost")
}

func TestEvalRun_EstimateIsAnAliasOfDryRun(t *testing.T) {
	resetEvalFlags(t)
	root := evalProject(t)
	evalFlags.estimate = true
	evalFlags.runnerCommand = "exit 9"
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	failed, err := runEval(evalRunCmd, nil)

	require.NoError(t, err)
	assert.False(t, failed)
	assert.Contains(t, out.String(), "Dry run")
	assert.Contains(t, out.String(), "range $")
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName))
}

func TestEvalRun_DescriptionFromMeasuresACandidateAndRecordsNothing(t *testing.T) {
	// Arrange
	resetEvalFlags(t)
	root := activationProject(t)
	file := filepath.Join(t.TempDir(), "candidate.txt")
	require.NoError(t, os.WriteFile(file, []byte("Deploy a service to the staging environment\n"), 0o600))
	evalFlags.mode, evalFlags.surface, evalFlags.descriptionFrom = evals.ModeActivation, evals.SurfaceRetrieval, file
	evalFlags.format, evalFlags.date = evals.FormatJSON, "2026-10-06"
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)
	source, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"))
	require.NoError(t, err)

	// Act
	_, err = runEval(evalRunCmd, []string{"deploy"})

	// Assert
	require.NoError(t, err)
	var report evals.ActivationReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.Len(t, report.Skills, 1)
	assert.Contains(t, report.Skills[0].Warnings[0], "description replaced")
	after, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, string(source), string(after))
	store, err := evals.LoadStore(filepath.Join(root, ".ai-rulez", evals.StoreFileName))
	require.NoError(t, err)
	_, recorded := store.Get("deploy")
	assert.False(t, recorded)
}

func TestReadDescriptionFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
		want    string
		wantErr string
	}{
		{name: "trimmed text", content: "  Use when deploying.\n", want: "Use when deploying."},
		{name: "empty", content: " \n", wantErr: "is empty"},
		{name: "hidden characters", content: "Use when\u200b deploying", wantErr: "hidden"},
		{name: "not utf-8", content: "bad \xff bytes", wantErr: "UTF-8"},
		{name: "too large", content: strings.Repeat("a", 20<<10), wantErr: "limit"},
		{name: "missing", missing: true, wantErr: "read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "d.txt")
			if !tt.missing {
				require.NoError(t, os.WriteFile(file, []byte(tt.content), 0o600))
			}

			got, err := readDescriptionFile(file)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
