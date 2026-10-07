package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Hook event names handled by HandleHook. Claude Code's are verified against the
// hook input schemas in Claude Code 2.1.289; Codex uses the same SubagentStart and
// SubagentStop names and Cursor the camelCase forms (matched case-insensitively),
// both per their documentation (see docs/telemetry.md).
const (
	HookInstructionsLoaded = "InstructionsLoaded"
	HookSubagentStart      = "SubagentStart"
	HookSubagentStop       = "SubagentStop"
)

const (
	agentStateFile = "telemetry-agents.json"
	agentStateTTL  = 24 * time.Hour
	agentLockFile  = ".telemetry-agents.lock"
	hookInputLimit = 4 << 20
)

// hookPayload is the subset of a Claude Code hook input the handler reads.
// Fields such as prompt, transcript_path, last_assistant_message, globs and
// trigger_file_path are deliberately not declared, so they can never reach an
// event.
type hookPayload struct {
	Name       string `json:"hook_event_name"`
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	FilePath   string `json:"file_path"`
	MemoryType string `json:"memory_type"`
	LoadReason string `json:"load_reason"`
	AgentID    string `json:"agent_id"`
	AgentType  string `json:"agent_type"`
	// Cursor: conversation_id is the session, subagent_id / subagent_type name the
	// subagent, and subagentStop reports duration_ms itself.
	ConversationID string `json:"conversation_id"`
	SubagentID     string `json:"subagent_id"`
	SubagentType   string `json:"subagent_type"`
	DurationMS     int64  `json:"duration_ms"`
}

// normalize folds the Cursor field names into the Claude/Codex ones.
func (h *hookPayload) normalize() {
	if h.SessionID == "" {
		h.SessionID = h.ConversationID
	}
	if h.AgentID == "" {
		h.AgentID = h.SubagentID
	}
	if h.AgentType == "" {
		h.AgentType = h.SubagentType
	}
}

// HookOptions configures HandleHook.
type HookOptions struct {
	Harness string
	Role    string
}

// HandleHook reads one hook input from in and records the item event it
// describes, if any. It returns the event recorded, or nil for an event it does
// not handle. Only Claude Code's InstructionsLoaded, SubagentStart and
// SubagentStop are handled here; skill loads stay with `telemetry record`.
func (p *Pipeline) HandleHook(ctx context.Context, in io.Reader, options HookOptions) (*Event, error) {
	if !p.Settings.RecordActive() {
		return nil, nil // nothing is recorded: do not read or parse the harness's input
	}
	data, err := io.ReadAll(io.LimitReader(in, hookInputLimit))
	if err != nil {
		return nil, oops.Wrapf(err, "read hook event")
	}
	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, oops.Wrapf(err, "parse hook event")
	}
	payload.normalize()
	harness := options.Harness
	if harness == "" {
		harness = "claude"
	}
	event := Event{
		Source: SourceHook, Harness: harness, Role: options.Role,
		Session: p.Session(payload.SessionID), Outcome: OutcomeLoaded,
	}
	name := strings.ToLower(payload.Name)
	if harness != "claude" && !strings.HasPrefix(name, "subagent") {
		// Codex and Cursor document no instruction-load event, so a stray one is not recorded.
		return nil, nil
	}
	switch name {
	case strings.ToLower(HookInstructionsLoaded):
		item := ClassifyInstruction(p.Root, payload.CWD, payload.FilePath)
		event.Kind, event.ID = item.Kind, item.ID
		event.LoadReason, event.MemoryType = payload.LoadReason, payload.MemoryType
		if item.Path != "" {
			event.Digest = FileDigest(filepath.Join(p.Root, filepath.FromSlash(item.Path)))
			if p.Settings.IncludePaths {
				event.Path = item.Path
			}
		}
	case strings.ToLower(HookSubagentStart):
		if payload.AgentType == "" {
			return nil, nil // no id to record under; a hook must not fail the harness over it
		}
		event.Kind, event.ID, event.LoadReason = KindAgent, payload.AgentType, ReasonSubagentStart
		p.noteAgentStart(payload)
	case strings.ToLower(HookSubagentStop):
		if payload.AgentType == "" {
			return nil, nil
		}
		event.Kind, event.ID, event.LoadReason = KindAgent, payload.AgentType, ReasonSubagentStop
		event.Outcome = OutcomeUsed
		event.DurationMS = payload.DurationMS
		if event.DurationMS == 0 {
			event.DurationMS = p.agentDuration(payload)
		}
	default:
		return nil, nil
	}
	if err := p.Record(ctx, event); err != nil {
		return nil, err
	}
	return &event, nil
}

// agentKey identifies one subagent run without storing the raw ids.
func (p *Pipeline) agentKey(payload hookPayload) string {
	if payload.AgentID == "" || p.Salt == "" {
		return ""
	}
	return p.Session(payload.SessionID) + ":" + p.Session(payload.AgentID)
}

func (p *Pipeline) agentStatePath() string {
	return filepath.Join(filepath.Dir(p.LogPath), agentStateFile)
}

// noteAgentStart remembers when a subagent started so SubagentStop can report its
// duration. State is a small 0600 file of hashed keys to unix milliseconds.
func (p *Pipeline) noteAgentStart(payload hookPayload) {
	key := p.agentKey(payload)
	if key == "" {
		return
	}
	p.updateAgents(func(m map[string]int64) { m[key] = p.Clock().UnixMilli() })
}

func (p *Pipeline) agentDuration(payload hookPayload) int64 {
	key := p.agentKey(payload)
	if key == "" {
		return 0
	}
	var started int64
	p.updateAgents(func(m map[string]int64) {
		started = m[key]
		delete(m, key)
	})
	if started == 0 {
		return 0
	}
	return max(p.Clock().UnixMilli()-started, 0)
}

func (p *Pipeline) updateAgents(fn func(map[string]int64)) {
	dir := filepath.Dir(p.LogPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	release, err := lock(filepath.Join(dir, agentLockFile), appendLockWait, staleLock)
	if err != nil {
		return
	}
	defer release()
	state := map[string]int64{}
	if data, readErr := safefs.ReadRegular(p.agentStatePath()); readErr == nil {
		_ = json.Unmarshal(data, &state) //nolint:errcheck // a corrupt file starts over
	}
	cutoff := p.Clock().Add(-agentStateTTL).UnixMilli()
	for key, started := range state {
		if started < cutoff {
			delete(state, key)
		}
	}
	fn(state)
	if data, err := json.Marshal(state); err == nil {
		_ = writeFileAtomic(p.agentStatePath(), data) //nolint:errcheck // duration is best effort
	}
}
