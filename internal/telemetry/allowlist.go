package telemetry

import "strings"

// Event JSON field names, shared by the allowlist and the encoder.
const (
	fieldKind       = "kind"
	fieldID         = "id"
	fieldDigest     = "digest"
	fieldPath       = "path"
	fieldSource     = "source"
	fieldHarness    = "harness"
	fieldRole       = "role"
	fieldServed     = "served"
	fieldSession    = "session"
	fieldOutcome    = "outcome"
	fieldLoadReason = "load_reason"
	fieldMemoryType = "memory_type"
	fieldDuration   = "duration_ms"
	fieldEventID    = "event_id"
)

// Attr is one row of the export allowlist: the single place that decides which
// event fields can leave the machine and under what name. The encoder builds
// payloads only from this table; a field with no row cannot be exported.
type Attr struct {
	// Name is the OTLP log attribute name.
	Name string
	// Field is the Event JSON field it reads.
	Field string
	// Metric is the metric label name; empty when the field is not a label.
	Metric string
	// Gate names the opt-in that must be on for the field to be exported, "" when
	// it is always exported.
	Gate string
}

// Gates that open optional attributes.
const (
	GatePaths   = "include_paths"
	GateSession = "include_session"
)

// Allowlist is the closed set of exported event fields. docs/telemetry.md mirrors
// it and a test fails if either drifts.
var Allowlist = []Attr{
	{Name: "ai_rulez.item.kind", Field: fieldKind, Metric: fieldKind},
	{Name: "ai_rulez.item.id", Field: fieldID, Metric: fieldID},
	{Name: "ai_rulez.item.digest", Field: fieldDigest, Metric: "digest_short"},
	{Name: "ai_rulez.item.path", Field: fieldPath, Gate: GatePaths},
	{Name: "ai_rulez.source", Field: fieldSource},
	{Name: "ai_rulez.harness", Field: fieldHarness, Metric: fieldHarness},
	{Name: "ai_rulez.role", Field: fieldRole, Metric: fieldRole},
	{Name: "ai_rulez.served", Field: fieldServed, Metric: fieldServed},
	{Name: "ai_rulez.session", Field: fieldSession, Gate: GateSession},
	{Name: "ai_rulez.outcome", Field: fieldOutcome, Metric: fieldOutcome},
	{Name: "ai_rulez.load_reason", Field: fieldLoadReason},
	{Name: "ai_rulez.memory_type", Field: fieldMemoryType},
	{Name: "ai_rulez.duration_ms", Field: fieldDuration},
	{Name: "ai_rulez.event_id", Field: fieldEventID},
}

// Mapped names the event fields that are exported through the record envelope
// rather than as attributes: the timestamp and the event name (derived from the
// outcome). "v" is exported as the ai_rulez.schema_version resource attribute.
var Mapped = []string{"ts", "event", "v"}

// digestShort is the metric label form of a digest: the first 12 hex digits
// after the scheme prefix, so a label stays low-cardinality per skill version.
func digestShort(digest string) string {
	if i := strings.IndexByte(digest, ':'); i >= 0 {
		digest = digest[i+1:]
	}
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}
