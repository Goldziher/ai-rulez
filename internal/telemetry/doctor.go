package telemetry

import (
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// DoctorReport is what `telemetry doctor` prints: the resolved configuration and
// its sources, the gates that keep export off, and the spool state. It never
// contains the endpoint path or query, a header name or value, or an event.
type DoctorReport struct {
	Recording bool   `json:"recording"`
	Export    bool   `json:"export"`
	Killed    string `json:"kill_switch,omitempty"`
	// Blockers lists, in order, why export is off.
	Blockers []string        `json:"export_blockers,omitempty"`
	Settings []DoctorSetting `json:"settings"`
	// IgnoredRepoKeys are repository-config keys that only user scope may set.
	IgnoredRepoKeys []string `json:"ignored_repo_keys,omitempty"`
	Problems        []string `json:"problems,omitempty"`
	// EndpointHost is host[:port] only.
	EndpointHost string       `json:"endpoint_host,omitempty"`
	HeadersSet   []string     `json:"headers_env_set,omitempty"`
	HeadersUnset []string     `json:"headers_env_unset,omitempty"`
	Consent      string       `json:"consent"`
	Buffer       DoctorBuffer `json:"buffer"`
}

// DoctorSetting is one resolved key.
type DoctorSetting struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// DoctorBuffer describes the outbox.
type DoctorBuffer struct {
	Path       string `json:"path"`
	Events     int    `json:"events"`
	Bytes      int64  `json:"bytes"`
	MaxEvents  int    `json:"max_events"`
	LastFlush  string `json:"last_flush,omitempty"`
	LastStatus string `json:"last_status,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Sent       int64  `json:"sent"`
	Dropped    int64  `json:"dropped"`
	Rejected   int64  `json:"rejected"`
}

// settingKeys is the display order of the resolved keys.
var settingKeys = []string{"enabled", "allow_network", "otlp_endpoint", "otlp_protocol", "headers_env", "service_name", "resource", "sample", "include_paths", "include_session", "salt_file"}

// headersValue lists the header variable names, never their values.
func headersValue(s *Settings) string {
	if len(s.HeadersEnv) == 0 {
		return "(none)"
	}
	names := make([]string, len(s.HeadersEnv))
	for i, name := range s.HeadersEnv {
		names[i] = displayHeaderName(name)
	}
	return strings.Join(names, ",")
}

// resourceValue lists the resource attributes as sorted key=value pairs.
func resourceValue(s *Settings) string {
	if len(s.Resource) == 0 {
		return "(none)"
	}
	pairs := make([]string, 0, len(s.Resource))
	for _, key := range slices.Sorted(maps.Keys(s.Resource)) {
		pairs = append(pairs, key+"="+s.Resource[key])
	}
	return strings.Join(pairs, ",")
}

// settingValue renders one key for display: never the endpoint path, never a
// header, never a salt file path.
func settingValue(s *Settings, host, key string) string {
	switch key {
	case "enabled":
		return fmt.Sprint(s.Enabled)
	case "allow_network":
		return fmt.Sprint(s.AllowNetwork)
	case "otlp_endpoint":
		if host == "" {
			return "(unset)"
		}
		return host + " (host only)"
	case "otlp_protocol":
		return s.Protocol
	case "headers_env":
		return headersValue(s)
	case "service_name":
		return s.ServiceName
	case "resource":
		return resourceValue(s)
	case "sample":
		return fmt.Sprint(s.Sample)
	case "include_paths":
		return fmt.Sprint(s.IncludePaths)
	case "include_session":
		return fmt.Sprint(s.IncludeSession)
	case "salt_file":
		if s.SaltFile == "" {
			return "(default)"
		}
		return "(custom)"
	}
	return ""
}

func consentText(s *Settings) string {
	switch s.ConsentState {
	case ConsentRecord:
		return "granted by the consent record (" + s.Consent.GrantedAt + ") for " + endpointHost(s.Endpoint)
	case ConsentEnv:
		return "granted by " + EnvAllowNetwork
	case ConsentConfig:
		return "granted by user config (allow_network)"
	case ConsentStale:
		return "stale: " + s.ConsentDetail + "; run `ai-rulez telemetry enable` again"
	case ConsentInvalid:
		return "consent record unusable: " + s.ConsentDetail
	case ConsentDenied, ConsentPolicy:
		return "refused: " + s.ConsentDetail
	}
	return "no consent: network export is off (run `ai-rulez telemetry enable --endpoint URL`)"
}

// exportBlockers lists, in order, why export is off; nil when it is on.
func exportBlockers(s *Settings) []string {
	if s.ExportActive() {
		return nil
	}
	var out []string
	switch {
	case s.Killed != "":
		out = append(out, "kill switch: "+s.Killed)
	case !s.Enabled:
		out = append(out, "telemetry is not enabled")
	}
	if !s.AllowNetwork {
		out = append(out, noConsentBlocker(s))
	}
	if s.Endpoint == "" {
		out = append(out, "no endpoint (pass --endpoint to `ai-rulez telemetry enable`, or set otlp_endpoint in the user config)")
	}
	for _, problem := range s.blocking {
		out = append(out, "invalid: "+problem)
	}
	return out
}

// noConsentBlocker explains the missing consent in one clause.
func noConsentBlocker(s *Settings) string {
	switch s.ConsentState {
	case ConsentStale:
		return "consent is stale (" + s.ConsentDetail + "): run `ai-rulez telemetry enable` again"
	case ConsentInvalid:
		return "the consent record is unusable (" + s.ConsentDetail + ")"
	case ConsentDenied, ConsentPolicy:
		return s.ConsentDetail
	}
	return "no consent (run `ai-rulez telemetry enable`, or set allow_network in the user config)"
}

// Diagnose builds the report for the spool in dir.
func Diagnose(s *Settings, dir string, getenv func(string) string) *DoctorReport {
	r := &DoctorReport{
		Recording: s.RecordActive(), Export: s.ExportActive(), Killed: s.Killed, Problems: s.Problems,
		IgnoredRepoKeys: s.Ignored, EndpointHost: endpointHost(s.Endpoint), Consent: consentText(s), Blockers: exportBlockers(s),
	}
	for _, key := range settingKeys {
		source := s.Sources[key]
		if source == "" {
			source = ScopeDefault
		}
		r.Settings = append(r.Settings, DoctorSetting{Key: key, Value: settingValue(s, r.EndpointHost, key), Source: source})
	}
	x := Exporter{HeadersEnv: s.HeadersEnv, Getenv: getenv}
	r.HeadersSet, r.HeadersUnset = x.HeaderNamesSet()

	spool := &Spool{Dir: dir}
	events, _, _ := spool.Pending() //nolint:errcheck // an unreadable outbox reads as empty here
	st := spool.ReadState()
	r.Buffer = DoctorBuffer{
		Path: spool.outbox(), Events: len(events), Bytes: spool.Size(), MaxEvents: spool.maxEvents(),
		LastFlush: st.LastFlush, LastStatus: st.LastStatus, LastError: st.LastError, Sent: st.Sent, Dropped: st.Dropped, Rejected: st.Rejected,
	}
	return r
}

func endpointHost(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// textWriter drops write errors: a doctor report on a closed pipe is not actionable.
type textWriter struct{ w io.Writer }

func (t textWriter) printf(format string, args ...any) {
	fmt.Fprintf(t.w, format, args...) //nolint:errcheck // see textWriter
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// Render writes the report as text.
func (r *DoctorReport) Render(out io.Writer) {
	w := textWriter{out}
	w.printf("telemetry recording: %s\n", onOff(r.Recording))
	w.printf("otlp export:         %s\n", onOff(r.Export))
	if r.Killed != "" {
		w.printf("kill switch:         %s\n", r.Killed)
	}
	for _, blocker := range r.Blockers {
		w.printf("  - export off: %s\n", blocker)
	}
	w.printf("\nsettings (value, source)\n")
	width := 28
	for _, s := range r.Settings {
		width = max(width, len(s.Value))
	}
	for _, s := range r.Settings {
		w.printf("  %-16s %-*s %s\n", s.Key, width, s.Value, s.Source)
	}
	if len(r.IgnoredRepoKeys) > 0 {
		sort.Strings(r.IgnoredRepoKeys)
		w.printf("\nignored in repository config (user scope only): %s\n", strings.Join(r.IgnoredRepoKeys, ", "))
	}
	if len(r.HeadersSet)+len(r.HeadersUnset) > 0 {
		w.printf("headers_env: %d set, %d unset", len(r.HeadersSet), len(r.HeadersUnset))
		if len(r.HeadersUnset) > 0 {
			w.printf(" (unset: %s)", strings.Join(r.HeadersUnset, ", "))
		}
		w.printf("\n")
	}
	w.printf("consent: %s\n", r.Consent)
	for _, problem := range r.Problems {
		w.printf("problem: %s\n", problem)
	}
	b := r.Buffer
	w.printf("\nbuffer: %d events, %d bytes (max %d events)\n  %s\n", b.Events, b.Bytes, b.MaxEvents, b.Path)
	if b.LastFlush == "" {
		w.printf("last flush: never\n")
	} else {
		w.printf("last flush: %s (%s)\n", b.LastFlush, b.LastStatus)
	}
	if b.LastStatus != "" && b.LastStatus != "ok" {
		w.printf("last error: %s\n", b.LastError)
	}
	w.printf("totals: sent %d, dropped %d, rejected %d\n", b.Sent, b.Dropped, b.Rejected)
}

// hiddenHeaderName stands in for a headers_env entry that is not a variable name:
// it may be a credential pasted by mistake, so it is never printed.
const hiddenHeaderName = "(invalid, hidden)"

var headerEnvName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// plausibleHeaderEnvName reports whether a headers_env entry is an environment
// variable name rather than a pasted credential. An upper-case string that is
// shaped like a credential (a long access-key id with no underscore) is not
// plausible, whatever the character set allows.
func plausibleHeaderEnvName(name string) bool {
	return headerEnvName.MatchString(name) && config.ValidateTelemetryEnvName(name) == ""
}

// displayHeaderName returns name when it is a plausible environment variable
// name and the placeholder otherwise.
func displayHeaderName(name string) string {
	if plausibleHeaderEnvName(name) {
		return name
	}
	return hiddenHeaderName
}
