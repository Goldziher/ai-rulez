package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const claudeSkillEvent = `{"hook_event_name":"PreToolUse","session_id":"raw-session-id-123","tool_name":"Skill","tool_input":{"skill":"alpha"}}`

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		out = append(out, m)
	}
	return out
}

func TestRecord_SessionIsSaltedHashNeverRaw(t *testing.T) {
	t.Setenv(SaltEnv, "")
	dir := t.TempDir()
	log := filepath.Join(dir, "usage.jsonl")
	for range 2 {
		_, err := Record(strings.NewReader(claudeSkillEvent), RecordOptions{LogPath: log, Now: fixedClock})
		require.NoError(t, err)
	}
	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "raw-session-id-123")

	lines := readLines(t, log)
	session, _ := lines[0]["session"].(string)
	assert.Len(t, session, 16)
	assert.Equal(t, session, lines[1]["session"], "one session hashes to one value")
	assert.EqualValues(t, EntrySchemaVersion, lines[0]["v"])

	saltFile := filepath.Join(dir, "usage.salt")
	info, err := os.Stat(saltFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	salt, _ := os.ReadFile(saltFile)
	assert.Equal(t, HashSession(strings.TrimSpace(string(salt)), "raw-session-id-123"), session)

	// another machine (another salt) produces a different hash for the same session
	assert.NotEqual(t, session, HashSession("other-salt", "raw-session-id-123"))
}

func TestRecord_SaltFromEnvAndUnwritableSaltOmitsSession(t *testing.T) {
	t.Setenv(SaltEnv, "env-salt")
	log := filepath.Join(t.TempDir(), "u.jsonl")
	entry, err := Record(strings.NewReader(claudeSkillEvent), RecordOptions{LogPath: log, Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, HashSession("env-salt", "raw-session-id-123"), entry.Session)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(log), "usage.salt"))

	t.Setenv(SaltEnv, "")
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	entry, err = Record(strings.NewReader(claudeSkillEvent), RecordOptions{
		LogPath: filepath.Join(t.TempDir(), "u.jsonl"), SaltPath: filepath.Join(blocker, "sub", "usage.salt"), Now: fixedClock,
	})
	require.NoError(t, err)
	assert.Empty(t, entry.Session, "without a salt the session is omitted, never written raw")
}

func TestRecord_OutcomeServedRoleAndHarness(t *testing.T) {
	t.Setenv(SaltEnv, "s")
	log := filepath.Join(t.TempDir(), "u.jsonl")
	entry, err := Record(strings.NewReader(claudeSkillEvent), RecordOptions{LogPath: log, Outcome: OutcomeUsed, Served: true, Role: " reviewer ", Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, OutcomeUsed, entry.Outcome)
	assert.True(t, entry.Served)
	assert.Equal(t, "reviewer", entry.Role)

	entry, err = Record(strings.NewReader(claudeSkillEvent), RecordOptions{LogPath: log, Outcome: "exploded", Now: fixedClock})
	require.NoError(t, err)
	assert.Empty(t, entry.Outcome, "an unknown outcome is omitted")

	entry, err = Record(strings.NewReader(claudeSkillEvent), RecordOptions{LogPath: log, Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, OutcomeLoaded, entry.Outcome)
	lines := readLines(t, log)
	assert.NotContains(t, lines[2], "served", "false flags are omitted")
	assert.NotContains(t, lines[2], "role")
}

func TestRecord_CodexAndCursorSkillReads(t *testing.T) {
	t.Setenv(SaltEnv, "s")
	tests := []struct {
		name, harness, event, want string
	}{
		{"codex cat", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"cat .agents/skills/deploy-staging/SKILL.md"}}`, "deploy-staging"},
		{"codex absolute", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"sed -n 1,40p '/home/u/proj/.agents/skills/alpha/SKILL.md'"}}`, "alpha"},
		{"cursor file read", HarnessCursor, `{"hook_event_name":"preToolUse","tool_name":"Read","tool_input":{"file_path":"/p/.cursor/skills/beta/SKILL.md"}}`, "beta"},
		{"codex unrelated command", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls -la"}}`, ""},
		{"codex other file in skill", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"cat skills/alpha/references/notes.md"}}`, ""},
		{"git add is not a load", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git add skills/deploy/SKILL.md"}}`, ""},
		{"rm is not a load", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm .agents/skills/deploy/SKILL.md"}}`, ""},
		{"sed -i is an edit", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"sed -i 's/a/b/' skills/deploy/SKILL.md"}}`, ""},
		{"write tool is not a load", HarnessCursor, `{"hook_event_name":"preToolUse","tool_name":"Write","tool_input":{"file_path":"/p/.cursor/skills/beta/SKILL.md"}}`, ""},
		{"edit tool is not a load", HarnessCursor, `{"hook_event_name":"preToolUse","tool_name":"StrReplace","tool_input":{"path":"/p/.cursor/skills/beta/SKILL.md"}}`, ""},
		{"cd then cat is a load", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"cd /p && cat skills/gamma/SKILL.md | head -5"}}`, "gamma"},
		{"env prefix then cat", HarnessCodex, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"LC_ALL=C cat skills/delta/SKILL.md"}}`, "delta"},
		{"codex other event", HarnessCodex, `{"hook_event_name":"Stop","tool_input":{"command":"cat skills/alpha/SKILL.md"}}`, ""},
		{"claude payload under codex", HarnessCodex, claudeSkillEvent, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "u.jsonl")
			entry, err := Record(strings.NewReader(tt.event), RecordOptions{LogPath: log, Harness: tt.harness, Now: fixedClock})
			require.NoError(t, err)
			if tt.want == "" {
				assert.Nil(t, entry)
				return
			}
			assert.Equal(t, tt.want, entry.ID)
			assert.Equal(t, tt.harness, entry.Harness)
			assert.Equal(t, "read", entry.Invocation)
			data, _ := os.ReadFile(log)
			assert.NotContains(t, string(data), "sed -n", "command text never reaches the log")
			assert.NotContains(t, string(data), "/home/u")
		})
	}
}

func TestReadLog_OldLinesStayReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.jsonl")
	old := `{"ts":"2026-01-01T00:00:00Z","event":"skill_invoked","skill":"alpha","id":"alpha","hash":"h","session":"raw-old-id","invocation":"tool","harness":"claude"}`
	future := `{"v":9,"ts":"2026-01-02T00:00:00Z","event":"skill_invoked","skill":"alpha","id":"alpha","invocation":"tool","harness":"claude","outcome":"used","served":true,"role":"r","brand_new_field":1}`
	require.NoError(t, os.WriteFile(path, []byte(old+"\n"+future+"\n"), 0o600))
	entries, skipped, err := ReadLog(path)
	require.NoError(t, err)
	assert.Zero(t, skipped)
	require.Len(t, entries, 2)
	assert.Zero(t, entries[0].Version)
	assert.Equal(t, "raw-old-id", entries[0].Session)
	assert.Equal(t, OutcomeUsed, entries[1].Outcome)
	assert.True(t, entries[1].Served)

	report := BuildReport(&Index{SchemaVersion: 1, Skills: []SkillRecord{{ID: "alpha", Hash: "h"}}}, entries, skipped)
	assert.Equal(t, 2, report.Used[0].Count)
}

func TestHookTemplate_CodexCursorAndUnsupported(t *testing.T) {
	codex, err := HookTemplate(HookTemplateOptions{Harness: HarnessCodex, Role: "ops lead"})
	require.NoError(t, err)
	var decoded map[string]map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(codex, &decoded))
	entry := decoded["hooks"]["PreToolUse"][0]
	assert.Equal(t, "Bash", entry.Matcher)
	assert.Equal(t, `ai-rulez usage record --harness codex --role 'ops lead'`, entry.Hooks[0].Command)
	assert.Len(t, decoded["hooks"], 1, "only the documented PreToolUse event")

	cursor, err := HookTemplate(HookTemplateOptions{Harness: HarnessCursor, LogPath: "/tmp/u.jsonl"})
	require.NoError(t, err)
	var flat struct {
		Version int `json:"version"`
		Hooks   map[string][]struct {
			Command string `json:"command"`
			Matcher string `json:"matcher"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(cursor, &flat))
	assert.Equal(t, 1, flat.Version)
	assert.Equal(t, "Shell", flat.Hooks["preToolUse"][0].Matcher)
	assert.Contains(t, flat.Hooks["preToolUse"][0].Command, "--harness cursor")
	assert.Contains(t, flat.Hooks["preToolUse"][0].Command, `--log "/tmp/u.jsonl"`)

	assert.Equal(t, ".codex/hooks.json", HookFile(HarnessCodex))
	assert.Equal(t, ".cursor/hooks.json", HookFile(HarnessCursor))
	assert.Equal(t, ".claude/settings.json", HookFile(""))

	_, err = HookTemplate(HookTemplateOptions{Harness: "gemini"})
	var unsupported *UnsupportedHarnessError
	require.ErrorAs(t, err, &unsupported)
	assert.Contains(t, err.Error(), "gemini")

	claude, err := HookTemplate(HookTemplateOptions{Harness: HarnessClaude})
	require.NoError(t, err)
	assert.NotContains(t, string(claude), "--harness", "the Claude template is unchanged")
}

func TestFeedback_RecordsIdentifiersAndKeepsNotesLocal(t *testing.T) {
	dir := t.TempDir()
	indexPath := writeIndex(t, dir, SkillRecord{ID: "alpha", Hash: "blake3:aa"})
	log := filepath.Join(dir, ".ai-rulez", "local", FeedbackFileName)
	noteFile := filepath.Join(dir, "my-note.txt")
	secret := "the deploy skill told me to use prod creds: hunter2"
	require.NoError(t, os.WriteFile(noteFile, []byte(secret), 0o600))

	entry, err := RecordFeedback("plugin:alpha", FeedbackMisled, FeedbackOptions{
		LogPath: log, IndexPath: indexPath, NoteFile: noteFile, Harness: "claude", Role: "dev", Now: fixedClock,
	})
	require.NoError(t, err)
	assert.Equal(t, "alpha", entry.ID)
	assert.Equal(t, "plugin:alpha", entry.Skill)
	assert.Equal(t, "blake3:aa", entry.Hash)
	assert.Equal(t, "20261004T120000Z-alpha-misled.txt", entry.Note)

	line, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.NotContains(t, string(line), "hunter2", "note text never enters the feedback log")
	assert.NotContains(t, string(line), "prod creds")

	notePath := filepath.Join(filepath.Dir(log), "feedback-notes", entry.Note)
	kept, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, secret, string(kept))
	info, err := os.Stat(notePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// a second note in the same second does not overwrite the first
	second, err := RecordFeedback("alpha", FeedbackStale, FeedbackOptions{LogPath: log, NoteFile: noteFile, Now: fixedClock})
	require.NoError(t, err)
	assert.NotEqual(t, entry.Note, second.Note)
	third, err := RecordFeedback("alpha", FeedbackStale, FeedbackOptions{LogPath: log, NoteFile: noteFile, Now: fixedClock})
	require.NoError(t, err)
	assert.NotEqual(t, second.Note, third.Note)
}

func TestFeedback_NoteNeverReachesSharedArtifacts(t *testing.T) {
	dir := t.TempDir()
	index := &Index{SchemaVersion: IndexSchemaVersion, Skills: []SkillRecord{{ID: "alpha", Hash: "blake3:aa"}}}
	before, err := index.Marshal()
	require.NoError(t, err)
	noteFile := filepath.Join(dir, "n.txt")
	require.NoError(t, os.WriteFile(noteFile, []byte("private words"), 0o600))
	_, err = RecordFeedback("alpha", FeedbackGreat, FeedbackOptions{LogPath: filepath.Join(dir, "f.jsonl"), NoteFile: noteFile, Now: fixedClock})
	require.NoError(t, err)
	after, err := index.Marshal()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	for _, name := range []string{"f.jsonl"} {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		assert.NotContains(t, string(data), "private words")
	}
}

func TestFeedback_Validation(t *testing.T) {
	log := filepath.Join(t.TempDir(), "f.jsonl")
	_, err := RecordFeedback("alpha", "meh", FeedbackOptions{LogPath: log})
	assert.ErrorContains(t, err, "unknown feedback kind")
	_, err = RecordFeedback("../etc/passwd", FeedbackGreat, FeedbackOptions{LogPath: log})
	assert.ErrorContains(t, err, "invalid skill name")
	_, err = RecordFeedback("", FeedbackGreat, FeedbackOptions{LogPath: log})
	assert.Error(t, err)
	_, err = RecordFeedback("alpha", FeedbackGreat, FeedbackOptions{})
	assert.ErrorContains(t, err, "no feedback log")
	_, err = RecordFeedback("alpha", FeedbackGreat, FeedbackOptions{LogPath: log, NoteFile: filepath.Join(t.TempDir(), "missing")})
	assert.Error(t, err)
	assert.NoFileExists(t, log, "a failed note must not leave a half-written record")

	big := filepath.Join(t.TempDir(), "big")
	require.NoError(t, os.WriteFile(big, make([]byte, maxNoteBytes+1), 0o600))
	_, err = RecordFeedback("alpha", FeedbackGreat, FeedbackOptions{LogPath: log, NoteFile: big})
	assert.ErrorContains(t, err, "larger than")
}

func TestFeedback_ReadCountsAndReportJoin(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "f.jsonl")
	for _, kind := range []string{FeedbackMisled, FeedbackMisled, FeedbackGreat} {
		_, err := RecordFeedback("alpha", kind, FeedbackOptions{LogPath: log, Now: fixedClock})
		require.NoError(t, err)
	}
	_, err := RecordFeedback("idle", FeedbackStale, FeedbackOptions{LogPath: log, Now: fixedClock})
	require.NoError(t, err)
	f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("garbage\n{\"event\":\"skill_feedback\",\"id\":\"x\",\"kind\":\"bogus\"}\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	entries, skipped, err := ReadFeedback(log)
	require.NoError(t, err)
	assert.Len(t, entries, 4)
	assert.Equal(t, 2, skipped)
	counts := FeedbackCounts(entries)
	assert.Equal(t, map[string]int{"misled": 2, "great": 1}, counts["alpha"])
	assert.Equal(t, "misled 2, great 1", FeedbackText(counts["alpha"]))

	_, _, err = ReadFeedback(filepath.Join(dir, "none"))
	assert.Error(t, err)

	index := &Index{SchemaVersion: 1, Skills: []SkillRecord{{ID: "alpha"}, {ID: "idle"}}}
	report := BuildReport(index, []Entry{{ID: "alpha", Time: "t"}}, 0)
	report.Join(entries, map[string]EvalSummary{"alpha": {PassRate: 0.9, Passing: true}})
	assert.Equal(t, 4, report.FeedbackEvents)
	assert.Equal(t, 2, report.Used[0].Feedback["misled"])
	require.NotNil(t, report.Used[0].Eval)
	assert.InDelta(t, 0.9, report.Used[0].Eval.PassRate, 1e-9)
	assert.Equal(t, 1, report.Never[0].Feedback["stale"])
	assert.Nil(t, report.Never[0].Eval)

	// the JSON stays backward compatible: no new keys for rows without data
	plain, err := json.Marshal(BuildReport(index, nil, 0).Never[0])
	require.NoError(t, err)
	assert.NotContains(t, string(plain), "feedback")
	assert.NotContains(t, string(plain), "eval")
}

func TestSalt_EmptyFileIsReplacedAndLooseModeIsTightened(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.salt")
	require.NoError(t, os.WriteFile(path, nil, 0o644)) //nolint:gosec // a loose mode is the point of the test
	salt := loadSalt(path)
	assert.NotEmpty(t, salt, "an empty salt file must not silently drop every session hash")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, salt, loadSalt(path), "the regenerated salt is stable")

	loose := filepath.Join(dir, "loose.salt")
	require.NoError(t, os.WriteFile(loose, []byte("abc\n"), 0o644)) //nolint:gosec // a loose mode is the point of the test
	assert.Equal(t, "abc", loadSalt(loose))
	info, err = os.Stat(loose)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestFeedback_NoteIsBoundedBeforeReadingAndDirIsPrivate(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "f.jsonl")

	big := filepath.Join(dir, "big.txt")
	require.NoError(t, os.WriteFile(big, make([]byte, maxNoteBytes+10), 0o600))
	_, err := RecordFeedback("alpha", FeedbackWrong, FeedbackOptions{LogPath: log, NoteFile: big})
	require.Error(t, err)

	// a device or directory is not a note; it must be refused without reading it
	_, err = RecordFeedback("alpha", FeedbackWrong, FeedbackOptions{LogPath: log, NoteFile: dir})
	require.Error(t, err)
	if _, statErr := os.Stat("/dev/zero"); statErr == nil {
		_, err = RecordFeedback("alpha", FeedbackWrong, FeedbackOptions{LogPath: log, NoteFile: "/dev/zero"})
		require.Error(t, err)
	}

	// an existing world-listable notes directory is tightened
	notes := filepath.Join(dir, notesDirName)
	require.NoError(t, os.MkdirAll(notes, 0o755)) //nolint:gosec // a loose mode is the point of the test
	require.NoError(t, os.Chmod(notes, 0o755))    //nolint:gosec // umask may have narrowed it
	ok := filepath.Join(dir, "ok.txt")
	require.NoError(t, os.WriteFile(ok, []byte("fine"), 0o600))
	_, err = RecordFeedback("alpha", FeedbackWrong, FeedbackOptions{LogPath: log, NoteFile: ok})
	require.NoError(t, err)
	info, err := os.Stat(notes)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
