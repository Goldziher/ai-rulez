package evals

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeClaude(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude is a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o700)) //nolint:gosec // test script
	return script
}

func TestClaudePluginEval_TimeoutKillsTheProcessTree(t *testing.T) {
	// Arrange: a "claude" that forks a long sleeper and waits on it
	pidFile := filepath.Join(t.TempDir(), "pid")
	bin := fakeClaude(t, "sleep 60 & echo $! > "+pidFile+"\nwait")
	runner := &ClaudePluginEval{Bin: bin, Timeout: 500 * time.Millisecond}

	// Act
	start := time.Now()
	_, err := runner.Run(context.Background(), sampleRequest(t))

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "timed out")
	assert.Less(t, time.Since(start), 15*time.Second)
	assert.True(t, processGone(t, pidFile), "the whole tree must die with the timeout")
}

func TestClaudePluginEval_OutputIsCapped(t *testing.T) {
	// ~24 MiB on stdout and stderr; the run must finish and keep memory bounded.
	bin := fakeClaude(t, "head -c 25165824 /dev/zero | tr '\\0' x\nhead -c 25165824 /dev/zero | tr '\\0' y >&2\nexit 1")
	var forwarded countingWriter
	runner := &ClaudePluginEval{Bin: bin, Stderr: &forwarded}
	_, err := runner.Run(context.Background(), sampleRequest(t))
	require.Error(t, err)
	assert.LessOrEqual(t, forwarded.n, maxToolOutputBytes, "stderr forwarding is capped")
}

type countingWriter struct{ n int }

func (c *countingWriter) Write(p []byte) (int, error) { c.n += len(p); return len(p), nil }

func TestFingerprint_CoversTheRunnerProgramContent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script")
	}
	script := filepath.Join(t.TempDir(), "runner.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho one\n"), 0o700)) //nolint:gosec // test script
	runner := &CommandRunner{Command: script + " --flag"}
	before := runner.Fingerprint()
	assert.Contains(t, before, "sha256:")

	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho two\n"), 0o700)) //nolint:gosec // test script
	assert.NotEqual(t, before, runner.Fingerprint(), "editing the script the command runs must change the cache key")

	assert.NotContains(t, (&CommandRunner{Command: "no-such-program-xyz --a"}).Fingerprint(), "sha256:")
}

func TestRun_ForgedResultsFileDoesNotSkipAnEditedSkill(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: a\n---\nbody\n", twoCases)
	runner := goodRunner()
	opts := baseOptions(cfg, runner)
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 1, runner.calls)

	// A record whose key matches but whose recorded digests are not the skill on disk.
	rec, ok := opts.Store.Get("alpha")
	require.True(t, ok)
	rec.Digest = "sha256:forged"
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, runner.calls, "a record that does not describe the current skill is never replayed")
}

func TestRun_UnreportedCostIsChargedAtTheBudget(t *testing.T) {
	cfg := projectWithSkills(t)
	silent := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: "ok"})
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, silent)
	opts.Ablation = false
	opts.MaxCostUSD = 10
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, 1, silent.calls, "no reported cost under --max-cost: the whole budget is assumed spent")
	assert.Equal(t, RunOverBudget, report.Skills[1].Status)
	require.NotEmpty(t, report.Skills[0].Warnings)
	assert.Contains(t, report.Skills[0].Warnings[0], "reported no cost")

	// without a cap nothing changes
	silent.calls = 0
	opts = baseOptions(projectWithSkills(t), silent)
	opts.Ablation = false
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, silent.calls)
}

func TestRun_CostAboveTheGivenBudgetWarns(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "x", twoCases)
	spender := &fakeRunner{fn: func(req *Request) (*Response, error) {
		return &Response{Version: ProtocolVersion, Results: []Result{{Case: "fires", Arm: ArmWith, Triggered: bp(true), Output: "ok", CostUSD: 11}}}, nil
	}}
	opts := baseOptions(cfg, spender)
	opts.MaxCostUSD = 10
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, report.Skills[0].Warnings, 1)
	assert.Contains(t, report.Skills[0].Warnings[0], "exceeds the $10.00 budget")

	var out strings.Builder
	require.NoError(t, report.Write(&out, FormatMarkdown))
	assert.Contains(t, out.String(), "warning: cost $11.00 exceeds")
}
