package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Protocols accepted by [telemetry] otlp_protocol. Only OTLP/HTTP with a JSON
// body is implemented; the others are reserved names that validation rejects
// with a clear message rather than silently falling back.
const (
	TelemetryProtocolHTTPJSON     = "http/json"
	TelemetryProtocolHTTPProtobuf = "http/protobuf"
	TelemetryProtocolGRPC         = "grpc"
)

// TelemetryConfig configures item-load telemetry: identifier-only events about
// which skills, rules, agents and context files a harness loaded. Everything is
// off by default. Local JSONL recording needs only Enabled; network export also
// needs AllowNetwork and an endpoint, and a repository config cannot grant
// either (see internal/telemetry and docs/telemetry.md for the trust rule).
type TelemetryConfig struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
	// AllowNetwork is the explicit consent for OTLP export. User scope only.
	AllowNetwork bool `yaml:"allow_network,omitempty" json:"allow_network,omitempty" toml:"allow_network,omitempty"` //nolint:tagliatelle
	// OTLPEndpoint is the collector base URL ("https://collector:4318"); /v1/logs
	// and /v1/metrics are appended. User scope only.
	OTLPEndpoint string `yaml:"otlp_endpoint,omitempty" json:"otlp_endpoint,omitempty" toml:"otlp_endpoint,omitempty"` //nolint:tagliatelle
	// OTLPProtocol is "http/json" (the default and only implemented value).
	OTLPProtocol string `yaml:"otlp_protocol,omitempty" json:"otlp_protocol,omitempty" toml:"otlp_protocol,omitempty"` //nolint:tagliatelle
	// HeadersEnv names environment variables whose values are "k=v,k2=v2" header
	// lists. Names only: a literal credential is rejected. User scope only.
	HeadersEnv []string `yaml:"headers_env,omitempty" json:"headers_env,omitempty" toml:"headers_env,omitempty"` //nolint:tagliatelle
	// ServiceName is the service.name resource attribute (default "ai-rulez").
	ServiceName string `yaml:"service_name,omitempty" json:"service_name,omitempty" toml:"service_name,omitempty"` //nolint:tagliatelle
	// Sample is the fraction of sessions exported, 0..1 (default 1). Sampling is
	// per salted session hash, so one session is wholly in or out.
	Sample *float64 `yaml:"sample,omitempty" json:"sample,omitempty" toml:"sample,omitempty"`
	// IncludePaths adds the repo-relative path of a loaded item to exported
	// events. Default false. User scope only.
	IncludePaths bool `yaml:"include_paths,omitempty" json:"include_paths,omitempty" toml:"include_paths,omitempty"` //nolint:tagliatelle
	// SaltFile is the file holding the session-hash salt. User scope only.
	SaltFile string `yaml:"salt_file,omitempty" json:"salt_file,omitempty" toml:"salt_file,omitempty"` //nolint:tagliatelle
}

var (
	telemetryEnvName     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	telemetryServiceName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
)

// Validate returns one message per problem in the telemetry settings; nil when
// the section is valid. It checks shape only: the trust rule (which scope may
// set what) is applied by internal/telemetry.
func (t *TelemetryConfig) Validate() []string {
	if t == nil {
		return nil
	}
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	switch t.OTLPProtocol {
	case "", TelemetryProtocolHTTPJSON:
	case TelemetryProtocolHTTPProtobuf, TelemetryProtocolGRPC:
		add("telemetry.otlp_protocol: %q is not implemented (only %q)", t.OTLPProtocol, TelemetryProtocolHTTPJSON)
	default:
		add("telemetry.otlp_protocol: unknown value %q (use %q)", t.OTLPProtocol, TelemetryProtocolHTTPJSON)
	}
	if t.OTLPEndpoint != "" {
		if problem := ValidateTelemetryEndpoint(t.OTLPEndpoint); problem != "" {
			add("telemetry.otlp_endpoint: %s", problem)
		}
	}
	for _, name := range t.HeadersEnv {
		if problem := ValidateTelemetryEnvName(name); problem != "" {
			add("telemetry.headers_env: %s", problem)
		}
	}
	if t.ServiceName != "" && !telemetryServiceName.MatchString(t.ServiceName) {
		add("telemetry.service_name: %q must match %s", t.ServiceName, telemetryServiceName)
	}
	if t.Sample != nil && (*t.Sample < 0 || *t.Sample > 1) {
		add("telemetry.sample: %v must be between 0 and 1", *t.Sample)
	}
	if strings.ContainsRune(t.SaltFile, 0) {
		add("telemetry.salt_file: contains a NUL byte")
	}
	return problems
}

// ValidateTelemetryEndpoint returns a problem description for an endpoint that is
// not an http(s) URL without credentials, or that is plain http to a non-loopback
// host. Empty means acceptable.
func ValidateTelemetryEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "not a URL with a host"
	}
	if parsed.User != nil {
		return "must not embed credentials (use headers_env)"
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "must not carry a query or fragment"
	}
	switch parsed.Scheme {
	case "https":
		return ""
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return ""
		}
		return "plain http is only allowed for a loopback host (use https)"
	}
	return "scheme must be https (or http for loopback)"
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateTelemetryEnvName rejects anything that is not an environment variable
// NAME, including values that look like a pasted credential: an upper-case name
// of 20 or more characters with no underscore (an access-key id), or any string
// with lower case, dashes, spaces or an equals sign (a token, "Bearer x", "k=v").
func ValidateTelemetryEnvName(name string) string {
	if !telemetryEnvName.MatchString(name) {
		return "must be an environment variable name like OTLP_HEADERS; a literal header or credential is not accepted (value hidden)"
	}
	if len(name) >= 20 && !strings.Contains(name, "_") {
		return "looks like a literal credential, not an environment variable name (value hidden)"
	}
	return ""
}
