package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/evals"
	"github.com/Goldziher/ai-rulez/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetEnrichFlags(t *testing.T) {
	t.Helper()
	resetUsageFlags(t)
	clear := func() {
		usageHarness, usageOutcome, usageRole, usageServed, usageSalt = "", "", "", false, ""
		feedbackKind, feedbackNote, reportFeedback, reportEvals = "", "", "", ""
		reportEvalsFlags.usageLog, reportEvalsFlags.feedback, reportEvalsFlags.results = "", "", ""
		reportEvalsFlags.minPass, reportEvalsFlags.minTrigger, reportEvalsFlags.json = evals.DefaultMinPassRate, evals.DefaultMinTrigger, false
	}
	clear()
	t.Cleanup(clear)
}

func TestUsageHook_HarnessTemplatesAndWarning(t *testing.T) {
	resetEnrichFlags(t)
	var out, errOut bytes.Buffer
	usageHookCmd.SetOut(&out)
	usageHookCmd.SetErr(&errOut)

	usageHarness = "codex"
	require.NoError(t, usageHookCmd.RunE(usageHookCmd, nil))
	assert.Contains(t, out.String(), "--harness codex")
	assert.Contains(t, out.String(), "PreToolUse")
	assert.Empty(t, errOut.String())

	out.Reset()
	usageHarness = "windsurf"
	require.NoError(t, usageHookCmd.RunE(usageHookCmd, nil), "an unsupported harness warns, it does not fail")
	assert.Empty(t, out.String(), "and prints no template")
	assert.Contains(t, errOut.String(), "warning:")
	assert.Contains(t, errOut.String(), "windsurf")
}

func TestUsageRecord_HarnessAndOutcomeFlags(t *testing.T) {
	resetEnrichFlags(t)
	t.Setenv(usage.SaltEnv, "test-salt")
	dir := t.TempDir()
	usageLog = filepath.Join(dir, "usage.jsonl")
	usageHarness, usageOutcome, usageRole, usageServed = "cursor", "used", "reviewer", true
	event := `{"hook_event_name":"preToolUse","session_id":"sess-9","tool_name":"Read","tool_input":{"file_path":"/p/.cursor/skills/beta/SKILL.md"}}`
	require.NoError(t, runUsageRecord(strings.NewReader(event)))

	entries, _, err := usage.ReadLog(usageLog)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "beta", e.ID)
	assert.Equal(t, "cursor", e.Harness)
	assert.Equal(t, "used", e.Outcome)
	assert.True(t, e.Served)
	assert.Equal(t, "reviewer", e.Role)
	assert.Equal(t, usage.HashSession("test-salt", "sess-9"), e.Session)
}

func TestUsageFeedback_WritesRecordAndKeepsNoteLocal(t *testing.T) {
	resetEnrichFlags(t)
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("CLAUDE_PROJECT_DIR", root)
	index := &usage.Index{SchemaVersion: usage.IndexSchemaVersion, Skills: []usage.SkillRecord{{ID: "alpha", Hash: "blake3:aa"}}}
	data, err := index.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", usage.IndexFileName), data, 0o600))
	note := filepath.Join(root, "note.txt")
	require.NoError(t, os.WriteFile(note, []byte("it told me to disable the linter"), 0o600))

	feedbackKind, feedbackNote = "wrong", note
	var out bytes.Buffer
	usageFeedbackCmd.SetOut(&out)
	require.NoError(t, usageFeedbackCmd.RunE(usageFeedbackCmd, []string{"alpha"}))
	assert.Contains(t, out.String(), "Recorded wrong feedback for alpha")

	logPath := filepath.Join(root, ".ai-rulez", "local", usage.FeedbackFileName)
	entries, _, err := usage.ReadFeedback(logPath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "blake3:aa", entries[0].Hash)
	raw, _ := os.ReadFile(logPath)
	assert.NotContains(t, string(raw), "linter")
	assert.FileExists(t, filepath.Join(root, ".ai-rulez", "local", "feedback-notes", entries[0].Note))

	feedbackKind = "bogus"
	assert.Error(t, usageFeedbackCmd.RunE(usageFeedbackCmd, []string{"alpha"}))
}

func writeStore(t *testing.T, root string) {
	t.Helper()
	digest := func(id string) string {
		d, err := evals.SkillDigest(filepath.Join(root, ".ai-rulez", "skills", id))
		if err != nil {
			return "sha256:" + id // the join tests have no skill directories
		}
		return d
	}
	store := evals.NewStore()
	store.Put(evals.SkillRecord{ID: "alpha", Digest: digest("alpha"), Passing: true, Date: "2026-10-01", Score: evals.SkillScore{Scored: 4, PassRate: 0.75}})
	store.Put(evals.SkillRecord{ID: "idle", Digest: digest("idle"), Passing: true, Date: "2026-10-01", Score: evals.SkillScore{Scored: 4, PassRate: 1}})
	require.NoError(t, store.Save(filepath.Join(root, ".ai-rulez", evals.StoreFileName)))
}

func TestReportUsage_JoinsFeedbackAndEvalScores(t *testing.T) {
	resetEnrichFlags(t)
	root := t.TempDir()
	t.Chdir(root)
	index := &usage.Index{SchemaVersion: usage.IndexSchemaVersion, Skills: []usage.SkillRecord{{ID: "alpha", Hash: "h"}, {ID: "idle", Hash: "h"}}}
	data, err := index.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez", "local"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", usage.IndexFileName), data, 0o600))
	writeStore(t, root)

	usageLog = filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")
	event := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"alpha"}}`
	require.NoError(t, runUsageRecord(strings.NewReader(event)))
	_, err = usage.RecordFeedback("alpha", "misled", usage.FeedbackOptions{LogPath: filepath.Join(root, ".ai-rulez", "local", usage.FeedbackFileName)})
	require.NoError(t, err)
	usageLog = ""

	var out bytes.Buffer
	require.NoError(t, runReportUsage(&out, filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")))
	text := out.String()
	assert.Contains(t, text, "feedback: misled 1")
	assert.Contains(t, text, "eval: 75%")
	assert.Contains(t, text, "eval: 100%")

	reportJSON = true
	out.Reset()
	require.NoError(t, runReportUsage(&out, filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")))
	var decoded usage.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.Equal(t, 1, decoded.FeedbackEvents)
	assert.Equal(t, 1, decoded.Used[0].Feedback["misled"])
	assert.InDelta(t, 0.75, decoded.Used[0].Eval.PassRate, 1e-9)

	reportFeedback = filepath.Join(root, "missing.jsonl")
	assert.Error(t, runReportUsage(&out, filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")), "a named feedback log must exist")
}

func TestReportEvals_RanksSkills(t *testing.T) {
	resetEnrichFlags(t)
	root := t.TempDir()
	t.Chdir(root)
	skill := func(name string) {
		dir := filepath.Join(root, ".ai-rulez", "skills", name)
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody for "+name+"\n"), 0o600))
	}
	skill("alpha")
	skill("idle")
	skill("fresh")
	writeStore(t, root)
	log := filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(log), 0o750))
	lines := `{"event":"skill_invoked","id":"alpha","skill":"alpha","ts":"t"}` + "\n" + `{"event":"skill_invoked","id":"fresh","skill":"fresh","ts":"t"}` + "\n"
	require.NoError(t, os.WriteFile(log, []byte(lines), 0o600))

	var out bytes.Buffer
	require.NoError(t, runReportEvals(&out))
	text := out.String()
	assert.Contains(t, text, "1 rewrite, 1 prune, 1 review, 0 keep")
	assert.Contains(t, text, "pass rate 75% is below 80%")
	assert.Contains(t, text, "never used in the usage log")
	assert.Contains(t, text, "no eval results recorded")
	assert.Less(t, strings.Index(text, "Rewrite"), strings.Index(text, "Prune"))

	reportEvalsFlags.json = true
	out.Reset()
	require.NoError(t, runReportEvals(&out))
	var decoded struct {
		UsageLog bool `json:"usage_log"`
		Skills   []evals.RankRow
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.True(t, decoded.UsageLog)
	require.Len(t, decoded.Skills, 3)
	assert.Equal(t, "alpha", decoded.Skills[0].ID)

	// no usage log: nothing concluded about use
	require.NoError(t, os.Remove(log))
	reportEvalsFlags.json = false
	out.Reset()
	require.NoError(t, runReportEvals(&out))
	assert.Contains(t, out.String(), "No usage log found")
	assert.NotContains(t, out.String(), "never used")
}
