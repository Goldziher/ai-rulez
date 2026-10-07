package telemetry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipelineFor builds a recording pipeline rooted at a temp project, with a
// clock the test advances and a spawn counter.
func pipelineFor(t *testing.T, settings Settings) (*Pipeline, *time.Time, *atomic.Int32) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
	now := fixedNow
	spawns := &atomic.Int32{}
	p := Build(settings, BuildOptions{
		Root: root, ConfigDirName: ".ai-rulez", Version: "test", LocalLog: true,
		Clock: func() time.Time { return now },
		Spawn: func() error { spawns.Add(1); return nil },
	})
	return p, &now, spawns
}

func enabled() Settings {
	return Resolve(Layers{Repo: &config.TelemetryConfig{Enabled: ptrBool(true)}, Getenv: env()})
}

func exporting() Settings {
	return Resolve(Layers{User: &config.TelemetryConfig{Enabled: ptrBool(true), AllowNetwork: true, OTLPEndpoint: "http://127.0.0.1:1"}, Getenv: env()})
}

func hookJSON(fields string) *bytes.Reader {
	return bytes.NewReader([]byte(`{"session_id":"sess-1","cwd":"/x",` + fields + `}`))
}

func TestHandleHook_InstructionsLoadedBecomesARuleEvent(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	ruleFile := filepath.Join(p.Root, ".claude", "rules", "atomic-commits.md")
	write(t, ruleFile, "# Atomic commits\nsecret prompt text\n")

	in := hookJSON(`"hook_event_name":"InstructionsLoaded","file_path":"` + ruleFile + `","memory_type":"Project","load_reason":"path_glob_match","globs":["**/*.go"],"trigger_file_path":"` + p.Root + `/main.go","prompt":"TOP SECRET PROMPT"`)
	event, err := p.HandleHook(context.Background(), in, HookOptions{Role: "backend"})
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, KindRule, event.Kind)
	assert.Equal(t, "atomic-commits", event.ID)
	assert.Equal(t, "path_glob_match", event.LoadReason)
	assert.Equal(t, "Project", event.MemoryType)
	assert.Equal(t, "backend", event.Role)
	assert.Equal(t, "claude", event.Harness)
	assert.Equal(t, SourceHook, event.Source)
	assert.True(t, strings.HasPrefix(event.Digest, "blake3:"), event.Digest)
	assert.Empty(t, event.Path, "paths are off unless include_paths is set")
	assert.Len(t, event.Session, 16)
	assert.NotContains(t, event.Session, "sess-1")

	log, err := os.ReadFile(p.LogPath)
	require.NoError(t, err)
	for _, leak := range []string{"TOP SECRET", "secret prompt text", "**/*.go", "main.go", "sess-1", p.Root} {
		assert.NotContains(t, string(log), leak)
	}
}

func TestHandleHook_ContextFilesAndOutsideFiles(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	home := t.TempDir()
	cases := []struct {
		file, kind, id string
	}{
		{filepath.Join(p.Root, "CLAUDE.md"), KindContext, "CLAUDE.md"},
		{filepath.Join(p.Root, "services", "api", "CLAUDE.md"), KindContext, "services/api/CLAUDE.md"},
		{filepath.Join(p.Root, ".claude", "rules", "go", "style.md"), KindRule, "go/style"},
		{filepath.Join(home, ".claude", "CLAUDE.md"), KindContext, "CLAUDE.md"},
		{filepath.Join(home, ".claude", "rules", "mine.md"), KindRule, "mine"},
	}
	for _, c := range cases {
		item := ClassifyInstruction(p.Root, p.Root, c.file)
		assert.Equal(t, c.kind, item.Kind, c.file)
		assert.Equal(t, c.id, item.ID, c.file)
		assert.NotContains(t, item.ID, home, "a home directory never becomes part of an id")
	}
	assert.Empty(t, ClassifyInstruction(p.Root, p.Root, filepath.Join(home, ".claude", "CLAUDE.md")).Path)
	// A relative file_path resolves against the payload cwd.
	assert.Equal(t, "CLAUDE.md", ClassifyInstruction(p.Root, p.Root, "CLAUDE.md").ID)
}

func TestHandleHook_IncludePathsOptIn(t *testing.T) {
	s := Resolve(Layers{User: &config.TelemetryConfig{Enabled: ptrBool(true), IncludePaths: true}, Getenv: env()})
	p, _, _ := pipelineFor(t, s)
	file := filepath.Join(p.Root, ".claude", "rules", "r.md")
	event, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"InstructionsLoaded","file_path":"`+file+`","memory_type":"Project","load_reason":"session_start"`), HookOptions{})
	require.NoError(t, err)
	assert.Equal(t, ".claude/rules/r.md", event.Path)
}

func TestHandleHook_SubagentDuration(t *testing.T) {
	p, now, _ := pipelineFor(t, enabled())
	start, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"code-reviewer"`), HookOptions{})
	require.NoError(t, err)
	assert.Equal(t, KindAgent, start.Kind)
	assert.Equal(t, OutcomeLoaded, start.Outcome)

	*now = now.Add(2500 * time.Millisecond)
	stop, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStop","agent_id":"a1","agent_type":"code-reviewer","last_assistant_message":"PRIVATE","agent_transcript_path":"/tmp/t.jsonl"`), HookOptions{})
	require.NoError(t, err)
	assert.Equal(t, OutcomeUsed, stop.Outcome)
	assert.EqualValues(t, 2500, stop.DurationMS)

	// Stop without a Start records no duration rather than guessing one.
	orphan, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStop","agent_id":"zzz","agent_type":"x"`), HookOptions{})
	require.NoError(t, err)
	assert.Zero(t, orphan.DurationMS)

	log, _ := os.ReadFile(p.LogPath)
	assert.NotContains(t, string(log), "PRIVATE")
	assert.NotContains(t, string(log), "t.jsonl")
	info, err := os.Stat(p.agentStatePath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestHandleHook_IgnoresOtherEventsAndDisabledTelemetry(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	event, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"PreToolUse","tool_name":"Bash"`), HookOptions{})
	require.NoError(t, err)
	assert.Nil(t, event)

	off, _, _ := pipelineFor(t, Resolve(Layers{Getenv: env()}))
	event, err = off.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStart","agent_type":"x"`), HookOptions{})
	require.NoError(t, err)
	assert.Nil(t, event)
	_, statErr := os.Stat(off.LogPath)
	assert.True(t, os.IsNotExist(statErr), "nothing is written when telemetry is off")

	_, err = p.HandleHook(context.Background(), strings.NewReader("not json"), HookOptions{})
	assert.Error(t, err)
}

func TestPipeline_RecordNeverWaitsOnADeadCollector(t *testing.T) {
	p, _, spawns := pipelineFor(t, exporting())
	require.NotNil(t, p.Spool)
	start := time.Now()
	for i := 0; i < 20; i++ {
		require.NoError(t, p.Record(context.Background(), Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}))
	}
	per := time.Since(start) / 20
	assert.Less(t, per, 50*time.Millisecond, "recording with a dead endpoint must stay under the 50 ms budget")
	assert.EqualValues(t, 1, spawns.Load(), "one detached flush is requested, rate limited")
	events, _, err := p.Spool.Pending()
	require.NoError(t, err)
	assert.Len(t, events, 20)
}

func TestPipeline_SamplingAppliesToExportNotToTheLocalLog(t *testing.T) {
	s := Resolve(Layers{User: &config.TelemetryConfig{Enabled: ptrBool(true), AllowNetwork: true, OTLPEndpoint: "http://127.0.0.1:1", Sample: ptrFloat(0)}, Getenv: env()})
	p, _, _ := pipelineFor(t, s)
	require.NoError(t, p.Record(context.Background(), Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}))
	events, _, _ := p.Spool.Pending()
	assert.Empty(t, events)
	log, _ := os.ReadFile(p.LogPath)
	assert.Contains(t, string(log), `"id":"r"`)
}

func ptrFloat(f float64) *float64 { return &f }

func TestHandleHook_CodexSubagentEvents(t *testing.T) {
	p, now, _ := pipelineFor(t, enabled())
	start, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStart","turn_id":"t1","agent_id":"a1","agent_type":"explorer","permission_mode":"default"`), HookOptions{Harness: "codex"})
	require.NoError(t, err)
	require.NotNil(t, start)
	assert.Equal(t, "codex", start.Harness)
	assert.Equal(t, KindAgent, start.Kind)
	assert.Equal(t, "explorer", start.ID)

	*now = now.Add(time.Second)
	stop, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"SubagentStop","agent_id":"a1","agent_type":"explorer","last_assistant_message":"PRIVATE","stop_hook_active":false`), HookOptions{Harness: "codex"})
	require.NoError(t, err)
	assert.EqualValues(t, 1000, stop.DurationMS)
	log, _ := os.ReadFile(p.LogPath)
	assert.NotContains(t, string(log), "PRIVATE")
}

func TestHandleHook_CursorSubagentEvents(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	cursor := func(fields string) *bytes.Reader {
		return bytes.NewReader([]byte(`{"conversation_id":"conv-9","cwd":"/x",` + fields + `}`))
	}
	start, err := p.HandleHook(context.Background(), cursor(`"hook_event_name":"subagentStart","subagent_id":"s1","subagent_type":"explore","task":"PRIVATE TASK","git_branch":"main"`), HookOptions{Harness: "cursor"})
	require.NoError(t, err)
	require.NotNil(t, start)
	assert.Equal(t, "cursor", start.Harness)
	assert.Equal(t, "explore", start.ID)
	assert.Equal(t, KindAgent, start.Kind)
	assert.Equal(t, p.Session("conv-9"), start.Session, "the conversation id identifies the session")

	stop, err := p.HandleHook(context.Background(), cursor(`"hook_event_name":"subagentStop","subagent_type":"explore","status":"completed","summary":"PRIVATE SUMMARY","duration_ms":4200,"modified_files":["/x/secret.go"]`), HookOptions{Harness: "cursor"})
	require.NoError(t, err)
	assert.Equal(t, OutcomeUsed, stop.Outcome)
	assert.EqualValues(t, 4200, stop.DurationMS, "Cursor reports the duration itself")

	log, _ := os.ReadFile(p.LogPath)
	for _, leak := range []string{"PRIVATE", "secret.go", "conv-9"} {
		assert.NotContains(t, string(log), leak)
	}
}

func TestHandleHook_InstructionsLoadedIsClaudeOnly(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	for _, harness := range []string{"codex", "cursor"} {
		event, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"InstructionsLoaded","file_path":"/x/CLAUDE.md"`), HookOptions{Harness: harness})
		require.NoError(t, err)
		assert.Nil(t, event, harness)
	}
}

func TestHandleHook_InactiveRecordingDoesNotReadOrParseTheInput(t *testing.T) {
	off, _, _ := pipelineFor(t, Settings{})

	event, err := off.HandleHook(context.Background(), strings.NewReader("not json"), HookOptions{})

	require.NoError(t, err)
	assert.Nil(t, event)
}

func TestHandleHook_SubagentWithoutATypeIsSkipped(t *testing.T) {
	p, _, _ := pipelineFor(t, enabled())
	for _, name := range []string{"SubagentStart", "SubagentStop"} {
		t.Run(name, func(t *testing.T) {
			event, err := p.HandleHook(context.Background(), hookJSON(`"hook_event_name":"`+name+`","agent_id":"a1"`), HookOptions{})

			require.NoError(t, err, "a hook must not fail the harness over a missing field")
			assert.Nil(t, event)
		})
	}
}

func TestBuild_NoProjectRecordsNothingAndCreatesNothing(t *testing.T) {
	root := t.TempDir()
	p := Build(enabled(), BuildOptions{Root: root, ConfigDirName: ".ai-rulez", LocalLog: true})
	event := Event{Version: 1, Name: EventItem, Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}
	require.NoError(t, p.Recorder.Record(context.Background(), event))
	_, err := os.Stat(filepath.Join(root, ".ai-rulez"))
	assert.True(t, os.IsNotExist(err), "no .ai-rulez/local may appear outside a project")
}
