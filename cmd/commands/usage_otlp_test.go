package commands

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCollector records the event ids of the OTLP/HTTP JSON logs it receives.
type fakeCollector struct {
	mu     sync.Mutex
	ids    []string
	evals  int
	status int
}

func (c *fakeCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	zr, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, "gzip", http.StatusBadRequest)
		return
	}
	body, _ := io.ReadAll(zr) //nolint:errcheck // a test server
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status != 0 {
		w.WriteHeader(c.status)
		return
	}
	if r.URL.Path == "/v1/logs" {
		var req struct {
			ResourceLogs []struct {
				ScopeLogs []struct {
					LogRecords []struct {
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								String string `json:"stringValue"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"logRecords"`
				} `json:"scopeLogs"`
			} `json:"resourceLogs"`
		}
		_ = json.Unmarshal(body, &req) //nolint:errcheck // a test server
		for _, rl := range req.ResourceLogs {
			for _, sl := range rl.ScopeLogs {
				for _, rec := range sl.LogRecords {
					for _, a := range rec.Attributes {
						switch {
						case a.Key == "ai_rulez.item.id":
							c.ids = append(c.ids, a.Value.String)
						case a.Key == "event.name" && a.Value.String == "ai_rulez.eval.result":
							c.evals++
						}
					}
				}
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (c *fakeCollector) received() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string(nil), c.ids...)
	sort.Strings(out)
	return out
}

func consentedProject(t *testing.T, collector *fakeCollector) (telemetryEnv, string) {
	t.Helper()
	resetConsentFlags(t)
	resetExportFlags(t)
	srv := httptest.NewServer(collector)
	t.Cleanup(srv.Close)
	env := setupTelemetry(t, "", "")
	t.Chdir(env.root)
	return env, srv.URL
}

func TestUsageExportOTLP_PushesPastTheCursorOnceAndHistoryOnlyOnRequest(t *testing.T) {
	// Arrange: two loads before consent, consent, two loads after.
	collector := &fakeCollector{}
	env, endpoint := consentedProject(t, collector)
	recordSkillLoads(t, env, "old-a", "old-b")
	telEnableEndpoint = endpoint
	require.NoError(t, runTelemetryEnable(&bytes.Buffer{}))
	recordSkillLoads(t, env, "new-a", "new-b")
	usageExportTo = "otlp"
	var out bytes.Buffer

	// Act
	require.NoError(t, runUsageExportOTLP(&out))

	// Assert: only what was recorded after consent, each once (the hooks already queued them).
	assert.Equal(t, []string{"new-a", "new-b"}, collector.received())
	assert.Contains(t, out.String(), "delivered 2 in 1 batches")

	out.Reset()
	require.NoError(t, runUsageExportOTLP(&out))
	assert.Equal(t, []string{"new-a", "new-b"}, collector.received(), "a second run sends nothing")
	assert.Contains(t, out.String(), "delivered 0 in 0 batches")

	usageExportAll = true
	out.Reset()
	require.NoError(t, runUsageExportOTLP(&out))
	assert.Equal(t, []string{"new-a", "new-b", "old-a", "old-b"}, collector.received(), "--all sends the history, and not the delivered events again")
}

func TestUsageExportOTLP_DryRunSendsAndMovesNothing(t *testing.T) {
	collector := &fakeCollector{}
	env, endpoint := consentedProject(t, collector)
	telEnableEndpoint, telEnableBackfill = endpoint, true
	recordSkillLoads(t, env, "a", "b", "c")
	require.NoError(t, runTelemetryEnable(&bytes.Buffer{}))
	usageExportTo, usageExportDryRun = "otlp", true
	var out bytes.Buffer

	require.NoError(t, runUsageExportOTLP(&out))

	assert.Contains(t, out.String(), "would queue")
	assert.Contains(t, out.String(), "nothing sent")
	assert.Empty(t, collector.received())
}

func TestUsageExportOTLP_WithEvalsSendsResultsOnceAsEvalEvents(t *testing.T) {
	collector := &fakeCollector{}
	env, endpoint := consentedProject(t, collector)
	writeStore(t, env.root) // signs with its own user key and moves XDG_CONFIG_HOME: consent comes after
	telEnableEndpoint = endpoint
	require.NoError(t, runTelemetryEnable(&bytes.Buffer{}))
	usageExportTo, usageExportWithEvals = "otlp", true
	var out bytes.Buffer

	require.NoError(t, runUsageExportOTLP(&out))

	assert.Equal(t, []string{"alpha", "idle"}, collector.received())
	assert.Equal(t, 2, collector.evals)
	assert.Contains(t, out.String(), "2 eval results")

	out.Reset()
	require.NoError(t, runUsageExportOTLP(&out))
	assert.Equal(t, 2, collector.evals, "the same results are not exported twice")
	assert.Contains(t, out.String(), "0 eval results")
}

func TestUsageExportOTLP_ARejectedBatchIsAnErrorAndCounted(t *testing.T) {
	collector := &fakeCollector{status: http.StatusBadRequest}
	env, endpoint := consentedProject(t, collector)
	telEnableEndpoint = endpoint
	require.NoError(t, runTelemetryEnable(&bytes.Buffer{}))
	recordSkillLoads(t, env, "a")
	usageExportTo = "otlp"

	err := runUsageExportOTLP(&bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "export")
	telemetryStatusCmd.SetOut(&bytes.Buffer{})
	var out bytes.Buffer
	telemetryStatusCmd.SetOut(&out)
	require.NoError(t, telemetryStatusCmd.RunE(telemetryStatusCmd, nil))
	assert.Contains(t, out.String(), "rejected 1")
	assert.Contains(t, out.String(), "failed flushes 1")
}

func TestUsageExportFile_WithEvalsAddsEvalResultRecords(t *testing.T) {
	resetExportFlags(t)
	env := setupTelemetry(t, "", "")
	t.Chdir(env.root)
	recordSkillLoads(t, env, "deploy")
	writeStore(t, env.root)
	dest := t.TempDir() + "/usage.ndjson"
	usageExportTo, usageExportWithEvals = "file", true
	var out bytes.Buffer

	require.NoError(t, runUsageExport(&out, []string{dest}))

	assert.Contains(t, out.String(), "wrote 3 events")
	data := readFileString(t, dest)
	assert.Equal(t, 2, strings.Count(data, "ai_rulez.eval.result"))
	assert.Contains(t, data, `"doubleValue":0.75`)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
