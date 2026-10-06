package oci

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func artifact() Artifact {
	return Artifact{
		Config: []byte(`{"schema_version":1,"name":"acme"}` + "\n"),
		Layers: []Layer{
			{MediaType: BundleMediaType, Title: "acme-1.0.0.tar.gz", Data: []byte("tarball bytes")},
			{MediaType: LockMediaType, Title: "ai-rulez.lock", Data: []byte("version = 2\n")},
		},
		Version: "1.0.0", Source: "https://github.com/acme/skills", Revision: "0f3e", Created: 1700000000,
	}
}

// localRegistry starts an in-memory OCI registry on a loopback address and
// returns its host:port.
func localRegistry(t *testing.T) string {
	t.Helper()
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // never read the developer's registry credentials
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestPack_IsReproducible(t *testing.T) {
	a, err := Pack(context.Background(), artifact())
	require.NoError(t, err)
	b, err := Pack(context.Background(), artifact())
	require.NoError(t, err)

	assert.Equal(t, a, b)
	assert.Contains(t, string(a.Manifest), `"artifactType":"application/vnd.ai-rulez.bundle.v1"`)
	assert.Contains(t, string(a.Manifest), `"org.opencontainers.image.created":"2023-11-14T22:13:20Z"`)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, a.Digest)
}

func TestPack_ChangesWithTheContent(t *testing.T) {
	changed := artifact()
	changed.Layers[0].Data = []byte("other bytes")

	a, err := Pack(context.Background(), artifact())
	require.NoError(t, err)
	b, err := Pack(context.Background(), changed)
	require.NoError(t, err)

	assert.NotEqual(t, a.Digest, b.Digest)
}

func TestPushPull_RoundTripsAgainstALocalRegistry(t *testing.T) {
	host := localRegistry(t)
	packed, err := Pack(context.Background(), artifact())
	require.NoError(t, err)

	digest, err := Push(context.Background(), Target{Ref: host + "/acme/skills:1.0.0"}, artifact())
	require.NoError(t, err)
	got, err := Pull(context.Background(), Target{Ref: host + "/acme/skills@" + digest})

	require.NoError(t, err)
	assert.Equal(t, packed.Digest, digest, "the pushed digest is the packed one")
	assert.Equal(t, digest, got.Digest)
	assert.Equal(t, artifact().Config, got.Config)
	require.Len(t, got.Layers, 2)
	assert.Equal(t, "acme-1.0.0.tar.gz", got.Layers[0].Title)
	assert.Equal(t, []byte("tarball bytes"), got.Layers[0].Data)
	assert.Equal(t, LockMediaType, got.Layers[1].MediaType)
}

func TestPull_ByTag(t *testing.T) {
	host := localRegistry(t)
	_, err := Push(context.Background(), Target{Ref: host + "/acme/skills:1.0.0"}, artifact())
	require.NoError(t, err)

	got, err := Pull(context.Background(), Target{Ref: host + "/acme/skills:1.0.0"})

	require.NoError(t, err)
	assert.Equal(t, "1.0.0", got.Manifest.Annotations["org.opencontainers.image.version"])
}

func TestPush_Errors(t *testing.T) {
	host := localRegistry(t)
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"no tag", host + "/acme/skills", "tag"},
		{"a digest instead of a tag", host + "/acme/skills@sha256:" + strings.Repeat("a", 64), "tag"},
		{"not a reference", "Not A Ref", "invalid OCI reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Push(context.Background(), Target{Ref: tt.ref}, artifact())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPull_MissingTag(t *testing.T) {
	host := localRegistry(t)

	_, err := Pull(context.Background(), Target{Ref: host + "/acme/skills:nope"})

	require.Error(t, err)
}

func TestIsLoopback(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1:5000": true, "localhost:5000": true, "localhost": true, "[::1]:5000": true,
		"ghcr.io": false, "registry.example.com:443": false, "10.0.0.5:5000": false,
	}
	for host, want := range tests {
		assert.Equal(t, want, isLoopback(host), host)
	}
}
