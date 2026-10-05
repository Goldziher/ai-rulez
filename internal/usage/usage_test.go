package usage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixedClock = func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }

func writeIndex(t *testing.T, dir string, records ...SkillRecord) string {
	t.Helper()
	data, err := (&Index{SchemaVersion: IndexSchemaVersion, Skills: records}).Marshal()
	require.NoError(t, err)
	path := filepath.Join(dir, ".ai-rulez", IndexFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestIndexMarshal_IsSortedStableAndEndsWithNewline(t *testing.T) {
	t.Parallel()

	index := &Index{SchemaVersion: IndexSchemaVersion, Skills: []SkillRecord{
		{ID: "b", Source: "s/b", Hash: "h"},
		{ID: "a", Source: "z/a", Hash: "h", Outputs: map[string][]string{"claude": {"x"}, "codex": {"y"}}},
		{ID: "a", Source: "a/a", Hash: "h"},
	}}
	first, err := index.Marshal()
	require.NoError(t, err)
	second, err := index.Marshal()
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.True(t, bytes.HasSuffix(first, []byte("\n")))
	var decoded Index
	require.NoError(t, json.Unmarshal(first, &decoded))
	assert.Equal(t, []string{"a/a", "z/a", "s/b"}, []string{decoded.Skills[0].Source, decoded.Skills[1].Source, decoded.Skills[2].Source})
	assert.Equal(t, "a", index.Skills[1].ID, "Marshal must not reorder the receiver")
}

func TestLoadIndex_Errors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := LoadIndex(filepath.Join(dir, "missing.json"))
	require.Error(t, err)

	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"schema_version": 99}`), 0o600))
	_, err = LoadIndex(bad)
	require.ErrorContains(t, err, "unsupported")
}

func TestRecord(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	indexPath := writeIndex(t, project, SkillRecord{ID: "alpha", Source: "s", Hash: "blake3:aa"},
		SkillRecord{ID: "dup", Source: "one", Hash: "blake3:11"}, SkillRecord{ID: "dup", Source: "two", Hash: "blake3:22"})

	tests := []struct {
		name    string
		event   string
		want    *Entry
		noEntry bool
	}{
		{
			name:  "skill tool call",
			event: `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Skill","tool_input":{"skill":"alpha"}}`,
			want:  &Entry{Skill: "alpha", ID: "alpha", Hash: "blake3:aa", Session: "s1", Invocation: "tool"},
		},
		{
			name:  "plugin skill resolves to its own name",
			event: `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"billing:alpha"}}`,
			want:  &Entry{Skill: "billing:alpha", ID: "alpha", Hash: "blake3:aa", Invocation: "tool"},
		},
		{
			name:  "slash command",
			event: `{"hook_event_name":"UserPromptExpansion","session_id":"s2","command_name":"alpha","command_args":"secret","prompt":"/alpha secret"}`,
			want:  &Entry{Skill: "alpha", ID: "alpha", Hash: "blake3:aa", Session: "s2", Invocation: "slash"},
		},
		{
			name:  "ambiguous id has no hash",
			event: `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"dup"}}`,
			want:  &Entry{Skill: "dup", ID: "dup", Invocation: "tool"},
		},
		{
			name:  "unknown skill is still logged",
			event: `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"ghost"}}`,
			want:  &Entry{Skill: "ghost", ID: "ghost", Invocation: "tool"},
		},
		{name: "other tool", event: `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`, noEntry: true},
		{name: "other event", event: `{"hook_event_name":"SessionStart"}`, noEntry: true},
		{name: "skill tool without a name", event: `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{}}`, noEntry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := filepath.Join(t.TempDir(), "nested", "usage.jsonl")
			entry, err := Record(strings.NewReader(tt.event), RecordOptions{LogPath: log, IndexPath: indexPath, Now: fixedClock})
			require.NoError(t, err)
			if tt.noEntry {
				assert.Nil(t, entry)
				_, statErr := os.Stat(log)
				assert.True(t, os.IsNotExist(statErr), "an ignored event must not create the log")
				return
			}
			tt.want.Time, tt.want.Event, tt.want.Harness = "2026-10-04T12:00:00Z", EventSkillInvoked, "claude"
			tt.want.Version, tt.want.Outcome = EntrySchemaVersion, OutcomeLoaded
			if tt.want.Session != "" {
				salt, saltErr := os.ReadFile(filepath.Join(filepath.Dir(log), "usage.salt"))
				require.NoError(t, saltErr)
				tt.want.Session = HashSession(strings.TrimSpace(string(salt)), tt.want.Session)
				assert.Len(t, tt.want.Session, 16)
			}
			assert.Equal(t, tt.want, entry)

			data, err := os.ReadFile(log)
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(string(data), "\n"))
			assert.NotContains(t, string(data), "secret", "arguments and prompts never reach the log")
			assert.NotContains(t, string(data), "command_args")
		})
	}
}

func TestRecord_FindsTheIndexFromTheEventWorkingDirectory(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeIndex(t, project, SkillRecord{ID: "alpha", Hash: "blake3:aa"})
	event, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Skill", "cwd": project,
		"tool_input": map[string]any{"skill": "alpha"},
	})
	require.NoError(t, err)

	entry, err := Record(bytes.NewReader(event), RecordOptions{LogPath: filepath.Join(t.TempDir(), "l"), Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, "blake3:aa", entry.Hash)
}

func TestRecord_AppendsAndRejectsGarbage(t *testing.T) {
	t.Parallel()

	log := filepath.Join(t.TempDir(), "usage.jsonl")
	event := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"a"}}`
	for range 3 {
		_, err := Record(strings.NewReader(event), RecordOptions{LogPath: log, Now: fixedClock})
		require.NoError(t, err)
	}
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(string(data), "\n"))

	_, err = Record(strings.NewReader("not json"), RecordOptions{LogPath: log})
	require.Error(t, err)
}

func TestRecord_SinkCommandReceivesTheLine(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	sink := filepath.Join(t.TempDir(), "sink.txt")
	event := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"a"}}`
	_, err := Record(strings.NewReader(event), RecordOptions{SinkCommand: "cat > '" + sink + "'", Now: fixedClock})
	require.NoError(t, err)
	data, err := os.ReadFile(sink)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"skill":"a"`)

	_, err = Record(strings.NewReader(event), RecordOptions{SinkCommand: "exit 3"})
	require.Error(t, err)
}

func TestBuildReport(t *testing.T) {
	t.Parallel()

	index := &Index{SchemaVersion: IndexSchemaVersion, Skills: []SkillRecord{
		{ID: "used", Hash: "blake3:new", Owner: "team"},
		{ID: "edited", Hash: "blake3:new"},
		{ID: "idle", Domain: "ops", Owner: "ops-team"},
		{ID: "also-idle"},
	}}
	entries := []Entry{
		{Time: "2026-01-01T00:00:00Z", ID: "used", Hash: "blake3:new"},
		{Time: "2026-02-01T00:00:00Z", ID: "used", Hash: "blake3:new"},
		{Time: "2026-01-15T00:00:00Z", ID: "used", Hash: "blake3:new"},
		{Time: "2026-01-05T00:00:00Z", ID: "edited", Hash: "blake3:old"},
		{Time: "2026-01-06T00:00:00Z", ID: "edited", Hash: "blake3:new"},
		{Time: "2026-01-07T00:00:00Z", ID: "removed"},
	}
	report := BuildReport(index, entries, 2)

	assert.Equal(t, 6, report.Events)
	assert.Equal(t, 2, report.Skipped)
	require.Len(t, report.Used, 2)
	assert.Equal(t, "used", report.Used[0].ID, "most used first")
	assert.Equal(t, "2026-02-01T00:00:00Z", report.Used[0].LastSeen)
	assert.Equal(t, []string{"also-idle", "idle"}, []string{report.Never[0].ID, report.Never[1].ID})
	require.Len(t, report.Changed, 1)
	assert.Equal(t, ChangedSkill{ID: "edited", CurrentHash: "blake3:new", LoggedHash: []string{"blake3:old"}}, report.Changed[0])
	require.Len(t, report.Unknown, 1)
	assert.Equal(t, "removed", report.Unknown[0].ID)
}

func TestReadLog_SkipsUnreadableLines(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "log.jsonl")
	content := `{"event":"skill_invoked","id":"a","ts":"t"}` + "\n\nnot json\n" + `{"event":"other","id":"b"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	entries, skipped, err := ReadLog(path)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, 2, skipped)
}

func TestHookTemplate(t *testing.T) {
	t.Parallel()

	template, err := HookTemplate(HookTemplateOptions{})
	require.NoError(t, err)
	var decoded struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(template, &decoded))
	assert.Equal(t, "Skill", decoded.Hooks["PreToolUse"][0].Matcher)
	assert.Equal(t, "*", decoded.Hooks["UserPromptExpansion"][0].Matcher)
	command := decoded.Hooks["PreToolUse"][0].Hooks[0].Command
	assert.Equal(t, "command", decoded.Hooks["PreToolUse"][0].Hooks[0].Type)
	assert.Contains(t, command, "ai-rulez usage record")
	assert.Contains(t, command, `--log "${CLAUDE_PROJECT_DIR}/.ai-rulez/local/usage.jsonl"`)

	custom, err := HookTemplate(HookTemplateOptions{Executable: "/bin/ai-rulez", SinkCommand: "it's", IndexPath: "a b"})
	require.NoError(t, err)
	assert.Contains(t, string(custom), `--sink-command 'it'\\''s'`)
	assert.Contains(t, string(custom), `--index \"a b\"`)
	assert.NotContains(t, string(custom), "--log", "a sink command alone replaces the default log")
}
