package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// JSONL appends events to the local usage log. A skill event is written as the
// v2 usage line `usage record` writes, so every reader of that log keeps working;
// the other kinds are written as item events (event "item_event", field "v").
type JSONL struct {
	Path string
}

// Emit appends one line. The whole line goes out in a single write so concurrent
// sessions do not interleave.
func (j JSONL) Emit(_ context.Context, event *Event) error {
	var (
		line []byte
		err  error
	)
	if event.Kind == KindSkill {
		entry := ToUsageEntry(event)
		line, err = json.Marshal(entry)
	} else {
		line, err = json.Marshal(event)
	}
	if err != nil {
		return oops.Wrapf(err, "encode telemetry event")
	}
	return appendLine(j.Path, append(line, '\n'))
}

// Close does nothing: the file is opened per append.
func (JSONL) Close(context.Context) error { return nil }

func appendLine(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return oops.With("path", path).Wrapf(err, "create telemetry directory")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // machine-local log chosen by the user
	if err != nil {
		return oops.With("path", path).Wrapf(err, "open telemetry log")
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close() //nolint:errcheck // the write error is the one to report
		return oops.With("path", path).Wrapf(err, "write telemetry log")
	}
	return oops.Wrapf(file.Close(), "close telemetry log")
}
