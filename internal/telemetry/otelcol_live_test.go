package telemetry

import (
	"context"
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

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The live collector test sends the same events over http/json, http/protobuf
// and grpc to a real OpenTelemetry Collector (the image otelcol_e2e_test.go pins)
// and checks what its file exporter decoded. It is opt-in: it needs docker and
// AI_RULEZ_LIVE_OTEL=1, so go test ./... never pulls an image or opens a port.
const envLiveOtel = "AI_RULEZ_LIVE_OTEL"

const liveCollectorConfig = `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
      grpc:
        endpoint: 0.0.0.0:4317
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

// hostPortOf resolves the loopback address docker published for a container port.
func hostPortOf(t *testing.T, docker, id, port string) string {
	t.Helper()
	out, err := exec.Command(docker, "port", id, port).CombinedOutput() //nolint:gosec // fixed arguments
	require.NoError(t, err, string(out))
	hostPort := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	hostPort = strings.TrimPrefix(hostPort, "0.0.0.0")
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		hostPort = "127.0.0.1" + hostPort[i:]
	}
	return hostPort
}

func startLiveCollector(t *testing.T) (httpEndpoint, grpcEndpoint, outDir string) {
	t.Helper()
	if os.Getenv(envLiveOtel) != "1" {
		t.Skipf("set %s=1 to run the live collector test", envLiveOtel)
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
	require.NoError(t, os.WriteFile(cfg, []byte(liveCollectorConfig), 0o644)) //nolint:gosec // the collector runs as another uid and must read it

	out, err := exec.Command(docker, "run", "-d", "-p", "127.0.0.1::4318", "-p", "127.0.0.1::4317", //nolint:gosec // fixed arguments, test paths
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
	httpEndpoint = "http://" + hostPortOf(t, docker, id, "4318/tcp")
	grpcEndpoint = "http://" + hostPortOf(t, docker, id, "4317/tcp")
	require.Eventually(t, func() bool {
		resp, err := http.Get(httpEndpoint + "/v1/logs") //nolint:gosec,noctx // loopback readiness probe
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 60*time.Second, 250*time.Millisecond, "collector did not start")
	return httpEndpoint, grpcEndpoint, outDir
}

func TestLiveCollectorAcceptsEveryTransport(t *testing.T) {
	httpEndpoint, grpcEndpoint, outDir := startLiveCollector(t)
	runs := []struct{ protocol, endpoint, prefix string }{
		{config.TelemetryProtocolHTTPJSON, httpEndpoint, "a"},
		{config.TelemetryProtocolHTTPProtobuf, httpEndpoint, "b"},
		{config.TelemetryProtocolGRPC, grpcEndpoint, "c"},
	}
	for _, run := range runs {
		events := sampleEvents()
		for i := range events {
			events[i].EventID = fmt.Sprintf("%s%015d", run.prefix, i)
		}
		spool := &Spool{Dir: t.TempDir()}
		for i := range events {
			require.NoError(t, spool.Append(&events[i]))
		}
		x := &Exporter{
			Spool: spool, Endpoint: run.endpoint, Protocol: run.protocol, Retries: 1, Now: fixedClock,
			Encoder: Encoder{ServiceName: "ai-rulez-live", Resource: map[string]string{"team": "platform"}},
		}
		result, err := x.Flush(context.Background())
		require.NoError(t, err, run.protocol)
		assert.Equal(t, len(events), result.Sent, run.protocol)
		assert.Zero(t, result.Rejected, run.protocol)
	}

	allowed := map[string]bool{"event.name": true}
	for _, a := range Allowlist {
		allowed[a.Name] = true
	}
	var perTransport map[string][]string
	require.Eventually(t, func() bool {
		perTransport = eventIDsByTransport(t, filepath.Join(outDir, "logs.json"), allowed)
		return len(perTransport["a"]) == 4 && len(perTransport["b"]) == 4 && len(perTransport["c"]) == 4
	}, 20*time.Second, 300*time.Millisecond, "events seen per transport: %v", perTransport)
}

// exportedLogAttrs flattens the log records the collector wrote into their
// attribute maps, checking the team resource label on the way.
func exportedLogAttrs(t *testing.T, path string) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, rec := range readExported(t, path) {
		for _, rl := range rec.ResourceLogs {
			assert.Equal(t, "platform", attrMap(rl.Resource.Attributes)["team"])
			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					out = append(out, attrMap(lr.Attributes))
				}
			}
		}
	}
	return out
}

// eventIDsByTransport groups the event ids the collector wrote by the first
// character of the id, which each run of the test sets per transport, and checks
// every attribute against the allowlist.
func eventIDsByTransport(t *testing.T, path string, allowed map[string]bool) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, attrs := range exportedLogAttrs(t, path) {
		for key := range attrs {
			assert.True(t, allowed[key], "attribute %q is not on the allowlist", key)
		}
		id := attrs["ai_rulez.event_id"]
		out[id[:1]] = append(out[id[:1]], id)
	}
	return out
}
