package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeFile(t *testing.T) {
	t.Run("should write one logs request per line, golden and deterministic", func(t *testing.T) {
		read, err := ReadLogEvents(writeLog(t, previewLog))
		require.NoError(t, err)
		en := Encoder{ServiceName: "ai-rulez", ServiceVersion: "5.0.0"}

		first, err := en.EncodeFile(read.Events)
		require.NoError(t, err)
		second, err := en.EncodeFile(read.Events)
		require.NoError(t, err)

		assert.Equal(t, first, second)
		assert.Equal(t, 4, first.Events)
		assert.Equal(t, 1, first.Batches)
		golden(t, "export_file.golden.ndjson", first.Data)
	})

	t.Run("should split batches of the exporter's size", func(t *testing.T) {
		events := make([]Event, DefaultBatchMax+1)
		for i := range events {
			events[i] = Event{Time: "2026-10-05T09:12:40Z", EventID: fmt.Sprintf("%016x", i), Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}
		}

		got, err := (&Encoder{}).EncodeFile(events)

		require.NoError(t, err)
		assert.Equal(t, 2, got.Batches)
		lines := bytes.Split(bytes.TrimSuffix(got.Data, []byte("\n")), []byte("\n"))
		require.Len(t, lines, 2)
		for _, line := range lines {
			assert.True(t, json.Valid(line))
			assert.Contains(t, string(line), `"resourceLogs"`)
			assert.NotContains(t, string(line), `"resourceMetrics"`)
		}
	})

	t.Run("should write nothing for no events", func(t *testing.T) {
		got, err := (&Encoder{}).EncodeFile(nil)

		require.NoError(t, err)
		assert.Empty(t, got.Data)
		assert.Zero(t, got.Batches)
	})
}
