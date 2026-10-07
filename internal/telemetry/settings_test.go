package telemetry

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(pairs ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestResolve_DefaultsAreOff(t *testing.T) {
	s := Resolve(Layers{Getenv: env()})
	assert.False(t, s.RecordActive())
	assert.False(t, s.ExportActive())
	assert.Equal(t, DefaultServiceName, s.ServiceName)
	assert.InDelta(t, 1.0, s.Sample, 0)
}

func TestResolve_RepoConfigCannotEnableNetworkExport(t *testing.T) {
	hostile := &config.TelemetryConfig{
		Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://evil.example.com", HeadersEnv: []string{"HOME_TOKEN"},
		IncludePaths: true, IncludeSession: true, SaltFile: "/etc/passwd", ServiceName: "mine",
		Resource: map[string]string{"team": "evil"},
	}
	s := Resolve(Layers{Repo: hostile, Getenv: env()})
	assert.True(t, s.RecordActive(), "a repo may switch local recording on")
	assert.False(t, s.ExportActive(), "but never network export")
	assert.False(t, s.AllowNetwork)
	assert.Empty(t, s.Endpoint)
	assert.Empty(t, s.HeadersEnv)
	assert.False(t, s.IncludePaths)
	assert.False(t, s.IncludeSession)
	assert.Empty(t, s.SaltFile)
	assert.Equal(t, DefaultServiceName, s.ServiceName, "service_name lands on the user's collector data: user scope only")
	assert.ElementsMatch(t, []string{"allow_network", "otlp_endpoint", "headers_env", "include_paths", "include_session", "salt_file", "service_name", "resource"}, s.Ignored)
	assert.Empty(t, s.Resource, "a repository cannot label the data on the user's collector")
}

func TestResolve_ServiceNameIsUserScopeOnly(t *testing.T) {
	repo := &config.TelemetryConfig{ServiceName: "attacker-chosen"}
	user := &config.TelemetryConfig{ServiceName: "my-team"}

	fromRepo := Resolve(Layers{Repo: repo, Getenv: env()})
	assert.Equal(t, DefaultServiceName, fromRepo.ServiceName)
	assert.Contains(t, fromRepo.Ignored, "service_name")

	fromUser := Resolve(Layers{Repo: repo, User: user, Getenv: env()})
	assert.Equal(t, "my-team", fromUser.ServiceName)
	assert.Equal(t, ScopeUser, fromUser.Sources["service_name"])

	fromEnv := Resolve(Layers{Repo: repo, User: user, Getenv: env(EnvServiceName, "from-env")})
	assert.Equal(t, "from-env", fromEnv.ServiceName)
}

func TestResolve_UserScopeOptsInAndRepoCannotRedirectIt(t *testing.T) {
	user := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://collector.example.org:4318/", HeadersEnv: []string{"OTLP_HEADERS"}}
	repo := &config.TelemetryConfig{OTLPEndpoint: "https://evil.example.com"}
	s := Resolve(Layers{Repo: repo, User: user, Getenv: env()})
	assert.True(t, s.ExportActive())
	assert.Equal(t, "https://collector.example.org:4318", s.Endpoint, "the user's endpoint wins and loses its trailing slash")
	assert.Equal(t, ScopeUser, s.Sources["otlp_endpoint"])
}

func TestResolve_EnvironmentOptsIn(t *testing.T) {
	s := Resolve(Layers{Getenv: env(EnvEnabled, "on", EnvAllowNetwork, "1", EnvEndpoint, "http://127.0.0.1:4318", EnvSample, "0.25")})
	assert.True(t, s.ExportActive())
	assert.InDelta(t, 0.25, s.Sample, 0)
	assert.Equal(t, ScopeEnv, s.Sources["allow_network"])
}

func TestResolve_KillSwitchesWin(t *testing.T) {
	user := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org"}
	for name, e := range map[string]func(string) string{
		"off":          env(EnvEnabled, "off"),
		"do not track": env("DO_NOT_TRACK", "1"),
	} {
		s := Resolve(Layers{User: user, Getenv: e})
		assert.False(t, s.RecordActive(), name)
		assert.False(t, s.ExportActive(), name)
		assert.NotEmpty(t, s.Killed, name)
	}
}

func TestResolve_InsecureEndpointAndBadProtocolBlockExport(t *testing.T) {
	for name, user := range map[string]*config.TelemetryConfig{
		"plain http":       {Enabled: true, AllowNetwork: true, OTLPEndpoint: "http://collector.example.org"},
		"credentials":      {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://user:pw@collector.example.org"},
		"unknown protocol": {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org", OTLPProtocol: "carrier-pigeon"},
		"grpc with a path": {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org/v1/logs", OTLPProtocol: "grpc"},
		"query":            {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org?x=1"},
	} {
		s := Resolve(Layers{User: user, Getenv: env()})
		assert.False(t, s.ExportActive(), name)
		assert.NotEmpty(t, s.Problems, name)
	}
	loopback := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "http://localhost:4318"}
	assert.True(t, Resolve(Layers{User: loopback, Getenv: env()}).ExportActive())
}

func TestResolve_RepoProblemsDoNotTurnOffAUsersExport(t *testing.T) {
	user := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org"}
	repo := &config.TelemetryConfig{OTLPEndpoint: "http://not-https.example.com"}
	s := Resolve(Layers{Repo: repo, User: user, Getenv: env()})
	assert.True(t, s.ExportActive(), "a hostile repo must not be able to disable export either")
	assert.NotEmpty(t, s.Problems, "but the problem is still reported")
}

func TestValidate_RejectsLiteralSecretsInHeadersEnv(t *testing.T) {
	for _, bad := range []string{
		"Bearer abc123", "authorization=Bearer x", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "sk-ant-api03-xxxx",
		"AKIAIOSFODNN7EXAMPLE", "lowercase_name", "has space", "", "9STARTS_WITH_DIGIT",
	} {
		problems := (&config.TelemetryConfig{HeadersEnv: []string{bad}}).Validate()
		require.NotEmpty(t, problems, bad)
		for _, p := range problems {
			if bad != "" {
				assert.NotContains(t, p, bad, "the rejected value must not be echoed back")
			}
		}
	}
	assert.Empty(t, (&config.TelemetryConfig{HeadersEnv: []string{"OTLP_HEADERS", "AI_RULEZ_OTLP_HEADERS"}}).Validate())
}

func TestValidate_Ranges(t *testing.T) {
	bad := 1.5
	problems := (&config.TelemetryConfig{Sample: &bad, ServiceName: "has space", OTLPProtocol: "xml"}).Validate()
	assert.Len(t, problems, 3)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestLoadRepo_OverlayWinsPerKeyAndUnknownKeysFail(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".ai-rulez", "config.toml"), "[telemetry]\nenabled = true\nservice_name = \"shared\"\nsample = 1\n")
	write(t, filepath.Join(root, ".ai-rulez", "config.local.toml"), "[telemetry]\nservice_name = \"mine\"\n")
	cfg, err := LoadRepo(root, ".ai-rulez")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "mine", cfg.ServiceName)

	write(t, filepath.Join(root, ".ai-rulez", "config.local.toml"), "[telemetry]\nenabeld = true\n")
	_, err = LoadRepo(root, ".ai-rulez")
	require.Error(t, err)
}

func TestResolveFor_UserFileAndYAMLRepo(t *testing.T) {
	root, xdg := t.TempDir(), t.TempDir()
	write(t, filepath.Join(root, ".ai-rulez", "config.yaml"), "telemetry:\n  enabled: true\n  otlp_endpoint: https://evil.example.com\n")
	write(t, filepath.Join(xdg, "ai-rulez", "config.toml"), "[telemetry]\nallow_network = true\notlp_endpoint = \"https://collector.example.org:4318\"\n")
	s := ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", xdg), nil)
	assert.True(t, s.ExportActive())
	assert.Equal(t, "https://collector.example.org:4318", s.Endpoint)
	assert.Contains(t, s.Ignored, "otlp_endpoint")

	// Without the user file the same repo cannot export.
	s = ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", t.TempDir()), nil)
	assert.True(t, s.RecordActive())
	assert.False(t, s.ExportActive())
}

func TestResolveFor_BrokenConfigNeverBreaksAHook(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".ai-rulez", "config.toml"), "[telemetry\nbroken")
	s := ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", t.TempDir()), nil)
	assert.False(t, s.RecordActive())
	assert.NotEmpty(t, s.Problems)
}

func TestResolve_ResourceIsUserScopeAndEnvOnly(t *testing.T) {
	repo := &config.TelemetryConfig{Resource: map[string]string{"team": "attacker"}}
	user := &config.TelemetryConfig{Resource: map[string]string{"team": "platform", "env": "prod"}}

	fromRepo := Resolve(Layers{Repo: repo, Getenv: env()})
	assert.Empty(t, fromRepo.Resource)
	assert.Contains(t, fromRepo.Ignored, "resource")

	fromUser := Resolve(Layers{Repo: repo, User: user, Getenv: env()})
	assert.Equal(t, map[string]string{"team": "platform", "env": "prod"}, fromUser.Resource)
	assert.Equal(t, ScopeUser, fromUser.Sources["resource"])

	fromEnv := Resolve(Layers{User: user, Getenv: env(EnvResource, "team=data, region=eu")})
	assert.Equal(t, map[string]string{"team": "data", "env": "prod", "region": "eu"}, fromEnv.Resource, "env wins per key")
	assert.Equal(t, ScopeEnv, fromEnv.Sources["resource"])
}

func TestResolve_InvalidResourceBlocksExport(t *testing.T) {
	user := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org"}
	tests := []struct {
		name string
		user *config.TelemetryConfig
		env  func(string) string
	}{
		{"reserved key in user config", &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org", Resource: map[string]string{"host.name": "x"}}, env()},
		{"reserved key from env", user, env(EnvResource, "user.id=bob")},
		{"malformed env pair", user, env(EnvResource, "team")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Resolve(Layers{User: tt.user, Getenv: tt.env})
			assert.False(t, s.ExportActive())
			assert.NotEmpty(t, s.Problems)
		})
	}
}

func TestDiagnose_ShowsResourceAndItsSource(t *testing.T) {
	// Arrange
	user := &config.TelemetryConfig{Enabled: true, Resource: map[string]string{"team": "platform", "env": "prod"}}
	repo := &config.TelemetryConfig{Resource: map[string]string{"team": "evil"}}
	s := Resolve(Layers{User: user, Repo: repo, Getenv: env()})

	// Act
	report := Diagnose(&s, t.TempDir(), env())

	// Assert
	var got DoctorSetting
	for _, setting := range report.Settings {
		if setting.Key == "resource" {
			got = setting
		}
	}
	assert.Equal(t, DoctorSetting{Key: "resource", Value: "env=prod,team=platform", Source: ScopeUser}, got)
	assert.Contains(t, report.IgnoredRepoKeys, "resource")
}

// A credential pasted into headers_env (instead of a variable name) must not be
// echoed by doctor, in text, in the settings row or in JSON.
func TestDiagnose_NeverEchoesAPastedCredentialInHeadersEnv(t *testing.T) {
	// Arrange
	const secret = "Bearer sk-live-abcdef0123456789"
	s := Resolve(Layers{Getenv: env(EnvHeadersEnv, secret+",OTLP_HEADERS")})

	// Act
	report := Diagnose(&s, t.TempDir(), env("OTLP_HEADERS", "x"))
	var text bytes.Buffer
	report.Render(&text)
	asJSON, err := json.Marshal(report)
	require.NoError(t, err)

	// Assert
	assert.NotContains(t, text.String(), "sk-live")
	assert.NotContains(t, string(asJSON), "sk-live")
	assert.Contains(t, text.String(), "OTLP_HEADERS")
	assert.Contains(t, text.String(), "(invalid, hidden)")
}

// A pasted credential that happens to look like a variable name (upper case, no
// underscore) must be hidden too, and its environment variable never looked up.
func TestDiagnose_HidesCredentialShapedHeadersEnvNames(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"aws access key id", "AKIAIOSFODNN7EXAMPLE"},
		{"temporary aws key id", "ASIAIOSFODNN7EXAMPLE12"},
		{"long upper case token", "ABCDEF0123456789ABCDEF0123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			looked := []string{}
			getenv := func(key string) string {
				looked = append(looked, key)
				if key == EnvHeadersEnv {
					return tt.secret + ",OTLP_HEADERS"
				}
				return ""
			}
			s := Resolve(Layers{Getenv: getenv})

			// Act
			report := Diagnose(&s, t.TempDir(), getenv)
			var text bytes.Buffer
			report.Render(&text)
			asJSON, err := json.Marshal(report)
			require.NoError(t, err)

			// Assert
			assert.NotContains(t, text.String(), tt.secret)
			assert.NotContains(t, string(asJSON), tt.secret)
			assert.NotContains(t, looked, tt.secret, "a credential-shaped name is never looked up")
			assert.Contains(t, text.String(), "(invalid, hidden)")
			assert.Contains(t, text.String(), "OTLP_HEADERS")
		})
	}
}

func TestRender_AlignsLongSettingValues(t *testing.T) {
	// Arrange
	user := &config.TelemetryConfig{Enabled: true, Resource: map[string]string{"deployment.environment": "production-eu-west-1", "team": "platform-engineering"}}
	s := Resolve(Layers{User: user, Getenv: env()})
	var text bytes.Buffer

	// Act
	Diagnose(&s, t.TempDir(), env()).Render(&text)

	// Assert: every settings row has its source in the same column.
	report := Diagnose(&s, t.TempDir(), env())
	lines := strings.Split(text.String(), "\n")
	offsets := map[int]bool{}
	for _, setting := range report.Settings {
		for _, line := range lines {
			if strings.HasPrefix(line, "  "+setting.Key+" ") {
				offsets[strings.LastIndex(line, setting.Source)] = true
			}
		}
	}
	assert.Len(t, offsets, 1, text.String())
}
