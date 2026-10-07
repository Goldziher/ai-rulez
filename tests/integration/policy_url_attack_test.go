package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

// policyServer serves policy files from a map over TLS; the map can change
// between requests, like a compromised or careless policy host.
type policyServer struct {
	mu    sync.Mutex
	files map[string]string
	srv   *httptest.Server
}

func newPolicyServer(t *testing.T, files map[string]string) *policyServer {
	t.Helper()
	p := &policyServer{files: files}
	p.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		body, ok := p.files[r.URL.Path]
		p.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *policyServer) set(path, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.files[path] = body
}

func (p *policyServer) pinned(path string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srv.URL + path + "@" + policyDigest(p.files[path])
}

func policyDigest(body string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(body, "\r\n", "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func discoverURL(t *testing.T, p *policyServer, home, flag string, clock time.Time) ([]policy.Layer, error) {
	t.Helper()
	return policy.Discover(policy.DiscoverOptions{
		Flag: flag, HTTPClient: p.srv.Client(), Clock: ambient.Fixed(clock),
		Env:          ambient.MapEnv{Vars: map[string]string{}, Home: home},
		ManagedPaths: []string{filepath.Join(home, "no-managed-policy.toml")},
	})
}

const basePolicy = "policy_version = 1\nname = \"base\"\n\n[lock]\nenforce = true\n\n[llm]\nallow_network = false\n"

// TestMaliciousURLPolicy is the manual pass's "malicious URL policy" attack
// repo, kept as a test: a policy fetched over https tries to reach outside
// what a URL policy may name, or to change after it was pinned.
func TestMaliciousURLPolicy(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	localParent := filepath.Join(t.TempDir(), "local.toml")
	require.NoError(t, os.WriteFile(localParent, []byte(basePolicy), 0o600))

	tests := []struct {
		name    string
		files   map[string]string
		flag    func(p *policyServer) string
		wantErr string
	}{
		{
			name:    "a URL policy cannot extend a local file",
			files:   map[string]string{"/team.toml": "policy_version = 1\nname = \"team\"\nextends = [\"" + localParent + "\"]\n"},
			flag:    func(p *policyServer) string { return p.pinned("/team.toml") },
			wantErr: "AR74",
		},
		{
			name:    "a URL policy cannot extend a relative path that climbs out",
			files:   map[string]string{"/team.toml": "policy_version = 1\nname = \"team\"\nextends = [\"../../../etc/ai-rulez/policy.toml\"]\n"},
			flag:    func(p *policyServer) string { return p.pinned("/team.toml") },
			wantErr: "AR74",
		},
		{
			name:    "a parent over plain http is refused",
			files:   map[string]string{"/team.toml": "policy_version = 1\nname = \"team\"\nextends = [\"http://policy.example.org/base.toml@" + policyDigest(basePolicy) + "\"]\n"},
			flag:    func(p *policyServer) string { return p.pinned("/team.toml") },
			wantErr: "https",
		},
		{
			name:    "an unpinned URL parent is refused",
			files:   map[string]string{"/base.toml": basePolicy},
			flag:    nil, // set below: the child names the server's own unpinned base
			wantErr: "AR741",
		},
		{
			name:    "a parent whose content differs from its pin fails closed",
			files:   map[string]string{"/base.toml": basePolicy + "# tampered\n"},
			flag:    nil,
			wantErr: "AR741",
		},
		{
			name:    "a child that loosens its parent is refused",
			files:   map[string]string{"/base.toml": basePolicy},
			flag:    nil,
			wantErr: "AR743",
		},
		{
			name:    "a cycle through pinned URLs cannot close and fails closed",
			files:   map[string]string{},
			flag:    nil,
			wantErr: "AR741",
		},
		{
			name:    "an oversized policy is refused",
			files:   map[string]string{"/big.toml": "policy_version = 1\n" + strings.Repeat("#", 300*1024) + "\n"},
			flag:    func(p *policyServer) string { return p.pinned("/big.toml") },
			wantErr: "AR74",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newPolicyServer(t, tt.files)
			flag := tt.flag
			switch tt.name {
			case "an unpinned URL parent is refused":
				p.set("/team.toml", "policy_version = 1\nname = \"team\"\nextends = [\""+p.srv.URL+"/base.toml\"]\n")
				flag = func(p *policyServer) string { return p.pinned("/team.toml") }
			case "a parent whose content differs from its pin fails closed":
				p.set("/team.toml", "policy_version = 1\nname = \"team\"\nextends = [\""+p.srv.URL+"/base.toml@"+policyDigest(basePolicy)+"\"]\n")
				flag = func(p *policyServer) string { return p.pinned("/team.toml") }
			case "a child that loosens its parent is refused":
				p.set("/team.toml", "policy_version = 1\nname = \"team\"\nextends = [\""+p.pinned("/base.toml")+"\"]\n\n[lock]\nenforce = false\n")
				flag = func(p *policyServer) string { return p.pinned("/team.toml") }
			case "a cycle through pinned URLs cannot close and fails closed":
				// a.toml pins b.toml, then b.toml is rewritten to extend a.toml: the
				// pins are content addresses, so the rewritten b no longer matches.
				p.set("/b.toml", "policy_version = 1\nname = \"b\"\n")
				a := "policy_version = 1\nname = \"a\"\nextends = [\"" + p.pinned("/b.toml") + "\"]\n"
				p.set("/a.toml", a)
				p.set("/b.toml", "policy_version = 1\nname = \"b\"\nextends = [\""+p.pinned("/a.toml")+"\"]\n")
				flag = func(p *policyServer) string { return p.pinned("/a.toml") }
			}

			// Act
			layers, err := discoverURL(t, p, t.TempDir(), flag(p), now)

			// Assert
			require.Error(t, err, "loaded %d layer(s)", len(layers))
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestURLPolicyCacheCannotOutliveItsPinOrMaxStale: a cached copy stands in for
// an unreachable host only for its pinned digest and only within max_stale.
func TestURLPolicyCacheCannotOutliveItsPinOrMaxStale(t *testing.T) {
	// Arrange
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	p := newPolicyServer(t, map[string]string{"/base.toml": basePolicy})
	home := t.TempDir()
	flag := p.pinned("/base.toml")
	_, err := discoverURL(t, p, home, flag, now)
	require.NoError(t, err, "first fetch fills the cache")
	p.mu.Lock()
	delete(p.files, "/base.toml") // the host now answers 404
	p.mu.Unlock()

	// Act
	fresh, freshErr := discoverURL(t, p, home, flag, now.Add(24*time.Hour))
	stale, staleErr := discoverURL(t, p, home, flag, now.Add(8*24*time.Hour))
	otherPin, otherErr := discoverURL(t, p, home, p.srv.URL+"/base.toml@"+policyDigest(basePolicy+"# other\n"), now)

	// Assert
	require.NoError(t, freshErr, "within max_stale the cached copy stands in")
	require.Len(t, fresh, 1)
	assert.NotEmpty(t, fresh[0].Note, "the layer says it came from the cache")
	require.Error(t, staleErr, "past max_stale the run fails closed (%d layers)", len(stale))
	assert.Contains(t, staleErr.Error(), "AR742")
	require.Error(t, otherErr, "a cached copy never stands in for another pin (%d layers)", len(otherPin))
}
