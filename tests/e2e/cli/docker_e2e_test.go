package cli

import (
	"bytes"
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

// The docker-gated end-to-end tests run the binary against real services in
// containers: a registry:2 for `publish --to oci` and an OpenTelemetry
// Collector for `telemetry flush`. They are opt-in (AI_RULEZ_E2E_DOCKER=1)
// because they pull images and open ports.
const (
	envE2EDocker = "AI_RULEZ_E2E_DOCKER"
	// Pinned by index digest; bump tag and digest together.
	registryImage = "registry:2@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373"
	collectorImg  = "otel/opentelemetry-collector-contrib:0.130.0@sha256:867d1074c2f750936fb9358ec9eefa009308053cf156b2c7ca1761ba5ef78452"
)

func requireDocker(t *testing.T) string {
	t.Helper()
	if os.Getenv(envE2EDocker) != "1" {
		t.Skipf("set %s=1 to run the docker-backed end-to-end tests", envE2EDocker)
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is not installed")
	}
	if err := exec.Command(docker, "info").Run(); err != nil { //nolint:gosec // fixed arguments
		t.Skip("the docker daemon is not reachable")
	}
	return docker
}

// startContainer runs image detached with one published loopback port and
// returns the container id and the host:port docker chose.
func startContainer(t *testing.T, docker, port string, args ...string) (id, hostPort string) {
	t.Helper()
	argv := append([]string{"run", "-d", "--rm", "-p", "127.0.0.1::" + port}, args...)
	out, err := exec.Command(docker, argv...).CombinedOutput() //nolint:gosec // fixed image, test paths
	require.NoError(t, err, string(out))
	id = strings.TrimSpace(string(out))
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command(docker, "logs", "--tail", "40", id).CombinedOutput() //nolint:gosec,errcheck // diagnostics only
			t.Logf("container logs:\n%s", logs)
		}
		_ = exec.Command(docker, "rm", "-f", id).Run() //nolint:gosec,errcheck // best-effort cleanup
	})
	portOut, err := exec.Command(docker, "port", id, port+"/tcp").CombinedOutput() //nolint:gosec // fixed arguments
	require.NoError(t, err, string(portOut))
	first := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	return id, "127.0.0.1" + first[strings.LastIndex(first, ":"):]
}

func waitHTTP(t *testing.T, url string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := http.Get(url) //nolint:gosec,noctx // loopback readiness probe
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 60*time.Second, 250*time.Millisecond, "%s did not answer", url)
}

// TestDockerPublishOCIE2E pushes a release to a registry:2 container with
// `publish --to oci --execute --yes`, then pulls and verifies it by reference
// with `publish verify`.
func TestDockerPublishOCIE2E(t *testing.T) {
	// Arrange
	docker := requireDocker(t)
	_, host := startContainer(t, docker, "5000", registryImage)
	waitHTTP(t, "http://"+host+"/v2/")
	env := newIsoEnv(t)
	env.set("DOCKER_CONFIG", t.TempDir())
	root := publishableProject(t, env)
	ref := host + "/acme/skills"

	// Act
	push := env.run(root, "publish", "--to", "oci", "--oci-ref", ref, "--execute", "--yes", "--format", "json")
	again := env.run(root, "publish", "--to", "oci", "--oci-ref", ref, "--execute", "--yes")
	verify := env.run(root, "publish", "verify", ref+":1.4.0", "--format", "json")
	writeTree(t, root, map[string]string{".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Use when deploying the service to production; not for local runs.\n---\n\n# Deploy\n\nRun the new pipeline.\n"})
	for _, args := range [][]string{{"generate", "--yes"}, {"generate", "--plugin"}, {"lock"}} {
		require.Equal(t, 0, env.run(root, args...).ExitCode, "%v", args)
	}
	env.commitAll(root, "same version, other content")
	clobber := env.run(root, "publish", "--to", "oci", "--oci-ref", ref, "--execute", "--yes")

	// Assert
	require.Equal(t, 0, push.ExitCode, "stdout: %s\nstderr: %s", push.Stdout, push.Stderr)
	requireJSONDoc(t, push)
	assert.Equal(t, 0, again.ExitCode, "the same release again is a no-op: %s", again.Stdout+again.Stderr)
	assert.Equal(t, 1, clobber.ExitCode, "other content under an existing tag needs --force: %s", clobber.Stdout+clobber.Stderr)
	assert.Contains(t, clobber.Stderr, "needs --force")
	require.Equal(t, 0, verify.ExitCode, "stdout: %s\nstderr: %s", verify.Stdout, verify.Stderr)
	doc := requireJSONDoc(t, verify)
	assert.Equal(t, "acme", doc["name"])
	assert.Equal(t, []any{}, doc["problems"])
}

const collectorConfig = `receivers:
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

// collectorEventIDs reads the event ids and attribute names the collector's
// file exporter wrote, ignoring a half-written last line.
func collectorEventIDs(t *testing.T, path string) (ids []string, keys map[string]bool) {
	t.Helper()
	keys = map[string]bool{}
	data, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		return nil, keys
	}
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		data = data[:i]
	} else {
		return nil, keys
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var rec struct {
			ResourceLogs []struct {
				ScopeLogs []struct {
					LogRecords []struct {
						Attributes []struct {
							Key   string         `json:"key"`
							Value map[string]any `json:"value"`
						} `json:"attributes"`
					} `json:"logRecords"`
				} `json:"scopeLogs"`
			} `json:"resourceLogs"`
		}
		require.NoError(t, json.Unmarshal(line, &rec))
		for _, rl := range rec.ResourceLogs {
			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					for _, a := range lr.Attributes {
						keys[a.Key] = true
						if a.Key == "ai_rulez.event_id" {
							ids = append(ids, fmt.Sprint(a.Value["stringValue"]))
						}
					}
				}
			}
		}
	}
	return ids, keys
}

// TestDockerTelemetryConsentE2E records hook events through the binary and
// flushes them to a real collector: nothing leaves before the user consents,
// a repository config cannot consent for the user, and what arrives carries
// only allowlisted, identifier-only attributes.
func TestDockerTelemetryConsentE2E(t *testing.T) {
	// Arrange
	docker := requireDocker(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(out, 0o750))
	require.NoError(t, os.Chmod(out, 0o777)) //nolint:gosec // the collector runs as another uid
	cfg := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(collectorConfig), 0o600))
	_, host := startContainer(t, docker, "4318", "-v", cfg+":/etc/otelcol/config.yaml:ro", "-v", out+":/out", collectorImg, "--config=/etc/otelcol/config.yaml")
	endpoint := "http://" + host
	waitHTTP(t, endpoint+"/v1/logs")
	root := minimalProjectIn(t, lingeringTempDir(t), "\n[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \""+endpoint+"\"\n")
	writeTree(t, root, map[string]string{"CLAUDE.md": "# Project\n"})
	env := newIsoEnv(t)
	env.set("CLAUDE_PROJECT_DIR", root)
	record := func(session string) {
		t.Helper()
		ev := fmt.Sprintf(`{"hook_event_name":"InstructionsLoaded","session_id":"secret-session-%s","cwd":%q,"file_path":%q,"memory_type":"Project","load_reason":"session_start","prompt":"TOPSECRET"}`,
			session, root, filepath.Join(root, "CLAUDE.md"))
		res := env.runStdin(root, ev, "telemetry", "record")
		require.Equal(t, 0, res.ExitCode, res.Stderr)
	}
	logs := filepath.Join(out, "logs.json")

	// Act 1: the repository asks for export; the user has not consented.
	record("a")
	refused := env.run(root, "telemetry", "flush")
	time.Sleep(time.Second)
	idsBefore, _ := collectorEventIDs(t, logs)

	// Act 2: the user consents to this collector, a load is recorded and flushed.
	enable := env.run(root, "telemetry", "enable", "--endpoint", endpoint)
	require.Equal(t, 0, enable.ExitCode, enable.Stderr)
	record("b")
	flush := env.run(root, "telemetry", "flush")
	var ids []string
	var keys map[string]bool
	require.Eventually(t, func() bool { ids, keys = collectorEventIDs(t, logs); return len(ids) > 0 }, 20*time.Second, 300*time.Millisecond)
	status := env.run(root, "telemetry", "status", "--format", "json")

	// Assert
	assert.NotEqual(t, 0, refused.ExitCode, "flush without user consent does not export: %s", refused.Stdout)
	assert.Empty(t, idsBefore, "nothing reached the collector before consent")
	require.Equal(t, 0, flush.ExitCode, flush.Stderr)
	assert.Len(t, ids, 1, "exactly the one event recorded after consent")
	for _, k := range []string{"ai_rulez.session", "ai_rulez.item.path"} {
		assert.False(t, keys[k], "%s is withheld by default", k)
	}
	data, err := os.ReadFile(logs) //nolint:gosec // test file
	require.NoError(t, err)
	assert.NotContains(t, string(data), "TOPSECRET")
	assert.NotContains(t, string(data), "secret-session")
	assert.NotContains(t, string(data), root, "no absolute path leaves the machine")
	require.Equal(t, 0, status.ExitCode, status.Stderr)
	var st struct {
		Delivery struct {
			Failures int `json:"failures"`
		} `json:"delivery"`
	}
	require.NoError(t, json.Unmarshal([]byte(status.Stdout), &st), status.Stdout)
	assert.Zero(t, st.Delivery.Failures)
}
