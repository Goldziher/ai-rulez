package telemetry

import (
	"context"
	"errors"
)

// Emitter receives normalized, stamped events. Implementations must not block
// the caller on the network: a slow or dead collector is the exporter's problem,
// never the harness's.
type Emitter interface {
	// Emit records one event. The event has passed Normalize and carries its
	// timestamp and event id.
	Emit(ctx context.Context, event *Event) error
	// Close flushes what it can within ctx and releases resources.
	Close(ctx context.Context) error
}

// Nop is an Emitter that discards events: the value used when telemetry is off,
// so callers never branch on nil.
type Nop struct{}

// Emit discards the event.
func (Nop) Emit(context.Context, *Event) error { return nil }

// Close does nothing.
func (Nop) Close(context.Context) error { return nil }

// Multi fans an event out to every emitter. A failing emitter does not stop the
// others; errors are joined.
type Multi []Emitter

// Emit sends the event to every emitter.
func (m Multi) Emit(ctx context.Context, event *Event) error {
	var errs []error
	for _, emitter := range m {
		if err := emitter.Emit(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close closes every emitter.
func (m Multi) Close(ctx context.Context) error {
	var errs []error
	for _, emitter := range m {
		if err := emitter.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Compose returns the cheapest Emitter for the list: Nop for none, the emitter
// itself for one, else a Multi. Nil entries are skipped.
func Compose(emitters ...Emitter) Emitter {
	var list Multi
	for _, emitter := range emitters {
		if emitter != nil {
			list = append(list, emitter)
		}
	}
	switch len(list) {
	case 0:
		return Nop{}
	case 1:
		return list[0]
	}
	return list
}
