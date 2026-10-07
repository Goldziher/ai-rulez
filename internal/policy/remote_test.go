package policy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

const remoteBody = "policy_version = 1\nname = \"remote\"\n[lock]\nenforce = true\n"

var epoch = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// remoteFixture is a TLS policy server and a user home for the cache.
type remoteFixture struct {
	srv  *httptest.Server
	home string
	body atomic.Value
	hits atomic.Int32
	down atomic.Bool
}

func newRemote(t *testing.T) *remoteFixture {
	t.Helper()
	f := &remoteFixture{home: t.TempDir()}
	f.body.Store(remoteBody)
	mux := http.NewServeMux()
	mux.HandleFunc("/policy.toml", func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		if f.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(f.body.Load().(string)))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/policy.toml", http.StatusFound) })
	mux.HandleFunc("/big.toml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("#", maxPolicyBytes+10)))
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *remoteFixture) url(path string) string { return f.srv.URL + path }

func (f *remoteFixture) pinned(path string) string {
	return f.url(path) + "@" + digest([]byte(remoteBody))
}

func (f *remoteFixture) opts(flag string) DiscoverOptions {
	return DiscoverOptions{
		Flag: flag, HTTPClient: f.srv.Client(), Clock: ambient.Fixed(epoch),
		Env: ambient.MapEnv{Vars: map[string]string{}, Home: f.home}, ManagedPaths: []string{filepath.Join(f.home, "none.toml")},
	}
}

func TestParseRef(t *testing.T) {
	hex := strings.Repeat("ab", 32)
	tests := []struct {
		name, raw, pin string
		want           Ref
		wantErr        string
	}{
		{"a path", "/etc/p.toml", "", Ref{Location: "/etc/p.toml"}, ""},
		{"a pinned path", "/etc/p.toml@sha256:" + hex, "", Ref{Location: "/etc/p.toml", Digest: "sha256:" + hex}, ""},
		{"a URL", "https://p.example.org/p.toml", "", Ref{Location: "https://p.example.org/p.toml", Remote: true}, ""},
		{"a pinned URL", "https://p.example.org/p.toml@sha256:" + strings.ToUpper(hex), "", Ref{Location: "https://p.example.org/p.toml", Digest: "sha256:" + hex, Remote: true}, ""},
		{"a separate pin", "https://p.example.org/p.toml", "sha256:" + hex, Ref{Location: "https://p.example.org/p.toml", Digest: "sha256:" + hex, Remote: true}, ""},
		{"agreeing pins", "https://p.example.org/p.toml@sha256:" + hex, "sha256:" + hex, Ref{Location: "https://p.example.org/p.toml", Digest: "sha256:" + hex, Remote: true}, ""},
		{"conflicting pins", "https://p.example.org/p.toml@sha256:" + hex, "sha256:" + strings.Repeat("cd", 32), Ref{}, "AR741"},
		{"a malformed separate pin", "https://p.example.org/p.toml", "sha256:abc", Ref{}, "not a digest"},
		{"http is refused", "http://p.example.org/p.toml", "", Ref{}, "must be https"},
		{"file is refused", "file:///etc/p.toml", "", Ref{}, "must be https"},
		{"ssh is refused", "ssh://host/p.toml", "", Ref{}, "must be https"},
		{"credentials are refused", "https://user:pw@p.example.org/p.toml", "", Ref{}, "credentials"},
		{"a user name is a credential too", "https://token@p.example.org/p.toml", "", Ref{}, "credentials"},
		{"no host", "https:///p.toml", "", Ref{}, "needs a host"},
		{"empty", "  ", "", Ref{}, "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := ParseRef(tt.raw, tt.pin)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "pw@", "credentials are never echoed")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRefDisplayDropsTheQuery(t *testing.T) {
	// Arrange
	ref, err := ParseRef("https://p.example.org/p.toml?token=s3cret#frag", "")
	require.NoError(t, err)
	// Act and Assert
	assert.Equal(t, "https://p.example.org/p.toml", ref.Display())
}

func TestDiscoverFetchesAPinnedURL(t *testing.T) {
	// Arrange
	f := newRemote(t)
	// Act
	layers, err := Discover(f.opts(f.pinned("/policy.toml")))
	// Assert
	require.NoError(t, err)
	require.Len(t, layers, 1)
	assert.Equal(t, OriginFlag, layers[0].Origin)
	assert.Equal(t, f.url("/policy.toml"), layers[0].Path)
	assert.Equal(t, digest([]byte(remoteBody)), layers[0].Digest)
	assert.Equal(t, "remote", layers[0].Name)
	assert.True(t, layers[0].Policy.Lock.Enforce)
	assert.Empty(t, layers[0].Note)
}

func TestDiscoverSeparateDigestFlagAndEnv(t *testing.T) {
	// Arrange
	f := newRemote(t)
	o := f.opts(f.url("/policy.toml"))
	o.FlagDigest = digest([]byte(remoteBody))
	envOpts := f.opts("")
	envOpts.Env = ambient.MapEnv{Vars: map[string]string{EnvPolicy: f.url("/policy.toml"), EnvPolicyDigest: digest([]byte(remoteBody))}, Home: f.home}
	// Act
	byFlag, flagErr := Discover(o)
	byEnv, envErr := Discover(envOpts)
	// Assert
	require.NoError(t, flagErr)
	require.NoError(t, envErr)
	assert.Equal(t, OriginFlag, byFlag[0].Origin)
	assert.Equal(t, OriginEnv, byEnv[0].Origin)
}

func TestDiscoverURLFailures(t *testing.T) {
	f := newRemote(t)
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(remoteBody)) }))
	t.Cleanup(other.Close)
	hop := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/policy.toml", http.StatusFound)
	}))
	t.Cleanup(hop.Close)
	tests := []struct {
		name    string
		flag    string
		client  *http.Client
		wantErr string
		errType any
	}{
		{"no digest", f.url("/policy.toml"), nil, "AR741", &DigestError{}},
		{"wrong digest", f.url("/policy.toml") + "@sha256:" + strings.Repeat("0", 64), nil, "AR741", &DigestError{}},
		{"http scheme", strings.Replace(f.url("/policy.toml"), "https://", "http://", 1), nil, "AR743", &ParseError{}},
		{"oversized body", f.pinned("/big.toml"), nil, "larger than 256 KiB", &ParseError{}},
		{"server error with no cache", f.pinned("/missing.toml"), nil, "AR742", &UnavailableError{}},
		{"redirect to another host is refused", hop.URL + "/x@" + digest([]byte(remoteBody)), hop.Client(), "AR742", &UnavailableError{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			o := f.opts(tt.flag)
			if tt.client != nil {
				o.HTTPClient = tt.client
			}
			// Act
			_, err := Discover(o)
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			switch tt.errType.(type) {
			case *DigestError:
				var e *DigestError
				assert.True(t, errors.As(err, &e))
			case *ParseError:
				var e *ParseError
				assert.True(t, errors.As(err, &e))
			case *UnavailableError:
				var e *UnavailableError
				assert.True(t, errors.As(err, &e))
			}
		})
	}
}

func TestDiscoverFollowsASameHostRedirect(t *testing.T) {
	// Arrange
	f := newRemote(t)
	// Act
	layers, err := Discover(f.opts(f.pinned("/redirect")))
	// Assert
	require.NoError(t, err)
	assert.Len(t, layers, 1)
}

func TestDiscoverURLCacheAndMaxStale(t *testing.T) {
	// Arrange: a first online load fills the cache.
	f := newRemote(t)
	_, err := Discover(f.opts(f.pinned("/policy.toml")))
	require.NoError(t, err)
	f.down.Store(true)
	tests := []struct {
		name      string
		advance   time.Duration
		maxStale  string
		offline   bool
		wantNote  string
		wantErr   string
		wantFetch bool
	}{
		{"unreachable, cached copy is fresh enough", time.Hour, "", false, "cached copy fetched 2026-10-06T12:00:00Z", "", true},
		{"unreachable, cached copy is exactly at max_stale", 7 * 24 * time.Hour, "", false, "cached copy", "", true},
		{"unreachable, cached copy is too old", 7*24*time.Hour + time.Minute, "", false, "", "older than max_stale (7d)", true},
		{"a shorter max_stale", 2 * time.Hour, "1h", false, "", "older than max_stale (1h)", true},
		{"a longer max_stale", 20 * 24 * time.Hour, "30d", false, "cached copy", "", true},
		{"max_stale 0 allows no stale copy", time.Minute, "0", false, "", "older than max_stale (0)", true},
		{"offline uses the cache without asking the network", time.Hour, "", true, "cached copy", "", false},
		{"offline and too old fails closed", 8 * 24 * time.Hour, "", true, "", "AR742", false},
		// RV-GOV-8: fetched while the clock ran 30 days ahead, read after it was corrected.
		{"a copy stamped in the future is not used", -25 * 24 * time.Hour, "1h", false, "", "in the future", true},
		{"offline, a copy stamped in the future is not used", -25 * 24 * time.Hour, "", true, "", "in the future", false},
		{"a stamp within the clock skew is used", -time.Minute, "1h", false, "cached copy", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			before := f.hits.Load()
			o := f.opts(f.pinned("/policy.toml"))
			o.Clock = ambient.Fixed(epoch.Add(tt.advance))
			o.MaxStale, o.Offline = tt.maxStale, tt.offline
			// Act
			layers, err := Discover(o)
			// Assert
			assert.Equal(t, tt.wantFetch, f.hits.Load() > before, "network use")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), "AR742")
				return
			}
			require.NoError(t, err)
			require.Len(t, layers, 1)
			assert.Contains(t, layers[0].Note, tt.wantNote)
			assert.True(t, layers[0].Policy.Lock.Enforce)
		})
	}
}

func TestDiscoverDigestMismatchIsNotMaskedByTheCache(t *testing.T) {
	// Arrange: the cache holds the pinned copy, then the server changes its content.
	f := newRemote(t)
	pin := f.pinned("/policy.toml")
	_, err := Discover(f.opts(pin))
	require.NoError(t, err)
	f.body.Store(strings.Replace(remoteBody, "enforce = true", "enforce = false", 1))
	// Act
	_, err = Discover(f.opts(pin))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR741")
}

func TestDiscoverIgnoresATamperedCache(t *testing.T) {
	// Arrange
	f := newRemote(t)
	pin := f.pinned("/policy.toml")
	_, err := Discover(f.opts(pin))
	require.NoError(t, err)
	f.down.Store(true)
	dir := filepath.Join(f.home, ".cache", "ai-rulez", "policy")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(t *testing.T)
	}{
		{"the body is replaced", func(t *testing.T) {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".toml") {
					require.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), []byte("policy_version = 1\n"), 0o600))
				}
			}
		}},
		{"the fetch time is moved forward", func(t *testing.T) {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".json") && e.Name() != tofuFile {
					p := filepath.Join(dir, e.Name())
					data, err := os.ReadFile(p)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(data), "2026-10-06T12:00:00Z", "2030-01-01T00:00:00Z", 1)), 0o600))
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: restore a good cache, then tamper.
			f.down.Store(false)
			_, err := Discover(f.opts(pin))
			require.NoError(t, err)
			f.down.Store(true)
			entries, err = os.ReadDir(dir)
			require.NoError(t, err)
			tt.mutate(t)
			// Act
			_, err = Discover(f.opts(pin))
			// Assert
			require.Error(t, err, "a cache entry that fails its HMAC is a miss")
			assert.Contains(t, err.Error(), "AR742")
			assert.Contains(t, err.Error(), "no cached copy")
		})
	}
}

func TestDiscoverTrustOnFirstUse(t *testing.T) {
	// Arrange
	f := newRemote(t)
	url := f.url("/policy.toml")
	interactive := func(o DiscoverOptions) DiscoverOptions { o.TrustOnFirstUse, o.Interactive = true, true; return o }

	t.Run("refused without a terminal", func(t *testing.T) {
		o := f.opts(url)
		o.TrustOnFirstUse = true
		_, err := Discover(o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interactive terminal")
	})
	t.Run("offline cannot trust for the first time", func(t *testing.T) {
		o := interactive(f.opts(url))
		o.Offline = true
		_, err := Discover(o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742")
	})
	t.Run("records the digest and later loads need no flag", func(t *testing.T) {
		layers, err := Discover(interactive(f.opts(url)))
		require.NoError(t, err)
		assert.Equal(t, digest([]byte(remoteBody)), layers[0].Digest)
		again, err := Discover(f.opts(url))
		require.NoError(t, err, "the recorded digest pins it from now on")
		assert.Equal(t, layers[0].Digest, again[0].Digest)
	})
	t.Run("a changed policy no longer matches the record", func(t *testing.T) {
		f.body.Store(strings.Replace(remoteBody, "enforce = true", "enforce = false", 1))
		_, err := Discover(f.opts(url))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
	})
}

func TestDiscoverTOFURecordIsAuthenticated(t *testing.T) {
	// Arrange: record, then forge the record to another digest.
	f := newRemote(t)
	url := f.url("/policy.toml")
	o := f.opts(url)
	o.TrustOnFirstUse, o.Interactive = true, true
	_, err := Discover(o)
	require.NoError(t, err)
	record := filepath.Join(f.home, ".cache", "ai-rulez", "policy", tofuFile)
	data, err := os.ReadFile(record)
	require.NoError(t, err)
	forged := strings.Replace(string(data), digest([]byte(remoteBody)), "sha256:"+strings.Repeat("0", 64), 1)
	require.NoError(t, os.WriteFile(record, []byte(forged), 0o600))
	// Act
	_, err = Discover(f.opts(url))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no digest", "a record that fails its HMAC is no record")
}

func TestDiscoverPinnedFile(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := writePolicy(t, dir, "p.toml", minimalPolicy)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	good := digest(data)
	opts := func(flag string) DiscoverOptions {
		return DiscoverOptions{Flag: flag, Env: ambient.MapEnv{Home: t.TempDir()}, ManagedPaths: []string{filepath.Join(dir, "none.toml")}}
	}
	// Act
	layers, goodErr := Discover(opts(path + "@" + good))
	_, badErr := Discover(opts(path + "@sha256:" + strings.Repeat("1", 64)))
	// Assert
	require.NoError(t, goodErr)
	assert.Equal(t, good, layers[0].Digest)
	require.Error(t, badErr)
	assert.Contains(t, badErr.Error(), "AR741")
}

func TestMaxStaleParsing(t *testing.T) {
	tests := []struct {
		raw, env string
		want     time.Duration
		wantErr  bool
	}{
		{"", "", DefaultMaxStale, false},
		{"3d", "", 72 * time.Hour, false},
		{"36h", "", 36 * time.Hour, false},
		{"0", "", -1, false},
		{"", "2d", 48 * time.Hour, false},
		{"1d", "2d", 24 * time.Hour, false},
		{"soon", "", 0, true},
		{"-1d", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw+"/"+tt.env, func(t *testing.T) {
			// Arrange
			o := DiscoverOptions{MaxStale: tt.raw, Env: ambient.MapEnv{Vars: map[string]string{EnvPolicyMaxStale: tt.env}}}
			// Act
			got, err := o.maxStale()
			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "AR743")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOfflineFromTheEnvironment(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes"} {
		assert.True(t, DiscoverOptions{Env: ambient.MapEnv{Vars: map[string]string{EnvPolicyOffline: v}}}.offline(), v)
	}
	for _, v := range []string{"", "0", "false", "no"} {
		assert.False(t, DiscoverOptions{Env: ambient.MapEnv{Vars: map[string]string{EnvPolicyOffline: v}}}.offline(), v)
	}
	assert.True(t, DiscoverOptions{Offline: true}.offline())
}
