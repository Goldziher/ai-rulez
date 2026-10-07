package testutil

import (
	"fmt"
	"strings"
	"sync"
)

// LogRecorder is a logger.Logger that keeps what it is given, so a test can say
// which logger a message went to. It is safe for concurrent use.
type LogRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *LogRecorder) record(level, msg string, args []any) {
	var b strings.Builder
	b.WriteString(level + " " + msg)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&b, " %v=%v", args[i], args[i+1])
	}
	r.mu.Lock()
	r.lines = append(r.lines, b.String())
	r.mu.Unlock()
}

// Debug implements logger.Logger.
func (r *LogRecorder) Debug(msg string, args ...any) { r.record("DEBUG", msg, args) }

// Info implements logger.Logger.
func (r *LogRecorder) Info(msg string, args ...any) { r.record("INFO", msg, args) }

// Warn implements logger.Logger.
func (r *LogRecorder) Warn(msg string, args ...any) { r.record("WARN", msg, args) }

// Error implements logger.Logger.
func (r *LogRecorder) Error(msg string, args ...any) { r.record("ERROR", msg, args) }

// Lines returns a copy of the recorded lines, "LEVEL message key=value ...".
func (r *LogRecorder) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// Level returns the recorded lines of one level (DEBUG, INFO, WARN or ERROR).
func (r *LogRecorder) Level(level string) []string {
	var out []string
	for _, l := range r.Lines() {
		if strings.HasPrefix(l, level+" ") {
			out = append(out, l)
		}
	}
	return out
}

// String joins every recorded line.
func (r *LogRecorder) String() string { return strings.Join(r.Lines(), "\n") }
