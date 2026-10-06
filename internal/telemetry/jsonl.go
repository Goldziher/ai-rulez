package telemetry

import (
	"context"
	"encoding/json"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
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
	return safefs.AppendLine(path, line) //nolint:wrapcheck // safefs errors carry the path
}
