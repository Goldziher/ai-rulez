package telemetry

import "context"

// Recorder validates and stamps events and hands them to an Emitter. It is the
// only place that reads the clock for an event timestamp and that assigns event
// ids, so a test injects one Clock and gets deterministic output.
type Recorder struct {
	Emitter Emitter
	Clock   Clock
	// Salt seeds event ids; it is the same machine salt that hashes sessions.
	Salt string
}

// Record normalizes e, stamps it when the caller has not, and emits it.
func (r *Recorder) Record(ctx context.Context, e Event) error {
	if err := e.Normalize(); err != nil {
		return err
	}
	if e.Time == "" {
		clock := r.Clock
		if clock == nil {
			clock = SystemClock
		}
		e.Time = FormatTime(clock())
	}
	if e.EventID == "" {
		e.EventID = newEventID(r.Salt, &e)
	}
	emitter := r.Emitter
	if emitter == nil {
		emitter = Nop{}
	}
	return emitter.Emit(ctx, &e)
}
