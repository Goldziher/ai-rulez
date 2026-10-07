package telemetry

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const consentEndpoint = "https://collector.example.org:4318"

func record(endpoint, protocol string, paths, session bool) *Consent {
	c := NewConsent(endpoint, protocol, paths, session, fixedNow, "5.0.0")
	return &c
}

func TestResolve_ConsentRecordGrantsExportOnlyWhileItMatches(t *testing.T) {
	tests := []struct {
		name        string
		consent     *Consent
		user        *config.TelemetryConfig
		env         func(string) string
		wantExport  bool
		wantState   string
		wantProblem bool
	}{
		{name: "a matching record enables recording and export", consent: record(consentEndpoint, "http/json", false, false), env: env(), wantExport: true, wantState: ConsentRecord},
		{name: "the record supplies the protocol", consent: record(consentEndpoint, "grpc", false, false), env: env(), wantExport: true, wantState: ConsentRecord},
		{name: "a different endpoint in the environment makes it stale", consent: record(consentEndpoint, "http/json", false, false), env: env(EnvEndpoint, "https://other.example.org"), wantState: ConsentStale},
		{name: "a different protocol in the environment makes it stale", consent: record(consentEndpoint, "http/json", false, false), env: env(EnvProtocol, "grpc"), wantState: ConsentStale},
		{name: "opting in to sessions after consent makes it stale", consent: record(consentEndpoint, "http/json", false, false), env: env(EnvIncludeSession, "1"), wantState: ConsentStale},
		{name: "opting in to paths in the user config makes it stale", consent: record(consentEndpoint, "http/json", false, false), user: &config.TelemetryConfig{IncludePaths: true}, env: env(), wantState: ConsentStale},
		{name: "a record that consented to sessions keeps them on", consent: record(consentEndpoint, "http/json", false, true), env: env(), wantExport: true, wantState: ConsentRecord},
		{name: "a record of another version is stale", consent: func() *Consent { c := record(consentEndpoint, "http/json", false, false); c.Version = 99; return c }(), env: env(), wantState: ConsentStale},
		{name: "a record from an allowlist that differed is stale", consent: func() *Consent {
			c := record(consentEndpoint, "http/json", false, false)
			c.Scope.FieldsHash = "old"
			return c
		}(), env: env(), wantState: ConsentStale},
		{name: "the environment can refuse over a record", consent: record(consentEndpoint, "http/json", false, false), env: env(EnvAllowNetwork, "0"), wantState: ConsentDenied},
		{name: "a kill switch beats a record", consent: record(consentEndpoint, "http/json", false, false), env: env("DO_NOT_TRACK", "1"), wantState: ConsentRecord},
		{name: "no record, no consent", env: env(), wantState: ConsentNone},
		{name: "allow_network in user config still works without a record", user: &config.TelemetryConfig{Enabled: ptrBool(true), AllowNetwork: true, OTLPEndpoint: consentEndpoint}, env: env(), wantExport: true, wantState: ConsentConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			s := Resolve(Layers{User: tt.user, Consent: tt.consent, Getenv: tt.env})

			// Assert
			assert.Equal(t, tt.wantExport && s.Killed == "", s.ExportActive())
			assert.Equal(t, tt.wantState, s.ConsentState)
			if tt.wantState == ConsentStale {
				assert.NotEmpty(t, s.ConsentDetail)
				assert.NotEmpty(t, exportBlockers(&s))
			}
		})
	}
}

func TestResolve_RecordProvidesEndpointProtocolAndGates(t *testing.T) {
	s := Resolve(Layers{Consent: record(consentEndpoint, "http/protobuf", true, true), Getenv: env()})
	assert.True(t, s.ExportActive())
	assert.Equal(t, consentEndpoint, s.Endpoint)
	assert.Equal(t, "http/protobuf", s.Protocol)
	assert.True(t, s.IncludePaths)
	assert.True(t, s.IncludeSession)
	for _, key := range []string{"enabled", "otlp_endpoint", "otlp_protocol", "allow_network", "include_paths", "include_session"} {
		assert.Equal(t, ScopeConsent, s.Sources[key], key)
	}
}

func TestResolve_RepoConfigCanNeverGrantConsent(t *testing.T) {
	hostile := &config.TelemetryConfig{Enabled: ptrBool(true), AllowNetwork: true, OTLPEndpoint: "https://evil.example.com", OTLPProtocol: "grpc"}
	s := Resolve(Layers{Repo: hostile, Getenv: env()})
	assert.False(t, s.ExportActive())
	assert.Equal(t, ConsentNone, s.ConsentState)
}

func TestResolve_AnUnreadableRecordIsReportedAndGrantsNothing(t *testing.T) {
	s := Resolve(Layers{ConsentErr: ErrConsentUnsafe, Getenv: env()})
	assert.False(t, s.ExportActive())
	assert.Equal(t, ConsentInvalid, s.ConsentState)
	assert.Contains(t, s.Problems[0], "consent record")
}

func TestResolveFor_OnlyTheUserDirectoryHoldsConsent(t *testing.T) {
	root, xdg := t.TempDir(), t.TempDir()
	// A hostile checkout ships a record next to its config and in its local dir.
	planted := record("https://evil.example.com", "http/json", false, false)
	require.NoError(t, SaveConsent(filepath.Join(root, ".ai-rulez", ConsentFileName), planted))
	require.NoError(t, SaveConsent(filepath.Join(root, ".ai-rulez", "local", ConsentFileName), planted))
	write(t, filepath.Join(root, ".ai-rulez", "config.toml"), "[telemetry]\nenabled = true\n")
	e := env("XDG_CONFIG_HOME", xdg)

	assert.False(t, ResolveFor(root, ".ai-rulez", e, nil).ExportActive(), "a record in the repository is never read")

	require.NoError(t, SaveConsent(ConsentPath(e), record(consentEndpoint, "http/json", false, false)))
	s := ResolveFor(root, ".ai-rulez", e, nil)
	assert.True(t, s.ExportActive())
	assert.Equal(t, consentEndpoint, s.Endpoint)
}

func TestConsentFile_RoundTripPermissionsAndRefusals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai-rulez", ConsentFileName)
	want := record(consentEndpoint, "grpc", false, true)
	require.NoError(t, SaveConsent(path, want))

	got, err := LoadConsent(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, got.Version)
	assert.Equal(t, "2026-10-05T09:12:44Z", got.GrantedAt)
	assert.Contains(t, got.Scope.Fields, "ai_rulez.session")

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		require.NoError(t, os.Chmod(path, 0o666)) //nolint:gosec // exercising the loose-mode refusal
		_, err = LoadConsent(path)
		require.ErrorIs(t, err, ErrConsentUnsafe)
		require.NoError(t, os.Chmod(path, 0o600))
	}

	write(t, path, `{"version":1,"endpoint":"https://x.example.org","allow_everything":true}`)
	require.NoError(t, os.Chmod(path, 0o600))
	_, err = LoadConsent(path)
	require.Error(t, err, "an unknown field is refused")

	missing, err := LoadConsent(filepath.Join(dir, "nope.json"))
	require.NoError(t, err)
	assert.Nil(t, missing)

	removed, err := RemoveConsent(path)
	require.NoError(t, err)
	assert.True(t, removed)
	removed, err = RemoveConsent(path)
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := map[string]string{
		"https://Collector.Example.org:4318/":  "https://collector.example.org:4318",
		"HTTPS://collector.example.org/v1/x//": "https://collector.example.org/v1/x",
		"http://127.0.0.1:4317":                "http://127.0.0.1:4317",
		"not a url":                            "",
		"":                                     "",
	}
	for in, want := range tests {
		assert.Equal(t, want, NormalizeEndpoint(in), in)
	}
}

func TestScopeFor_HashCoversTheGates(t *testing.T) {
	plain, paths, session := ScopeFor(false, false), ScopeFor(true, false), ScopeFor(false, true)
	assert.NotEqual(t, plain.FieldsHash, paths.FieldsHash)
	assert.NotEqual(t, plain.FieldsHash, session.FieldsHash)
	assert.NotEqual(t, paths.FieldsHash, session.FieldsHash)
	assert.Equal(t, plain.FieldsHash, ScopeFor(false, false).FieldsHash, "deterministic")
	assert.NotContains(t, plain.Fields, "ai_rulez.session")
}
