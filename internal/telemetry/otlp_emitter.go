package telemetry

import (
	"context"
	"sync"
	"time"
)

// OTLP is the Emitter for network export. Emit only appends to the on-disk
// spool, so it returns in well under a millisecond whether or not the collector
// is reachable. Shipping happens in Exporter.Flush: from a detached process for
// short-lived hooks, or from Start's background loop for a long-lived MCP server.
type OTLP struct {
	Spool    *Spool
	Exporter *Exporter
	// Sample is the fraction of sessions exported (0..1).
	Sample float64
	// Interval is the background flush period; default 5 minutes.
	Interval time.Duration

	once sync.Once
	stop chan struct{}
	done chan struct{}
}

// DefaultFlushInterval is how often a background loop or a spawned flush ships
// a non-full spool.
const DefaultFlushInterval = 5 * time.Minute

// Emit spools the event when it is inside the sample.
func (o *OTLP) Emit(_ context.Context, event *Event) error {
	if !Sampled(o.Sample, event) {
		return nil
	}
	return o.Spool.Append(event)
}

// Start runs a background flush loop until Close. It is for long-lived processes.
func (o *OTLP) Start() {
	o.once.Do(func() {
		o.stop, o.done = make(chan struct{}), make(chan struct{})
		interval := o.Interval
		if interval <= 0 {
			interval = DefaultFlushInterval
		}
		go func() {
			defer close(o.done)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-o.stop:
					return
				case <-ticker.C:
					ctx, cancel := context.WithTimeout(context.Background(), 2*DefaultRequestLimit)
					_, _ = o.Exporter.Flush(ctx) //nolint:errcheck // failures are recorded in the state file
					cancel()
				}
			}
		}()
	})
}

// Close stops the loop and makes one last flush bounded by ctx.
func (o *OTLP) Close(ctx context.Context) error {
	if o.stop != nil {
		close(o.stop)
		<-o.done
	}
	if o.Exporter == nil {
		return nil
	}
	_, err := o.Exporter.Flush(ctx)
	return err
}
