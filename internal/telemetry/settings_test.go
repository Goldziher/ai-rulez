package telemetry

import (
	"os"
	"path/filepath"
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
	assert.Equal(t, "mine", s.ServiceName, "harmless keys are honored")
	assert.ElementsMatch(t, []string{"allow_network", "otlp_endpoint", "headers_env", "include_paths", "include_session", "salt_file"}, s.Ignored)
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
		"plain http":  {Enabled: true, AllowNetwork: true, OTLPEndpoint: "http://collector.example.org"},
		"credentials": {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://user:pw@collector.example.org"},
		"grpc":        {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org", OTLPProtocol: "grpc"},
		"query":       {Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://c.example.org?x=1"},
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
	s := ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", xdg))
	assert.True(t, s.ExportActive())
	assert.Equal(t, "https://collector.example.org:4318", s.Endpoint)
	assert.Contains(t, s.Ignored, "otlp_endpoint")

	// Without the user file the same repo cannot export.
	s = ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", t.TempDir()))
	assert.True(t, s.RecordActive())
	assert.False(t, s.ExportActive())
}

func TestResolveFor_BrokenConfigNeverBreaksAHook(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".ai-rulez", "config.toml"), "[telemetry\nbroken")
	s := ResolveFor(root, ".ai-rulez", env("XDG_CONFIG_HOME", t.TempDir()))
	assert.False(t, s.RecordActive())
	assert.NotEmpty(t, s.Problems)
}
