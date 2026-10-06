package telemetry

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// Encoder returns the encoder the settings configure: the same one the exporter
// uses, so a preview and a file export render what a flush would send.
func (s *Settings) Encoder(version string) Encoder {
	return Encoder{
		ServiceName: s.ServiceName, ServiceVersion: version, Resource: s.Resource,
		IncludePaths: s.IncludePaths, IncludeSession: s.IncludeSession,
	}
}

// ExportBlockers lists why network export is off, nil when it is on.
func (s *Settings) ExportBlockers() []string { return exportBlockers(s) }

// LogRead is the exportable content of a usage log.
type LogRead struct {
	// Events are the valid events in file order, de-duplicated by event id.
	Events []Event
	// Rejected counts lines that were valid JSON events but failed validation
	// (an unknown kind, an invalid id) and were left out.
	Rejected int
}

// ReadLogEvents reads the events of a usage log that the exporter can send: skill
// lines (any log version) and item events, in file order. Every event goes
// through Normalize, so a field that fails its validator is dropped and an event
// whose identity fails is left out and counted. Feedback lines, the loads of a
// served skill's supporting files and lines that are not events are ignored.
// A line without an event id (log version 2 or older) gets one derived from its
// text, so exporting the same log twice yields the same ids.
func ReadLogEvents(path string) (LogRead, error) {
	file, err := safefs.OpenRegular(path)
	if err != nil {
		return LogRead{}, oops.With("path", path).Wrapf(err, "open usage log")
	}
	defer file.Close() //nolint:errcheck // read-only

	var out LogRead
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		event, ok := decodeLogLine(line)
		if !ok {
			continue
		}
		if event.EventID == "" {
			sum := sha256.Sum256(line)
			event.EventID = hex.EncodeToString(sum[:8])
		}
		if err := event.Normalize(); err != nil {
			out.Rejected++
			continue
		}
		if seen[event.EventID] {
			continue
		}
		seen[event.EventID] = true
		out.Events = append(out.Events, event)
	}
	return out, oops.Wrapf(scanner.Err(), "read usage log")
}

// decodeLogLine turns one log line into an event; false for a line that is not
// an exportable event.
func decodeLogLine(line []byte) (Event, bool) {
	if len(line) == 0 {
		return Event{}, false
	}
	var probe struct {
		Event string `json:"event"`
	}
	if json.Unmarshal(line, &probe) != nil {
		return Event{}, false
	}
	switch probe.Event {
	case usage.EventSkillInvoked:
		var entry usage.Entry
		if json.Unmarshal(line, &entry) != nil || entry.ID == "" || entry.Resource {
			return Event{}, false
		}
		return FromUsageEntry(&entry), true
	case EventItem:
		var event Event
		if json.Unmarshal(line, &event) != nil || event.ID == "" || event.Kind == "" {
			return Event{}, false
		}
		return event, true
	}
	return Event{}, false
}

// Request is one OTLP/HTTP request the exporter would make.
type Request struct {
	// Path is the signal path appended to the endpoint: /v1/logs or /v1/metrics.
	Path string
	// Body is the exact JSON body, before gzip.
	Body []byte
	// GzipBytes is the size of Body as sent, gzip-compressed.
	GzipBytes int
	// Events is the number of events the request covers.
	Events int
}

// Paths of the OTLP signals.
const (
	PathLogs    = "/v1/logs"
	PathMetrics = "/v1/metrics"
)

// Plan encodes events into the requests an export would make, in batches of
// batchMax (DefaultBatchMax when not positive): per batch one logs request and,
// with metrics set, the metrics request when the batch yields one. Nothing is
// sent. The observation time is the newest event time, so the same events always
// encode to the same bytes.
func (en *Encoder) Plan(events []Event, batchMax int, metrics bool) ([]Request, error) {
	if batchMax <= 0 {
		batchMax = DefaultBatchMax
	}
	observed := newestTime(events)
	var out []Request
	for start := 0; start < len(events); start += batchMax {
		batch := events[start:min(start+batchMax, len(events))]
		logs, err := en.EncodeLogs(batch, observed)
		if err != nil {
			return nil, oops.Wrapf(err, "encode logs")
		}
		request, err := newRequest(PathLogs, logs, len(batch))
		if err != nil {
			return nil, err
		}
		out = append(out, request)
		if !metrics {
			continue
		}
		body, err := en.EncodeMetrics(batch, observed)
		if err != nil {
			return nil, oops.Wrapf(err, "encode metrics")
		}
		if body == nil {
			continue
		}
		if request, err = newRequest(PathMetrics, body, len(batch)); err != nil {
			return nil, err
		}
		out = append(out, request)
	}
	return out, nil
}

func newRequest(path string, body []byte, events int) (Request, error) {
	compressed, err := gzipBytes(body)
	if err != nil {
		return Request{}, err
	}
	return Request{Path: path, Body: body, GzipBytes: len(compressed), Events: events}, nil
}

// newestTime is the latest parseable event time, the zero time for none.
func newestTime(events []Event) time.Time {
	var newest time.Time
	for i := range events {
		if t, err := time.Parse(time.RFC3339, events[i].Time); err == nil && t.After(newest) {
			newest = t
		}
	}
	return newest
}

// Fields lists the allowlist attribute names the encoder emits and those it
// holds back because their opt-in is closed, each sorted as the allowlist is
// ordered. event.name is always emitted.
func (en *Encoder) Fields() (exported, withheld []string) {
	exported = []string{"event.name"}
	for _, a := range Allowlist {
		if en.open(a.Gate) {
			exported = append(exported, a.Name)
		} else {
			withheld = append(withheld, a.Name)
		}
	}
	return exported, withheld
}

// DisplayURL renders where a request would go without anything that could carry
// a credential: scheme, host and path of the endpoint plus the signal path; the
// user info and query are dropped. "" when the endpoint is not a valid URL.
func DisplayURL(endpoint, path string) string {
	parsed, err := url.Parse(strings.TrimRight(endpoint, "/"))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/") + path
}
