package telemetry

import "bytes"

// FileExport is the content of an OTLP JSON file export.
type FileExport struct {
	// Data is the file content: one logs request per line, each line the JSON an
	// OTLP/HTTP POST to /v1/logs would carry. It is the format the OpenTelemetry
	// Collector's otlpjsonfile receiver reads for logs. Empty for no events.
	Data []byte
	// Events and Batches count what the file holds.
	Events, Batches int
}

// EncodeFile renders events as an OTLP JSON file: logs requests only, batches of
// DefaultBatchMax events, no metrics (the receiver reads one signal per file and
// a backend can derive the counts from the logs). The same events always give the
// same bytes: the observation time is the newest event time, nothing reads the
// clock.
func (en *Encoder) EncodeFile(events []Event) (FileExport, error) {
	plan, err := en.Plan(events, DefaultBatchMax, false)
	if err != nil {
		return FileExport{}, err
	}
	var buf bytes.Buffer
	for i := range plan {
		buf.Write(plan[i].Body)
		buf.WriteByte('\n')
	}
	return FileExport{Data: buf.Bytes(), Events: len(events), Batches: len(plan)}, nil
}
