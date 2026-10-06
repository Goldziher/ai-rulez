package telemetry

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Environment variables. Setting any of them is user-scope by definition: a
// repository cannot set a variable in your shell.
const (
	EnvEnabled        = "AI_RULEZ_TELEMETRY"
	EnvEndpoint       = "AI_RULEZ_TELEMETRY_ENDPOINT"
	EnvProtocol       = "AI_RULEZ_TELEMETRY_PROTOCOL"
	EnvAllowNetwork   = "AI_RULEZ_TELEMETRY_ALLOW_NETWORK"
	EnvHeadersEnv     = "AI_RULEZ_TELEMETRY_HEADERS_ENV"
	EnvServiceName    = "AI_RULEZ_TELEMETRY_SERVICE_NAME"
	EnvSample         = "AI_RULEZ_TELEMETRY_SAMPLE"
	EnvIncludePaths   = "AI_RULEZ_TELEMETRY_INCLUDE_PATHS"
	EnvIncludeSession = "AI_RULEZ_TELEMETRY_INCLUDE_SESSION"
	EnvSaltFile       = "AI_RULEZ_TELEMETRY_SALT_FILE"
	envDoNotTrack     = "DO_NOT_TRACK"
)

// DefaultServiceName is the service.name resource attribute.
const DefaultServiceName = "ai-rulez"

// Where a setting came from, for `telemetry doctor`.
const (
	ScopeDefault = "default"
	ScopeRepo    = "repo"
	ScopeUser    = "user"
	ScopeEnv     = "env"
)

// Settings is the effective telemetry configuration after the trust rule.
type Settings struct {
	// Enabled turns local recording of item events on.
	Enabled bool
	// AllowNetwork, Endpoint, Protocol, HeadersEnv, IncludePaths, IncludeSession
	// and SaltFile can only come from user scope (user config file or env).
	AllowNetwork   bool
	Endpoint       string
	Protocol       string
	HeadersEnv     []string
	IncludePaths   bool
	IncludeSession bool
	SaltFile       string
	ServiceName    string
	// Sample is the fraction of sessions exported, 0..1.
	Sample float64
	// Killed names the kill switch in force ("AI_RULEZ_TELEMETRY=off",
	// "DO_NOT_TRACK"); when set nothing is recorded or exported.
	Killed string
	// Sources maps each key to the scope that supplied the effective value.
	Sources map[string]string
	// Ignored lists repository-config keys that were not honored.
	Ignored []string
	// Problems lists every validation failure (the AR9K0 messages), repository
	// config included.
	Problems []string
	// blocking is the subset of Problems that stops export. A repository's bad
	// value for a key it may not set is reported but does not turn off the
	// export a user opted in to.
	blocking []string
}

// RecordActive reports whether local recording is on.
func (s Settings) RecordActive() bool { return s.Enabled && s.Killed == "" }

// ExportActive reports whether OTLP export is on: every gate must be open.
func (s Settings) ExportActive() bool {
	return s.RecordActive() && s.AllowNetwork && s.Endpoint != "" && s.Protocol == config.TelemetryProtocolHTTPJSON && len(s.blocking) == 0
}

// Sources of settings. Repo is a repository's own config plus its local overlay
// (an overlay is machine-local but lives in the checkout, so it gets no more
// trust); User is the user-level config file.
type Layers struct {
	Repo *config.TelemetryConfig
	User *config.TelemetryConfig
	// Getenv reads the environment; nil means os.Getenv.
	Getenv func(string) string
}

// Resolve applies the layers in precedence order (kill switches, environment,
// user config, repository config, defaults) and the trust rule: from the
// repository only enabled and sample are honored (service_name lands on the user's collector data, so it is user scope only).
func Resolve(layers Layers) Settings {
	getenv := layers.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	s := Settings{
		Protocol: config.TelemetryProtocolHTTPJSON, ServiceName: DefaultServiceName, Sample: 1,
		Sources: map[string]string{},
	}
	set := func(key, scope string) { s.Sources[key] = scope }

	if repo := layers.Repo; repo != nil {
		s.Problems = append(s.Problems, repo.Validate()...)
		if repo.Enabled {
			s.Enabled = true
			set("enabled", ScopeRepo)
		}
		if repo.Sample != nil {
			s.Sample = *repo.Sample
			set("sample", ScopeRepo)
		}
		s.Ignored = append(s.Ignored, PrivilegedKeys(repo)...)
	}
	if user := layers.User; user != nil {
		s.addBlocking(user.Validate()...)
		applyUser(&s, user, ScopeUser)
	}
	applyEnv(&s, getenv)

	// Validate the effective result too: an invalid env override must not export.
	effective := config.TelemetryConfig{
		OTLPEndpoint: s.Endpoint, OTLPProtocol: s.Protocol, HeadersEnv: s.HeadersEnv,
		ServiceName: s.ServiceName, Sample: &s.Sample, SaltFile: s.SaltFile,
	}
	s.addBlocking(effective.Validate()...)
	return s
}

func (s *Settings) addBlocking(problems ...string) {
	for _, problem := range problems {
		if !contains(s.blocking, problem) {
			s.blocking = append(s.blocking, problem)
		}
		if !contains(s.Problems, problem) {
			s.Problems = append(s.Problems, problem)
		}
	}
}

// PrivilegedKeys names the keys a repository config sets that only user scope may.
func PrivilegedKeys(repo *config.TelemetryConfig) []string {
	var out []string
	add := func(set bool, key string) {
		if set {
			out = append(out, key)
		}
	}
	add(repo.AllowNetwork, "allow_network")
	add(repo.OTLPEndpoint != "", "otlp_endpoint")
	add(repo.OTLPProtocol != "", "otlp_protocol")
	add(len(repo.HeadersEnv) > 0, "headers_env")
	add(repo.IncludePaths, "include_paths")
	add(repo.IncludeSession, "include_session")
	add(repo.SaltFile != "", "salt_file")
	add(repo.ServiceName != "", "service_name")
	return out
}

func applyUser(s *Settings, user *config.TelemetryConfig, scope string) {
	set := func(key string) { s.Sources[key] = scope }
	if user.Enabled {
		s.Enabled = true
		set("enabled")
	}
	if user.AllowNetwork {
		s.AllowNetwork = true
		set("allow_network")
	}
	if user.OTLPEndpoint != "" {
		s.Endpoint = strings.TrimRight(user.OTLPEndpoint, "/")
		set("otlp_endpoint")
	}
	if user.OTLPProtocol != "" {
		s.Protocol = user.OTLPProtocol
		set("otlp_protocol")
	}
	if len(user.HeadersEnv) > 0 {
		s.HeadersEnv = append([]string(nil), user.HeadersEnv...)
		set("headers_env")
	}
	if user.ServiceName != "" {
		s.ServiceName = user.ServiceName
		set("service_name")
	}
	if user.Sample != nil {
		s.Sample = *user.Sample
		set("sample")
	}
	if user.IncludePaths {
		s.IncludePaths = true
		set("include_paths")
	}
	if user.IncludeSession {
		s.IncludeSession = true
		set("include_session")
	}
	if user.SaltFile != "" {
		s.SaltFile = user.SaltFile
		set("salt_file")
	}
}

func applyEnv(s *Settings, getenv func(string) string) {
	set := func(key string) { s.Sources[key] = ScopeEnv }
	if isTruthy(getenv(envDoNotTrack)) {
		s.Killed = envDoNotTrack
	}
	if raw := getenv(EnvEnabled); raw != "" {
		switch {
		case isTruthy(raw):
			s.Enabled = true
			set("enabled")
		case isFalsy(raw):
			s.Killed = EnvEnabled + "=off"
		}
	}
	if v := getenv(EnvEndpoint); v != "" {
		s.Endpoint = strings.TrimRight(v, "/")
		set("otlp_endpoint")
	}
	if v := getenv(EnvProtocol); v != "" {
		s.Protocol = v
		set("otlp_protocol")
	}
	if v := getenv(EnvAllowNetwork); v != "" {
		s.AllowNetwork = isTruthy(v)
		set("allow_network")
	}
	if v := getenv(EnvHeadersEnv); v != "" {
		s.HeadersEnv = splitList(v)
		set("headers_env")
	}
	if v := getenv(EnvServiceName); v != "" {
		s.ServiceName = v
		set("service_name")
	}
	if v := getenv(EnvSample); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			s.Sample = f
			set("sample")
		} else {
			s.addBlocking("AI_RULEZ_TELEMETRY_SAMPLE: not a number")
		}
	}
	if v := getenv(EnvIncludePaths); v != "" {
		s.IncludePaths = isTruthy(v)
		set("include_paths")
	}
	if v := getenv(EnvIncludeSession); v != "" {
		s.IncludeSession = isTruthy(v)
		set("include_session")
	}
	if v := getenv(EnvSaltFile); v != "" {
		s.SaltFile = v
		set("salt_file")
	}
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func isFalsy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// LoadUser reads the [telemetry] table of the user config file. A missing file
// or table is not an error.
func LoadUser(getenv func(string) string) (*config.TelemetryConfig, string, error) {
	path := config.UserConfigFile(getenv)
	if path == "" {
		return nil, "", nil
	}
	cfg, err := loadTable([]string{path})
	return cfg, path, err
}

// LoadRepo reads the [telemetry] table of a project's config and, over it, of its
// local overlay. Both count as repository scope. Only that table is decoded, so
// the call is cheap enough for a hook.
func LoadRepo(root, configDirName string) (*config.TelemetryConfig, error) {
	dir := filepath.Join(root, configDirName)
	var paths []string
	for _, ext := range []string{"toml", "yaml", "yml", "json"} {
		if _, err := os.Stat(filepath.Join(dir, "config."+ext)); err == nil {
			paths = append(paths, filepath.Join(dir, "config."+ext))
			break
		}
	}
	for _, ext := range []string{"toml", "yaml", "yml", "json"} {
		if _, err := os.Stat(filepath.Join(dir, "config.local."+ext)); err == nil {
			paths = append(paths, filepath.Join(dir, "config.local."+ext))
			break
		}
	}
	return loadTable(paths)
}

// loadTable merges the [telemetry] table of each file in order (later wins per
// key) and decodes it strictly: an unknown key is an error, not silence.
func loadTable(paths []string) (*config.TelemetryConfig, error) {
	merged := map[string]any{}
	found := false
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // a config file the loader would read anyway
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, oops.With("path", path).Wrapf(err, "read config")
		}
		var doc map[string]any
		if strings.HasSuffix(path, ".toml") {
			err = toml.Unmarshal(data, &doc)
		} else {
			err = yaml.Unmarshal(data, &doc)
		}
		if err != nil {
			return nil, oops.With("path", path).Wrapf(err, "parse config")
		}
		table, ok := doc["telemetry"].(map[string]any)
		if !ok {
			continue
		}
		found = true
		for key, value := range table {
			merged[key] = value
		}
	}
	if !found {
		return nil, nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, oops.Wrapf(err, "encode telemetry table")
	}
	var cfg config.TelemetryConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, oops.Hint("Check the [telemetry] keys against docs/telemetry.md").Wrapf(err, "invalid [telemetry] table")
	}
	return &cfg, nil
}

// ResolveFor loads the user and repository layers for a project root and resolves
// them against the real environment. A table that fails to decode becomes a
// Problem (telemetry stays off for export) instead of an error, so a typo in a
// config never breaks a hook.
func ResolveFor(root, configDirName string, getenv func(string) string) Settings {
	user, _, userErr := LoadUser(getenv)
	repo, repoErr := LoadRepo(root, configDirName)
	s := Resolve(Layers{Repo: repo, User: user, Getenv: getenv})
	if userErr != nil {
		s.addBlocking("user config: " + firstLine(userErr.Error()))
	}
	if repoErr != nil {
		s.Problems = append(s.Problems, "repository config: "+firstLine(repoErr.Error()))
	}
	return s
}

func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}
