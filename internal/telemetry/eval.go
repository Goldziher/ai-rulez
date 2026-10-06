package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// EvalResult is the slice of a recorded eval run that an export carries: ids,
// the canonical digest the run covered, the harness it ran on and the scores.
// The eval package owns the full record (cases, outputs, cost); none of that is
// here, so none of it can be exported.
type EvalResult struct {
	SkillID string
	// LockDigest is the canonical skill digest (usage.DigestSchemeSkill) the run
	// covered; empty for a record written before it existed, which then exports with
	// no digest and joins by id only.
	LockDigest string
	Harness    string
	// Date is the run date as the eval store holds it: YYYY-MM-DD or RFC 3339.
	Date             string
	PassRate         float64
	TriggerPrecision *float64
	TriggerRecall    *float64
	AblationDelta    *float64
}

// EvalEvents turns eval results into eval_result events, one per result, in the
// order given. An event is built from the store, not recorded, so its id is a
// function of what it says: the same result always has the same id, and re-running
// an export (or a catch-up) does not make a backend count it twice. Results whose
// identity or pass rate fail validation are left out and counted.
func EvalEvents(results []EvalResult) (events []Event, rejected int) {
	for i := range results {
		r := &results[i]
		pass := r.PassRate
		e := Event{
			Name: EventEvalResult, Time: storeDate(r.Date), Kind: KindSkill, ID: r.SkillID,
			Digest: r.LockDigest, DigestScheme: usage.DigestSchemeSkill, Harness: r.Harness, Source: SourceCLI,
			PassRate: &pass, TriggerPrecision: r.TriggerPrecision, TriggerRecall: r.TriggerRecall, AblationDelta: r.AblationDelta,
		}
		if r.LockDigest == "" {
			e.DigestScheme = ""
		}
		if err := e.Normalize(); err != nil {
			rejected++
			continue
		}
		e.EventID = evalEventID(&e)
		events = append(events, e)
	}
	return events, rejected
}

// storeDate renders a store date as an RFC 3339 time; "" when it is neither a date
// nor a time.
func storeDate(date string) string {
	date = strings.TrimSpace(date)
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		return FormatTime(t)
	}
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return FormatTime(t)
	}
	return ""
}

func evalEventID(e *Event) string {
	parts := []string{"eval", e.ID, e.Digest, e.Harness, e.Time, floatText(e.PassRate), floatText(e.TriggerPrecision), floatText(e.TriggerRecall), floatText(e.AblationDelta)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func floatText(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'g', -1, 64)
}

// Unsent drops the events the outbox already holds or the cursor remembers as
// delivered, so a repeated export of the same eval results queues each once.
func (s *Spool) Unsent(events []Event) ([]Event, error) {
	known, err := s.knownIDs(s.ReadCursor())
	if err != nil {
		return nil, err
	}
	out := events[:0:0]
	for i := range events {
		if !known[events[i].EventID] {
			out = append(out, events[i])
		}
	}
	return out, nil
}
