package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
)

// maxOTLPLine bounds one line of an OTLP file: a collector may write many batches
// to one request, so the usual line cap is too small.
const maxOTLPLine = 64 << 20

// OTLPRead is the skill-usage evidence in an OTLP JSON file.
type OTLPRead struct {
	// Entries are the skill loads, as usage log entries so the same reports read
	// them; in file order.
	Entries []usage.Entry
	// Records counts the log records seen; Ignored counts those that were not skill
	// loads (rule, agent and context loads, eval results); Skipped counts lines that
	// were not OTLP logs requests.
	Records, Ignored, Skipped int
}

type readValue struct {
	String *string  `json:"stringValue"`
	Bool   *bool    `json:"boolValue"`
	Int    *string  `json:"intValue"`
	Double *float64 `json:"doubleValue"`
}

type readAttr struct {
	Key   string    `json:"key"`
	Value readValue `json:"value"`
}

type readRecord struct {
	TimeUnixNano string     `json:"timeUnixNano"`
	Attributes   []readAttr `json:"attributes"`
}

type readRequest struct {
	ResourceLogs []struct {
		ScopeLogs []struct {
			LogRecords []readRecord `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

// ReadOTLPFile reads an OTLP JSON logs file (one request per line, what `usage
// export --to file` writes and the collector's file exporter produces) back into
// skill-usage entries. Only the attributes of the allowlist are read; a line that
// is not an OTLP logs request is counted and skipped, never an error, so a file
// that mixes signals still reads.
func ReadOTLPFile(path string) (OTLPRead, error) {
	file, err := safefs.OpenRegular(path)
	if err != nil {
		return OTLPRead{}, oops.With("path", path).Wrapf(err, "open OTLP file")
	}
	defer file.Close() //nolint:errcheck // read-only
	var out OTLPRead
	reader := bufio.NewReaderSize(file, 256*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxOTLPLine {
			return out, oops.With("path", path).Errorf("an OTLP line exceeds %d bytes", maxOTLPLine)
		}
		if line = bytes.TrimSpace(line); len(line) > 0 {
			out.readLine(line)
		}
		if errors.Is(readErr, io.EOF) {
			return out, nil
		}
		if readErr != nil {
			return out, oops.With("path", path).Wrapf(readErr, "read OTLP file")
		}
	}
}

func (r *OTLPRead) readLine(line []byte) {
	var req readRequest
	if json.Unmarshal(line, &req) != nil || len(req.ResourceLogs) == 0 {
		r.Skipped++
		return
	}
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for i := range sl.LogRecords {
				r.Records++
				if entry, ok := entryFromRecord(&sl.LogRecords[i]); ok {
					r.Entries = append(r.Entries, entry)
				} else {
					r.Ignored++
				}
			}
		}
	}
}

// entryFromRecord maps a skill-load log record to a usage entry. Values are
// re-validated with the patterns the writer applies, so a hand-edited file cannot
// inject an id or digest the recorder would have refused.
func entryFromRecord(rec *readRecord) (usage.Entry, bool) {
	attrs := make(map[string]readValue, len(rec.Attributes))
	for _, a := range rec.Attributes {
		attrs[a.Key] = a.Value
	}
	text := func(key string) string {
		if v := attrs[key].String; v != nil {
			return *v
		}
		return ""
	}
	name := text("event.name")
	if !strings.HasPrefix(name, logEventNamePrefix) || text("ai_rulez.item.kind") != KindSkill {
		return usage.Entry{}, false
	}
	id := text("ai_rulez.item.id")
	if !idPattern.MatchString(id) || hasDotDot(id) {
		return usage.Entry{}, false
	}
	entry := usage.Entry{
		Version: usage.EntrySchemaVersion, Event: usage.EventSkillInvoked, Skill: id, ID: id,
		Time:       nanosToTime(rec.TimeUnixNano),
		Outcome:    pick(knownOutcome(text("ai_rulez.outcome")), knownOutcome(strings.TrimPrefix(name, logEventNamePrefix))),
		Harness:    valid(harnessPattern, text("ai_rulez.harness")),
		Role:       valid(rolePattern, text("ai_rulez.role")),
		Session:    valid(sessionPattern, text("ai_rulez.session")),
		EventID:    valid(eventIDPattern, text("ai_rulez.event_id")),
		Invocation: valid(reasonPattern, text("ai_rulez.load_reason")),
	}
	if v := attrs["ai_rulez.served"].Bool; v != nil {
		entry.Served = *v
	}
	if digest := valid(digestPattern, text("ai_rulez.item.digest")); digest != "" {
		if strings.HasPrefix(digest, "sha256:") {
			entry.Digest = digest
			if scheme := text("ai_rulez.item.digest_scheme"); validDigestScheme(digest, scheme) {
				entry.DigestScheme = scheme
			}
		} else {
			entry.Hash = digest
		}
	}
	return entry, true
}

func valid(pattern interface{ MatchString(string) bool }, value string) string {
	if value != "" && pattern.MatchString(value) {
		return value
	}
	return ""
}

func knownOutcome(outcome string) string {
	switch outcome {
	case OutcomeLoaded, OutcomeUsed, OutcomeAbandoned:
		return outcome
	}
	return ""
}

func pick(first, fallback string) string {
	if first != "" {
		return first
	}
	return fallback
}

// nanosToTime renders OTLP nanoseconds as an RFC 3339 UTC time; "" for 0 or junk.
func nanosToTime(nano string) string {
	n, err := strconv.ParseInt(nano, 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	return FormatTime(time.Unix(0, n))
}
