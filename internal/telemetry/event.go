package telemetry

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// SchemaVersion is the version written to the "v" field of item events. A new
// optional field does not bump it; a changed meaning or a removed field does.
const SchemaVersion = 1

// EventItem is the event name of every item event in the shared log.
const EventItem = usage.EventItem

// EventEvalResult is the event name of an eval result: the recorded scores of one
// skill at one digest. It is built from the eval store at export time and never
// written to the usage log.
const EventEvalResult = "eval_result"

// Item kinds.
const (
	KindSkill   = "skill"
	KindRule    = "rule"
	KindAgent   = "agent"
	KindCommand = "command"
	KindContext = "context"
)

// Kinds lists the accepted item kinds in display order.
var Kinds = []string{KindSkill, KindRule, KindAgent, KindCommand, KindContext}

// ListID is the id of an event for a listing or search call: the caller saw a
// catalog, not one item. Reports count it under load reasons and never list it
// as an item.
const ListID = "_list"

// Sources say which component observed the load.
const (
	SourceHook = "hook"
	SourceMCP  = "mcp"
	SourceCLI  = "cli"
)

// Outcomes of a load.
const (
	OutcomeLoaded    = usage.OutcomeLoaded
	OutcomeUsed      = usage.OutcomeUsed
	OutcomeAbandoned = usage.OutcomeAbandoned
)

// Load reasons the hooks produce. The field is free text in the model (a harness
// may add reasons) but is constrained to a short lower-case token.
const (
	ReasonSessionStart  = "session_start"
	ReasonNested        = "nested_traversal"
	ReasonPathGlob      = "path_glob_match"
	ReasonInclude       = "include"
	ReasonCompact       = "compact"
	ReasonSubagentStart = "subagent_start"
	ReasonSubagentStop  = "subagent_stop"
	ReasonList          = "list"
	ReasonRead          = "read"
)

// Event is one item event. Every field is an identifier or a small enum.
type Event struct {
	Version int    `json:"v"`
	Name    string `json:"event"`
	Time    string `json:"ts"`
	// EventID de-duplicates replays: the spool is at-least-once.
	EventID string `json:"event_id,omitempty"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	// Path is relative to the repository root, forward slashes. Empty for an item
	// outside the repository (a user-level rule) and whenever the caller has not
	// opted in to paths.
	Path string `json:"path,omitempty"`
	// Digest is the content digest when known: the lock's canonical "sha256:..."
	// digest for a skill that has one, else "blake3:..." (the usage index hash or a
	// hash of the instruction file a hook saw).
	Digest string `json:"digest,omitempty"`
	// DigestScheme names how a sha256 Digest was computed (usage.DigestSchemeSkill,
	// the canonical lock digest an eval joins on, or usage.DigestSchemeServed), so a
	// reader of an OTLP export can tell the two apart. Empty for a blake3 digest,
	// which names itself.
	DigestScheme string `json:"digest_scheme,omitempty"`
	Source       string `json:"source"`
	Harness      string `json:"harness,omitempty"`
	Role         string `json:"role,omitempty"`
	Served       bool   `json:"served,omitempty"`
	// Session is a salted hash of the harness session id, never the raw id.
	Session    string `json:"session,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	LoadReason string `json:"load_reason,omitempty"`
	// MemoryType is Claude Code's memory_type: User, Project, Local or Managed.
	MemoryType string `json:"memory_type,omitempty"`
	// DurationMS is set on agent completion.
	DurationMS int64 `json:"duration_ms,omitempty"`

	// The scores of an eval_result event (0..1; the ablation delta -1..1). They are
	// numbers the eval runner recorded, never prompts or outputs.
	PassRate         *float64 `json:"pass_rate,omitempty"`
	TriggerPrecision *float64 `json:"trigger_precision,omitempty"`
	TriggerRecall    *float64 `json:"trigger_recall,omitempty"`
	AblationDelta    *float64 `json:"ablation_delta,omitempty"`
}

var (
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/@+:-]{0,159}$`)
	harnessPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	rolePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	eventIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	sessionPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	digestPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_:.-]{0,135}$`)
	reasonPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

var memoryTypes = map[string]bool{"User": true, "Project": true, "Local": true, "Managed": true}

// Normalize validates e in place. The identity fields (kind, id, source, outcome)
// are required and an invalid value is an error; an invalid optional field is
// dropped, never exported half-sanitized. It does not set the time or event id:
// the Recorder does, in one place.
func (e *Event) Normalize() error {
	if e.Name == EventEvalResult {
		return e.normalizeEval()
	}
	e.Version, e.Name = SchemaVersion, EventItem
	e.PassRate, e.TriggerPrecision, e.TriggerRecall, e.AblationDelta = nil, nil, nil, nil // scores belong to eval_result only
	if !isKind(e.Kind) {
		return oops.With("kind", e.Kind).Errorf("telemetry: unknown item kind")
	}
	if !idPattern.MatchString(e.ID) || hasDotDot(e.ID) {
		return oops.Errorf("telemetry: invalid item id")
	}
	switch e.Source {
	case SourceHook, SourceMCP, SourceCLI:
	default:
		return oops.With("source", e.Source).Errorf("telemetry: unknown source")
	}
	switch e.Outcome {
	case OutcomeLoaded, OutcomeUsed, OutcomeAbandoned:
	default:
		return oops.With("outcome", e.Outcome).Errorf("telemetry: unknown outcome")
	}
	if cleaned, ok := cleanRelPath(e.Path); ok {
		e.Path = cleaned
	} else {
		e.Path = ""
	}
	if !matches(digestPattern, e.Digest) {
		e.Digest = ""
	}
	if !validDigestScheme(e.Digest, e.DigestScheme) {
		e.DigestScheme = ""
	}
	clearUnless(harnessPattern, &e.Harness)
	clearUnless(rolePattern, &e.Role)
	clearUnless(sessionPattern, &e.Session)
	clearUnless(reasonPattern, &e.LoadReason)
	if !memoryTypes[e.MemoryType] {
		e.MemoryType = ""
	}
	clearBadTime(&e.Time)
	if e.DurationMS < 0 {
		e.DurationMS = 0
	}
	return nil
}

// validDigestScheme accepts a scheme only beside a sha256 digest, and only the two
// the usage log defines.
func validDigestScheme(digest, scheme string) bool {
	if !strings.HasPrefix(digest, "sha256:") {
		return false
	}
	return scheme == usage.DigestSchemeSkill || scheme == usage.DigestSchemeServed
}

var skillDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// normalizeEval validates an eval_result event. Its identity is a skill id and a
// score; everything an item event would carry about a load (path, session, role,
// outcome, reason) is cleared, so a result cannot smuggle one out.
func (e *Event) normalizeEval() error {
	e.Version = SchemaVersion
	if e.Kind != KindSkill {
		return oops.With("kind", e.Kind).Errorf("telemetry: an eval result is about a skill")
	}
	if !idPattern.MatchString(e.ID) || hasDotDot(e.ID) {
		return oops.Errorf("telemetry: invalid item id")
	}
	if e.PassRate == nil || !inRange(*e.PassRate, 0, 1) {
		return oops.Errorf("telemetry: an eval result needs a pass rate between 0 and 1")
	}
	for _, score := range []**float64{&e.TriggerPrecision, &e.TriggerRecall} {
		if *score != nil && !inRange(**score, 0, 1) {
			*score = nil
		}
	}
	if e.AblationDelta != nil && !inRange(*e.AblationDelta, -1, 1) {
		e.AblationDelta = nil
	}
	e.Source = SourceCLI
	e.Outcome, e.Path, e.Session, e.Role, e.LoadReason, e.MemoryType = "", "", "", "", "", ""
	e.Served, e.DurationMS = false, 0
	if !skillDigestPattern.MatchString(e.Digest) || e.DigestScheme != usage.DigestSchemeSkill {
		e.Digest, e.DigestScheme = "", ""
	}
	clearUnless(harnessPattern, &e.Harness)
	clearBadTime(&e.Time)
	return nil
}

// clearUnless empties an optional field whose value pattern does not accept.
func clearUnless(pattern *regexp.Regexp, field *string) {
	if !matches(pattern, *field) {
		*field = ""
	}
}

// clearBadTime empties an optional time that is not RFC 3339 or that a
// nanosecond timestamp cannot hold: the Recorder stamps a new event, an export
// falls back to its own time.
func clearBadTime(field *string) {
	if *field == "" {
		return
	}
	if t, err := time.Parse(time.RFC3339, *field); err != nil || !nanosecondsRepresentable(t) {
		*field = ""
	}
}

func inRange(v, lo, hi float64) bool { return v >= lo && v <= hi } // false for NaN

func isKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func matches(pattern *regexp.Regexp, value string) bool {
	return value != "" && pattern.MatchString(value)
}

func hasDotDot(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// cleanRelPath accepts only a relative, forward-slash path that stays inside the
// repository. Anything else (absolute, drive letter, "..", backslash, NUL) is
// refused so an absolute path can never be exported by mistake.
func cleanRelPath(p string) (string, bool) {
	if p == "" || strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "/") || len(p) > 300 {
		return "", false
	}
	if len(p) > 1 && p[1] == ':' {
		return "", false
	}
	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// Clock supplies the time. The Recorder reads it once per event; nothing else in
// the package reads the wall clock for an event timestamp.
type Clock func() time.Time

// SystemClock is the wall clock in UTC.
func SystemClock() time.Time { return ambient.Clock(nil).Now().UTC() }

// FormatTime renders an event timestamp.
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// orderTimeLayout is RFC 3339 with milliseconds, for the consent grant and the
// cursor placement: two commands in the same second must still order.
const orderTimeLayout = "2006-01-02T15:04:05.000Z07:00"

// formatOrderTime renders a consent grant or a cursor placement time.
func formatOrderTime(t time.Time) string { return t.UTC().Format(orderTimeLayout) }

// newEventID returns 16 hex digits from a salted hash of the event's identity and
// a random nonce: distinct loads in the same second get distinct ids, while a
// replayed spool line keeps the id it was stored with.
func newEventID(salt string, e *Event) string {
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		nonce = []byte(e.Time)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{salt, e.Time, e.Kind, e.ID, e.Source, e.Session, hex.EncodeToString(nonce)}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// FromUsageEntry converts a skill usage line into an item event so the same
// exporter handles skills. The digest is the lock's canonical sha256 digest when
// the entry carries one (served loads), else the usage index's blake3 hash.
func FromUsageEntry(entry *usage.Entry) Event {
	digest, scheme := entry.Hash, ""
	if entry.Digest != "" {
		digest, scheme = entry.Digest, entry.DigestScheme
	}
	outcome := entry.Outcome
	if outcome == "" {
		outcome = OutcomeLoaded
	}
	eventID := entry.EventID
	if !matches(eventIDPattern, eventID) {
		eventID = "" // the recorder assigns one
	}
	return Event{
		Time: entry.Time, EventID: eventID, Kind: KindSkill, ID: entry.ID, Digest: digest, DigestScheme: scheme,
		Source: SourceHook, Harness: entry.Harness, Role: entry.Role, Served: entry.Served,
		Session: entry.Session, Outcome: outcome, LoadReason: entry.Invocation,
	}
}

// ToUsageEntry renders a skill event as the usage log line (the current entry
// version), keeping the log readable by every release that reads `telemetry record`
// output.
func ToUsageEntry(e *Event) usage.Entry {
	invocation := "tool"
	if e.Source == SourceMCP {
		invocation = "mcp"
	}
	entry := usage.Entry{
		Version: usage.EntrySchemaVersion, Time: e.Time, EventID: e.EventID, Event: usage.EventSkillInvoked,
		Skill: e.ID, ID: e.ID, Session: e.Session, Invocation: invocation,
		Harness: e.Harness, Outcome: e.Outcome, Served: e.Served, Role: e.Role,
	}
	if strings.HasPrefix(e.Digest, "sha256:") {
		// A load through the skills server reports the served-skill digest; the
		// canonical digest reaches the log through `telemetry record` and the index.
		entry.Digest, entry.DigestScheme = e.Digest, usage.DigestSchemeServed
		if e.DigestScheme != "" {
			entry.DigestScheme = e.DigestScheme
		}
	} else {
		entry.Hash = e.Digest // the index's blake3 hash
	}
	return entry
}
