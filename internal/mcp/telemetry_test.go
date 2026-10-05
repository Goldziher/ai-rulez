package mcp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/telemetry"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRecorder struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (f *fakeRecorder) Record(_ context.Context, e telemetry.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeRecorder) snapshot() []telemetry.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]telemetry.Event(nil), f.events...)
}

func connect(t *testing.T, srv *Server) *sdkmcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, s *sdkmcp.ClientSession, tool string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	return res
}

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, "rules"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "rules", "atomic-commits.md"), []byte("# Atomic\n"), 0o600))
	return dir
}

func TestTelemetry_ReadAndListToolsEmitMCPEvents(t *testing.T) {
	srv := NewServer("test")
	rec := &fakeRecorder{}
	srv.SetTelemetry(rec, TelemetryOptions{Harness: "claude", Role: "backend", Session: "5b1c0e9a7d3f2a64"})
	session := connect(t, srv)
	dir := project(t)

	res := call(t, session, "read_rule", map[string]any{"name": "atomic-commits", "working_directory": dir})
	require.False(t, res.IsError)
	res = call(t, session, "list_rules", map[string]any{"working_directory": dir})
	require.False(t, res.IsError)

	events := rec.snapshot()
	require.Len(t, events, 2)
	assert.Equal(t, telemetry.KindRule, events[0].Kind)
	assert.Equal(t, "atomic-commits", events[0].ID)
	assert.Equal(t, telemetry.SourceMCP, events[0].Source)
	assert.Equal(t, telemetry.ReasonRead, events[0].LoadReason)
	assert.Equal(t, "backend", events[0].Role)
	assert.Equal(t, "5b1c0e9a7d3f2a64", events[0].Session)
	assert.False(t, events[0].Served)
	assert.Equal(t, telemetry.ListID, events[1].ID)
	assert.Equal(t, telemetry.ReasonList, events[1].LoadReason)
	for i := range events {
		e := events[i]
		require.NoError(t, e.Normalize(), "every emitted event must pass the model's validation")
	}
}

func TestTelemetry_FailedCallsAndDisabledTelemetryEmitNothing(t *testing.T) {
	srv := NewServer("test")
	rec := &fakeRecorder{}
	srv.SetTelemetry(rec, TelemetryOptions{})
	session := connect(t, srv)
	dir := project(t)

	res := call(t, session, "read_rule", map[string]any{"name": "does-not-exist", "working_directory": dir})
	assert.True(t, res.IsError)
	call(t, session, "create_rule", map[string]any{"name": "new-rule", "content": "x", "working_directory": dir})
	assert.Empty(t, rec.snapshot(), "a failed read and a write are not loads")

	// With telemetry never set, the same calls behave as before and nothing panics.
	plain := connect(t, NewServer("test"))
	assert.False(t, call(t, plain, "read_rule", map[string]any{"name": "atomic-commits", "working_directory": dir}).IsError)

	// Setting a nil recorder turns it off again.
	srv.SetTelemetry(nil, TelemetryOptions{})
	call(t, session, "read_rule", map[string]any{"name": "atomic-commits", "working_directory": dir})
	assert.Empty(t, rec.snapshot())
}

func TestTelemetry_ServedSkillTools(t *testing.T) {
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	srv := NewSkillServer("test", cat)
	rec := &fakeRecorder{}
	srv.SetTelemetry(rec, TelemetryOptions{Harness: "claude"})
	session := connect(t, srv)

	require.False(t, call(t, session, "get_skill", map[string]any{"name": "git-workflow"}).IsError)
	require.False(t, call(t, session, "get_skill", map[string]any{"name": "skill://pdf-processing/SKILL.md"}).IsError)
	require.False(t, call(t, session, "search_skills", map[string]any{"query": "pdf"}).IsError)

	events := rec.snapshot()
	require.Len(t, events, 3)
	assert.Equal(t, "git-workflow", events[0].ID)
	assert.True(t, events[0].Served)
	assert.Equal(t, "pdf-processing", events[1].ID, "a skill:// URI reduces to the skill name")
	assert.Equal(t, telemetry.ListID, events[2].ID)
}

func TestSkillRefName(t *testing.T) {
	assert.Equal(t, "a", skillRefName("skill://a/SKILL.md"))
	assert.Equal(t, "a", skillRefName("a"))
	assert.Equal(t, "plugin:a", skillRefName("plugin:a"))
}
