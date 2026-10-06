package oci

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLive_RegistryV2RoundTrip pushes to and pulls from a real `registry:2`
// container. It is opt-in (AI_RULEZ_LIVE_PUBLISH=1) because it needs Docker and
// the registry image.
func TestLive_RegistryV2RoundTrip(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_PUBLISH") != "1" {
		t.Skip("set AI_RULEZ_LIVE_PUBLISH=1 to run against a registry:2 container")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is not installed")
	}
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	const port = "5173"
	name := fmt.Sprintf("ai-rulez-registry-%d", os.Getpid())
	out, err := exec.Command(docker, "run", "-d", "--rm", "--name", name, "-p", "127.0.0.1:"+port+":5000", "registry:2").CombinedOutput() //nolint:gosec // fixed argv in an opt-in test
	require.NoError(t, err, string(out))
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", name).Run() }) //nolint:gosec // fixed argv in an opt-in test
	host := "127.0.0.1:" + port
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + host + "/v2/") //nolint:noctx // readiness probe
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond, "registry did not come up")

	packed, err := Pack(context.Background(), artifact())
	require.NoError(t, err)
	digest, err := Push(context.Background(), Target{Ref: host + "/acme/skills:1.0.0"}, artifact())
	require.NoError(t, err)
	got, err := Pull(context.Background(), Target{Ref: host + "/acme/skills@" + digest})

	require.NoError(t, err)
	assert.Equal(t, packed.Digest, digest, "the registry reports the digest the plan recorded")
	assert.Equal(t, artifact().Config, got.Config)
	assert.True(t, strings.HasPrefix(got.Layers[0].Title, "acme-"))
}
