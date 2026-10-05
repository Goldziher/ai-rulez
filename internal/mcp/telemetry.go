package mcp

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/internal/telemetry"
)

// ItemRecorder receives one item event per tool call that reads or lists items.
// *telemetry.Pipeline satisfies it. Record must not block on the network.
type ItemRecorder interface {
	Record(ctx context.Context, event telemetry.Event) error
}

// TelemetryOptions sets the fields every MCP event carries.
type TelemetryOptions struct {
	Harness string
	Role    string
	// Session is a salted hash identifying this server process, so loads made
	// through one MCP connection group into one session.
	Session string
}

type itemTelemetry struct {
	recorder ItemRecorder
	options  TelemetryOptions
}

// argName is the tool argument holding an item name.
const argName = "name"

// toolItem says how a tool call maps to an item event.
type toolItem struct {
	kind string
	// idArg is the argument holding the item name; empty for a listing or search,
	// which records telemetry.ListID.
	idArg string
	// served marks tools of the served-skills surface.
	served bool
}

// telemetryTools lists the tools that read or list items. Adding a tool here is
// the whole integration for a new read tool (load_skill, find_skill): it is
// recorded after it succeeds, with no change to its handler.
var telemetryTools = map[string]toolItem{
	"read_rule":    {kind: telemetry.KindRule, idArg: argName},
	"read_context": {kind: telemetry.KindContext, idArg: argName},
	"read_skill":   {kind: telemetry.KindSkill, idArg: argName},
	"list_rules":   {kind: telemetry.KindRule},
	"list_context": {kind: telemetry.KindContext},
	"list_skills":  {kind: telemetry.KindSkill},
	// Served-skills surface (NewSkillServer).
	"get_skill":     {kind: telemetry.KindSkill, idArg: argName, served: true},
	"search_skills": {kind: telemetry.KindSkill, served: true},
	// Pending on the served-skills branch: find_skill / load_skill.
	"load_skill": {kind: telemetry.KindSkill, idArg: argName, served: true},
	"find_skill": {kind: telemetry.KindSkill, served: true},
}

// SetTelemetry turns on item telemetry for this server. It is safe to call before
// Run; with no call (or a nil recorder) tools behave exactly as before and no
// event is built.
func (s *Server) SetTelemetry(recorder ItemRecorder, options TelemetryOptions) {
	if recorder == nil {
		s.telemetry.Store(nil)
		return
	}
	s.telemetry.Store(&itemTelemetry{recorder: recorder, options: options})
}

// EmitItem records one item load from code that is not a registered tool (for
// example a resource read). It is a no-op when telemetry is off.
func (s *Server) EmitItem(ctx context.Context, kind, id string, served bool) {
	t := s.telemetry.Load()
	if t == nil {
		return
	}
	reason := telemetry.ReasonRead
	if id == "" {
		id, reason = telemetry.ListID, telemetry.ReasonList
	}
	_ = t.recorder.Record(ctx, telemetry.Event{ //nolint:errcheck // telemetry must never fail a tool call
		Kind: kind, ID: id, Source: telemetry.SourceMCP, Harness: t.options.Harness, Role: t.options.Role,
		Served: served, Session: t.options.Session, Outcome: telemetry.OutcomeLoaded, LoadReason: reason,
	})
}

func (s *Server) emitToolTelemetry(ctx context.Context, tool string, req *handlers.ToolRequest) {
	if s.telemetry.Load() == nil {
		return
	}
	item, ok := telemetryTools[tool]
	if !ok {
		return
	}
	id := ""
	if item.idArg != "" {
		id = req.GetString(item.idArg, "")
		if id == "" {
			return
		}
		id = skillRefName(id)
	}
	s.EmitItem(ctx, item.kind, id, item.served)
}

// skillRefName reduces a skill:// URI (skill://<name>/SKILL.md) to the skill name.
func skillRefName(ref string) string {
	rest, ok := strings.CutPrefix(ref, "skill://")
	if !ok {
		return ref
	}
	name, _, _ := strings.Cut(rest, "/")
	return name
}
