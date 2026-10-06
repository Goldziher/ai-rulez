package telemetry

import (
	"fmt"
	"io"
	"strings"
)

// Status is what `telemetry status` prints: a short answer to "is anything being
// sent, to where, under whose consent, and is it working". `telemetry doctor`
// keeps the long form (every setting and its source).
type Status struct {
	Recording bool   `json:"recording"`
	Export    bool   `json:"export"`
	Killed    string `json:"kill_switch,omitempty"`
	// Blockers lists why export is off, in order.
	Blockers     []string      `json:"export_blockers,omitempty"`
	EndpointHost string        `json:"endpoint_host,omitempty"`
	Protocol     string        `json:"protocol"`
	Consent      ConsentStatus `json:"consent"`
	// Exported and Withheld are the allowlist attributes that would and would not be sent.
	Exported []string       `json:"fields_exported"`
	Withheld []string       `json:"fields_withheld,omitempty"`
	Pending  PendingStatus  `json:"pending"`
	Cursor   CursorStatus   `json:"cursor"`
	Delivery DeliveryStatus `json:"delivery"`
	Problems []string       `json:"problems,omitempty"`
}

// ConsentStatus describes who granted network export, if anyone.
type ConsentStatus struct {
	// State is one of the Consent* constants.
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	// GrantedAt, Endpoint and Protocol come from the stored record when there is one.
	GrantedAt string `json:"granted_at,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
}

// PendingStatus counts what has not been delivered.
type PendingStatus struct {
	// Outbox is the number of events queued for the next flush.
	Outbox int `json:"outbox"`
	// Log is the number of usage-log events past the cursor (capped; LogCapped says so).
	Log       int  `json:"log"`
	LogCapped bool `json:"log_capped,omitempty"`
}

// CursorStatus is the export cursor.
type CursorStatus struct {
	Set         bool   `json:"set"`
	Offset      int64  `json:"offset,omitempty"`
	LastEventID string `json:"last_event_id,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// DeliveryStatus is the exporter's bookkeeping. Failures are silent to the
// harness by design, so this is the only place they show.
type DeliveryStatus struct {
	LastFlush           string `json:"last_flush,omitempty"`
	LastStatus          string `json:"last_status,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	Sent                int64  `json:"sent"`
	Dropped             int64  `json:"dropped"`
	Rejected            int64  `json:"rejected"`
	Failures            int64  `json:"failures"`
	ConsecutiveFailures int64  `json:"consecutive_failures"`
}

// statusLogCap bounds the log scan `status` does.
const statusLogCap = 100000

// BuildStatus reads the spool in dir and the usage log at logPath. Nothing is
// written: a status never moves the cursor and never touches the network.
func BuildStatus(s *Settings, dir, logPath string) *Status {
	st := &Status{
		Recording: s.RecordActive(), Export: s.ExportActive(), Killed: s.Killed, Blockers: exportBlockers(s),
		EndpointHost: endpointHost(s.Endpoint), Protocol: s.Protocol, Problems: s.Problems,
		Consent: ConsentStatus{State: s.ConsentState, Detail: s.ConsentDetail},
	}
	if s.Consent != nil {
		st.Consent.GrantedAt, st.Consent.Endpoint = s.Consent.GrantedAt, endpointHost(s.Consent.Endpoint)
	}
	encoder := s.Encoder("")
	st.Exported, st.Withheld = encoder.Fields()

	spool := &Spool{Dir: dir}
	if pending, _, err := spool.Pending(); err == nil {
		st.Pending.Outbox = len(pending)
	}
	cur := spool.ReadCursor()
	st.Cursor = CursorStatus{Set: cur.Set(), Offset: cur.Offset, LastEventID: cur.LastEventID, UpdatedAt: cur.UpdatedAt}
	if cur.Set() && logPath != "" {
		if n, err := spool.PendingInLog(logPath, statusLogCap); err == nil {
			st.Pending.Log = n
			st.Pending.LogCapped = n >= statusLogCap
		}
	}
	state := spool.ReadState()
	st.Delivery = DeliveryStatus{
		LastFlush: state.LastFlush, LastStatus: state.LastStatus, LastError: state.LastError, Sent: state.Sent,
		Dropped: state.Dropped, Rejected: state.Rejected, Failures: state.Failures, ConsecutiveFailures: state.ConsecutiveFailures,
	}
	return st
}

// Render writes the status as text.
func (st *Status) Render(out io.Writer) {
	w := textWriter{out}
	switch {
	case st.Export:
		w.printf("telemetry: export on, recording on\n")
	case st.Recording:
		w.printf("telemetry: recording on, export off\n")
	default:
		w.printf("telemetry: off\n")
	}
	if st.Killed != "" {
		w.printf("  kill switch: %s (nothing is recorded or sent)\n", st.Killed)
	}
	for _, blocker := range st.Blockers {
		if st.Export {
			break
		}
		w.printf("  export off: %s\n", blocker)
	}
	w.printf("consent:   %s\n", consentLine(&st.Consent))
	if st.EndpointHost != "" {
		w.printf("endpoint:  %s (%s, host only)\n", st.EndpointHost, st.Protocol)
	}
	w.printf("exports:   %s\n", strings.Join(st.Exported, ", "))
	if len(st.Withheld) > 0 {
		w.printf("withheld:  %s\n", strings.Join(st.Withheld, ", "))
	}
	w.printf("pending:   %d in the outbox, %d in the usage log past the cursor%s\n", st.Pending.Outbox, st.Pending.Log, ifCapped(st.Pending.LogCapped))
	if st.Cursor.Set {
		w.printf("cursor:    offset %d (updated %s)\n", st.Cursor.Offset, orDash(st.Cursor.UpdatedAt))
	} else {
		w.printf("cursor:    not set (the first flush places it at the end of the log; history is exported only on request)\n")
	}
	d := st.Delivery
	if d.LastFlush == "" {
		w.printf("last flush: never")
	} else {
		w.printf("last flush: %s", d.LastFlush)
	}
	if d.LastStatus != "" {
		w.printf(" (%s)", d.LastStatus)
	}
	w.printf("\n")
	w.printf("totals:    sent %d, dropped %d, rejected %d, failed flushes %d (%d in a row)\n", d.Sent, d.Dropped, d.Rejected, d.Failures, d.ConsecutiveFailures)
	if d.LastStatus != "" && d.LastStatus != "ok" && d.LastError != "" {
		w.printf("last error: %s\n", d.LastError)
	}
	for _, problem := range st.Problems {
		w.printf("problem:   %s\n", problem)
	}
}

func consentLine(c *ConsentStatus) string {
	switch c.State {
	case ConsentRecord:
		return fmt.Sprintf("granted by the consent record (%s) for %s", orDash(c.GrantedAt), orDash(c.Endpoint))
	case ConsentConfig:
		return "granted by allow_network in the user config"
	case ConsentEnv:
		return "granted by " + EnvAllowNetwork
	case ConsentStale:
		return "stale: " + c.Detail + " -> run `ai-rulez telemetry enable` again"
	case ConsentInvalid:
		return "record unusable: " + c.Detail
	case ConsentDenied, ConsentPolicy:
		return "refused: " + c.Detail
	}
	return "none -> run `ai-rulez telemetry enable --endpoint URL`"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ifCapped(capped bool) string {
	if capped {
		return " or more"
	}
	return ""
}
