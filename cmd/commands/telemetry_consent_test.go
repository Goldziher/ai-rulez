package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetConsentFlags(t *testing.T) {
	t.Helper()
	clear := func() {
		telEnableEndpoint, telEnableProtocol, telEnableSession, telEnablePaths, telEnableBackfill, telStatusJSON = "", "", false, false, false, false
	}
	clear()
	t.Cleanup(clear)
}

func consentFile(env telemetryEnv) string {
	return filepath.Join(env.xdg, "ai-rulez", telemetry.ConsentFileName)
}

func appendRawLog(t *testing.T, path, lines string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // test file
	require.NoError(t, err)
	_, err = f.WriteString(lines)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestTelemetryEnableStatusDisable_Lifecycle(t *testing.T) {
	// Arrange: a project with two skill loads already in its log.
	resetConsentFlags(t)
	env := setupTelemetry(t, "", "")
	recordSkillLoads(t, env, "deploy", "release-notes")
	telEnableEndpoint = "https://collector.example.org:4318/secret-path"
	var out bytes.Buffer

	// Act
	require.NoError(t, runTelemetryEnable(&out))

	// Assert: consent is stored per user, 0600, and says what it covers; no secrets echoed.
	text := out.String()
	assert.Contains(t, text, "consent recorded for https://collector.example.org:4318 (http/json)")
	assert.NotContains(t, text, "secret-path", "the path is never echoed")
	assert.Contains(t, text, "export is on")
	assert.Contains(t, text, "cursor: at the end of the usage log")
	record, err := telemetry.LoadConsent(consentFile(env))
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, 1, record.Version)
	assert.Equal(t, "https://collector.example.org:4318/secret-path", record.Endpoint)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(consentFile(env))
		require.NoError(t, statErr)
		assertFileMode(t, info, 0o600, "owner-only")
	}
	spool := &telemetry.Spool{Dir: telemetry.LocalDir(env.root, ".ai-rulez")}
	assert.True(t, spool.ReadCursor().Set())
	assert.Positive(t, spool.ReadCursor().Offset, "the existing history is behind the cursor")

	// Status reports the grant, the host and no path or header.
	out.Reset()
	telemetryStatusCmd.SetOut(&out)
	require.NoError(t, telemetryStatusCmd.RunE(telemetryStatusCmd, nil))
	assert.Contains(t, out.String(), "export on")
	assert.Contains(t, out.String(), "granted by the consent record")
	assert.Contains(t, out.String(), "collector.example.org:4318")
	assert.NotContains(t, out.String(), "secret-path")

	telStatusJSON = true
	out.Reset()
	require.NoError(t, telemetryStatusCmd.RunE(telemetryStatusCmd, nil))
	var status telemetry.Status
	require.NoError(t, json.Unmarshal(out.Bytes(), &status))
	assert.True(t, status.Export)
	assert.Equal(t, telemetry.ConsentRecord, status.Consent.State)

	// Disable removes the record; the log stays.
	out.Reset()
	require.NoError(t, runTelemetryDisable(&out))
	assert.Contains(t, out.String(), "consent withdrawn")
	assert.Contains(t, out.String(), "export is off")
	// MAN-2: the record was what turned recording on, so recording stops with it.
	assert.Contains(t, out.String(), "local recording is off")
	assert.NotContains(t, out.String(), "unchanged")
	assert.NoFileExists(t, consentFile(env))
	assert.FileExists(t, env.log)
	out.Reset()
	require.NoError(t, runTelemetryDisable(&out))
	assert.Contains(t, out.String(), "there was no consent record")
}

func TestTelemetryEnable_BackfillPlacesTheCursorAtTheStart(t *testing.T) {
	resetConsentFlags(t)
	env := setupTelemetry(t, "", "")
	recordSkillLoads(t, env, "deploy")
	telEnableEndpoint, telEnableBackfill = "https://collector.example.org", true

	var out bytes.Buffer
	require.NoError(t, runTelemetryEnable(&out))

	spool := &telemetry.Spool{Dir: telemetry.LocalDir(env.root, ".ai-rulez")}
	assert.Zero(t, spool.ReadCursor().Offset)
	assert.NotEmpty(t, spool.ReadCursor().LogID)
	assert.Contains(t, out.String(), "existing history is exported")
}

func TestTelemetryEnable_RefusesWhatItCannotConsentTo(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
		repo  string
		want  string
	}{
		{name: "no endpoint anywhere", setup: func() {}, want: "no collector endpoint"},
		{name: "plain http to a remote host", setup: func() { telEnableEndpoint = "http://collector.example.org" }, want: "plain http"},
		{name: "credentials in the endpoint", setup: func() { telEnableEndpoint = "https://user:pw@collector.example.org" }, want: "credentials"},
		{name: "an unknown protocol", setup: func() { telEnableEndpoint, telEnableProtocol = "https://c.example.org", "carrier-pigeon" }, want: "unknown value"},
		{name: "a grpc endpoint with a path", setup: func() { telEnableEndpoint, telEnableProtocol = "https://c.example.org/v1/logs", "grpc" }, want: "without a path"},
		{name: "an endpoint only the repository names", setup: func() {}, repo: "\n[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"https://evil.example.com\"\n", want: "no collector endpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConsentFlags(t)
			env := setupTelemetry(t, tt.repo, "")
			tt.setup()

			err := runTelemetryEnable(&bytes.Buffer{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.NoFileExists(t, consentFile(env), "a refused enable stores nothing")
		})
	}
}

func TestTelemetryEnable_GatesBecomePartOfTheConsent(t *testing.T) {
	resetConsentFlags(t)
	env := setupTelemetry(t, "", "")
	telEnableEndpoint, telEnableSession, telEnablePaths = "https://collector.example.org", true, true

	var out bytes.Buffer
	require.NoError(t, runTelemetryEnable(&out))

	record, err := telemetry.LoadConsent(consentFile(env))
	require.NoError(t, err)
	assert.True(t, record.Scope.IncludeSession)
	assert.True(t, record.Scope.IncludePaths)
	assert.Contains(t, record.Scope.Fields, "ai_rulez.session")
	assert.NotContains(t, out.String(), "withheld:")

	// Re-enabling without the flags is a fresh decision: the gates do not carry over.
	telEnableSession, telEnablePaths = false, false
	out.Reset()
	require.NoError(t, runTelemetryEnable(&out))
	record, err = telemetry.LoadConsent(consentFile(env))
	require.NoError(t, err)
	assert.False(t, record.Scope.IncludeSession)
	assert.Contains(t, out.String(), "withheld: ai_rulez.item.path, ai_rulez.session")
}

func TestTelemetryStatus_StaleConsentSaysSoAndExportStaysOff(t *testing.T) {
	resetConsentFlags(t)
	env := setupTelemetry(t, "", "")
	telEnableEndpoint = "https://collector.example.org"
	require.NoError(t, runTelemetryEnable(&bytes.Buffer{}))
	t.Setenv(telemetry.EnvEndpoint, "https://other.example.org")

	var out bytes.Buffer
	telemetryStatusCmd.SetOut(&out)
	require.NoError(t, telemetryStatusCmd.RunE(telemetryStatusCmd, nil))

	assert.Contains(t, out.String(), "stale: the endpoint changed")
	assert.Contains(t, out.String(), "recording on, export off")
	assert.FileExists(t, consentFile(env))
}

func TestTelemetryStatus_RepoConfigIsNotConsent(t *testing.T) {
	resetConsentFlags(t)
	setupTelemetry(t, "\n[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"https://evil.example.com\"\n", "")

	var out bytes.Buffer
	telemetryStatusCmd.SetOut(&out)
	require.NoError(t, telemetryStatusCmd.RunE(telemetryStatusCmd, nil))

	assert.Contains(t, out.String(), "recording on, export off")
	assert.Contains(t, out.String(), "consent:   none")
	assert.NotContains(t, out.String(), "evil.example.com")
}

func TestTelemetryDisable_ReportsAConfigGrantThatStaysOn(t *testing.T) {
	resetConsentFlags(t)
	setupTelemetry(t, "", "[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"http://127.0.0.1:1\"\n")

	var out bytes.Buffer
	require.NoError(t, runTelemetryDisable(&out))

	assert.Contains(t, out.String(), "export is still on because allow_network in your user config grants it")
}
