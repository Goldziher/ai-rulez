package telemetry

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The collector end-to-end test sends a batch to a real OpenTelemetry Collector
// and reads back what its file exporter wrote. It is opt-in: it needs docker and
// AI_RULEZ_E2E_OTELCOL=1 (task test:otelcol sets both the variable and the -run
// filter), so the default test run never pulls an image or opens a port.
const (
	envE2EOtelcol = "AI_RULEZ_E2E_OTELCOL"
	// otelcolImage is pinned by tag and index digest; bump both together.
	otelcolImage = "otel/opentelemetry-collector-contrib:0.130.0@sha256:867d1074c2f750936fb9358ec9eefa009308053cf156b2c7ca1761ba5ef78452"
)

const otelcolConfig = `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  file/logs:
    path: /out/logs.json
    flush_interval: 200ms
  file/metrics:
    path: /out/metrics.json
    flush_interval: 200ms
service:
  pipelines:
    logs:
      receivers: [otlp]
      exporters: [file/logs]
    metrics:
      receivers: [otlp]
      exporters: [file/metrics]
`

// startCollector runs the pinned collector and returns its OTLP/HTTP base URL and
// the directory its file exporter writes to.
func startCollector(t *testing.T) (endpoint, outDir string) {
	t.Helper()
	if os.Getenv(envE2EOtelcol) != "1" {
		t.Skipf("set %s=1 to run the real-collector test (task test:otelcol)", envE2EOtelcol)
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is not installed")
	}
	if err := exec.Command(docker, "info").Run(); err != nil { //nolint:gosec // fixed arguments
		t.Skip("the docker daemon is not reachable")
	}

	dir := t.TempDir()
	outDir = filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(outDir, 0o750))
	require.NoError(t, os.Chmod(outDir, 0o777)) //nolint:gosec // the collector runs as another uid and must write here
	cfg := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(otelcolConfig), 0o600))

	out, err := exec.Command(docker, "run", "-d", "--rm", "-p", "127.0.0.1::4318", //nolint:gosec // fixed arguments, test paths
		"-v", cfg+":/etc/otelcol/config.yaml:ro", "-v", outDir+":/out", otelcolImage, "--config=/etc/otelcol/config.yaml").CombinedOutput()
	require.NoError(t, err, string(out))
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command(docker, "logs", "--tail", "40", id).CombinedOutput() //nolint:gosec,errcheck // diagnostics only
			t.Logf("collector logs:\n%s", logs)
		}
		_ = exec.Command(docker, "rm", "-f", id).Run() //nolint:gosec,errcheck // best-effort cleanup
	})

	portOut, err := exec.Command(docker, "port", id, "4318/tcp").CombinedOutput() //nolint:gosec // fixed arguments
	require.NoError(t, err, string(portOut))
	hostPort := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	hostPort = strings.TrimPrefix(hostPort, "0.0.0.0")
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		hostPort = "127.0.0.1" + hostPort[i:]
	}
	endpoint = "http://" + hostPort

	require.Eventually(t, func() bool {
		resp, err := http.Get(endpoint + "/v1/logs") //nolint:gosec,noctx // loopback readiness probe
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true // a GET is answered (405) once the receiver listens
	}, 60*time.Second, 250*time.Millisecond, "collector did not start")
	return endpoint, outDir
}

type otlpKV struct {
	Key   string `json:"key"`
	Value struct {
		String *string `json:"stringValue"`
		Int    *string `json:"intValue"`
		Bool   *bool   `json:"boolValue"`
	} `json:"value"`
}

func attrMap(kvs []otlpKV) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		switch {
		case kv.Value.String != nil:
			m[kv.Key] = *kv.Value.String
		case kv.Value.Int != nil:
			m[kv.Key] = *kv.Value.Int
		case kv.Value.Bool != nil:
			m[kv.Key] = fmt.Sprint(*kv.Value.Bool)
		}
	}
	return m
}

type fileRecord struct {
	ResourceLogs []struct {
		Resource struct {
			Attributes []otlpKV `json:"attributes"`
		} `json:"resource"`
		ScopeLogs []struct {
			LogRecords []struct {
				Body       map[string]any `json:"body"`
				Attributes []otlpKV       `json:"attributes"`
			} `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
	ResourceMetrics []struct {
		Resource struct {
			Attributes []otlpKV `json:"attributes"`
		} `json:"resource"`
		ScopeMetrics []struct {
			Metrics []struct {
				Name string                     `json:"name"`
				Sum  map[string]json.RawMessage `json:"sum"`
				Hist map[string]json.RawMessage `json:"histogram"`
			} `json:"metrics"`
		} `json:"scopeMetrics"`
	} `json:"resourceMetrics"`
}

// readExported waits until the collector's file holds at least one line, then
// decodes every complete line. The file exporter appends while it is read, so
// the bytes after the last newline can be a half-written record: they are left
// for the next read.
func readExported(t *testing.T, path string) []fileRecord {
	t.Helper()
	var data []byte
	require.Eventually(t, func() bool {
		var err error
		data, err = os.ReadFile(path) //nolint:gosec // a test file
		return err == nil && bytes.Contains(data, []byte("\n"))
	}, 20*time.Second, 200*time.Millisecond, "collector wrote nothing to %s", path)
	data = data[:bytes.LastIndexByte(data, '\n')]
	var out []fileRecord
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var rec fileRecord
		require.NoError(t, json.Unmarshal(line, &rec), string(line))
		out = append(out, rec)
	}
	return out
}

func TestCollectorAcceptsExportedBatch(t *testing.T) {
	// Arrange
	endpoint, outDir := startCollector(t)
	spool := &Spool{Dir: t.TempDir()}
	events := sampleEvents()
	for i := range events {
		require.NoError(t, spool.Append(&events[i]))
	}
	resource := map[string]string{"team": "platform", "deployment.environment": "e2e"}
	x := &Exporter{
		Spool: spool, Endpoint: endpoint, Retries: 1,
		Encoder: Encoder{ServiceName: "ai-rulez-e2e", ServiceVersion: "0.0.0-test", Resource: resource},
		Now:     fixedClock,
	}

	// Act
	result, err := x.Flush(context.Background())

	// Assert: the collector accepted the gzipped logs and metrics bodies.
	require.NoError(t, err)
	assert.Equal(t, len(events), result.Sent)
	assert.Zero(t, result.Rejected)
	pending, _, err := spool.Pending()
	require.NoError(t, err)
	assert.Empty(t, pending, "an accepted batch leaves the outbox")

	// Assert: what the collector decoded matches what was sent and the allowlist.
	allowed := map[string]bool{"event.name": true}
	for _, a := range Allowlist {
		allowed[a.Name] = true
	}
	var ids []string
	for _, rec := range readExported(t, filepath.Join(outDir, "logs.json")) {
		for _, rl := range rec.ResourceLogs {
			res := attrMap(rl.Resource.Attributes)
			assert.Equal(t, "ai-rulez-e2e", res["service.name"])
			assert.Equal(t, "platform", res["team"])
			assert.Equal(t, "e2e", res["deployment.environment"])
			assert.Equal(t, fmt.Sprint(SchemaVersion), res["ai_rulez.schema_version"])
			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					attrs := attrMap(lr.Attributes)
					for key := range attrs {
						assert.True(t, allowed[key], "attribute %q is not in the allowlist", key)
					}
					assert.NotContains(t, attrs, "ai_rulez.session", "include_session is off")
					assert.NotContains(t, attrs, "ai_rulez.item.path", "include_paths is off")
					ids = append(ids, attrs["ai_rulez.event_id"])
				}
			}
		}
	}
	assert.ElementsMatch(t, []string{"aaaaaaaaaaaaaaa1", "aaaaaaaaaaaaaaa2", "aaaaaaaaaaaaaaa3", "aaaaaaaaaaaaaaa4"}, ids)

	names := map[string]bool{}
	for _, rec := range readExported(t, filepath.Join(outDir, "metrics.json")) {
		for _, rm := range rec.ResourceMetrics {
			assert.Equal(t, "platform", attrMap(rm.Resource.Attributes)["team"])
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					names[m.Name] = true
				}
			}
		}
	}
	assert.True(t, names[MetricLoads], "metrics seen: %v", names)
	assert.True(t, names[MetricAgentTime], "metrics seen: %v", names)
}

func TestCollectorStatusCodesMatchTheExporterContract(t *testing.T) {
	// Arrange
	endpoint, _ := startCollector(t)
	post := func(body []byte, gz bool) (*http.Response, error) {
		payload := body
		if gz {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(body) //nolint:errcheck // a bytes.Buffer does not fail
			require.NoError(t, zw.Close())
			payload = buf.Bytes()
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint+"/v1/logs", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if gz {
			req.Header.Set("Content-Encoding", "gzip")
		}
		return http.DefaultClient.Do(req)
	}

	// Act and Assert: a valid gzipped body is a 200 whose response decodes; a
	// malformed one is a 400, which the exporter treats as permanent (ErrRejected).
	body, err := (&Encoder{}).EncodeLogs(sampleEvents()[:1], fixedNow)
	require.NoError(t, err)
	ok, err := post(body, true)
	require.NoError(t, err)
	defer ok.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, ok.StatusCode)
	var reply struct {
		PartialSuccess struct {
			RejectedLogRecords any `json:"rejectedLogRecords"`
		} `json:"partialSuccess"`
	}
	require.NoError(t, json.NewDecoder(ok.Body).Decode(&reply))
	assert.Contains(t, []any{nil, "0", float64(0)}, reply.PartialSuccess.RejectedLogRecords, "nothing was partially rejected")

	bad, err := post([]byte(`{"resourceLogs": [`), true)
	require.NoError(t, err)
	defer bad.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusBadRequest, bad.StatusCode)
}
