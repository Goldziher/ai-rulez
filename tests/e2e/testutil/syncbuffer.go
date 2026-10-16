package testutil

import (
	"bytes"
	"fmt"
	"sync"
)

// SyncBuffer is a bytes.Buffer safe for the process that writes a child's standard error and the
// test goroutine that reads it for a failure message.
type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p.
func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // a bytes.Buffer write never fails
}

// Notef appends a formatted line to the buffer; there is nothing to report if it cannot.
func (b *SyncBuffer) Notef(format string, args ...any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fmt.Fprintf(&b.buf, format, args...) //nolint:errcheck // a bytes.Buffer write never fails
}

// String returns what was written so far.
func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
