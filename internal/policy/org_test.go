package policy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const orgBody = "policy_version = 1\nname = \"example-org\"\n[lock]\nenforce = true\n"

func TestOwnerFromRemote(t *testing.T) {
	tests := []struct {
		remote  string
		want    string
		wantErr string
	}{
		{"https://github.com/Example-Org/repo.git", "example-org", ""},
		{"https://github.com/example-org/repo", "example-org", ""},
		{"git@github.com:example-org/repo.git", "example-org", ""},
		{"ssh://git@github.com:22/example-org/repo.git", "example-org", ""},
		{"https://GITHUB.com/example-org/repo", "example-org", ""},
		{"https://gitlab.com/example-org/repo", "", "supports github.com only"},
		{"git@gitlab.com:o/r.git", "", "supports github.com only"},
		{"https://github.com.evil.test/o/r", "", "supports github.com only"},
		{"https://github.com/", "", "no valid GitHub owner"},
		{"https://github.com/-bad/repo", "", "no valid GitHub owner"},
		{"https://github.com/" + strings.Repeat("a", 40) + "/r", "", "no valid GitHub owner"},
		{"/local/path/repo", "", "not a GitHub URL"},
		{"", "", "not a GitHub URL"},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			// Act
			got, err := ownerFromRemote(tt.remote)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// userEnv is an environment whose user config is body ("" for none).
func userEnv(t *testing.T, body string) ambient.Env {
	t.Helper()
	home := t.TempDir()
	cfgHome := filepath.Join(home, "xdg")
	if body != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(cfgHome, "ai-rulez"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(cfgHome, "ai-rulez", "config.toml"), []byte(body), 0o600))
	}
	return ambient.MapEnv{Vars: map[string]string{"XDG_CONFIG_HOME": cfgHome}, Home: home}
}

func TestLoadUserSettings(t *testing.T) {
	hex := strings.Repeat("ab", 32)
	tests := []struct {
		name    string
		body    string
		want    UserSettings
		wantErr string
	}{
		{"no file", "", UserSettings{}, ""},
		{"no policy table", "[llm]\nallow_network = true\n", UserSettings{}, ""},
		{"discover org", "[policy]\ndiscover = \"ORG\"\n", UserSettings{Discover: "org"}, ""},
		{"digests", "[policy]\ndiscover = \"org\"\n[policy.digests]\nExample-Org = \"sha256:" + strings.ToUpper(hex) + "\"\n",
			UserSettings{Discover: "org", Digests: map[string]string{"example-org": "sha256:" + hex}}, ""},
		{"unknown discover mode", "[policy]\ndiscover = \"team\"\n", UserSettings{}, "not supported"},
		{"bad digest", "[policy.digests]\nacme = \"abc\"\n", UserSettings{}, "want an owner name and sha256"},
		{"bad owner", "[policy.digests]\n\"a b\" = \"sha256:" + hex + "\"\n", UserSettings{}, "want an owner name and sha256"},
		{"not toml", "[policy\n", UserSettings{}, "not valid TOML"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := LoadUserSettings(userEnv(t, tt.body))
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// orgFixture serves <owner>/.github/HEAD/ai-rulez-policy.toml.
type orgFixture struct {
	srv  *httptest.Server
	hits atomic.Int32
	body atomic.Value
	code atomic.Int32
}

func newOrg(t *testing.T) *orgFixture {
	t.Helper()
	f := &orgFixture{}
	f.body.Store(orgBody)
	f.code.Store(http.StatusOK)
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.URL.Path != "/example-org/.github/HEAD/ai-rulez-policy.toml" || int(f.code.Load()) != http.StatusOK {
			code := int(f.code.Load())
			if code == http.StatusOK {
				code = http.StatusNotFound
			}
			http.Error(w, "nope", code)
			return
		}
		_, _ = w.Write([]byte(f.body.Load().(string)))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *orgFixture) opts(t *testing.T, userConfig string) DiscoverOptions {
	t.Helper()
	return DiscoverOptions{
		Env: userEnv(t, userConfig), HTTPClient: f.srv.Client(), OrgRawBase: f.srv.URL, Clock: ambient.Fixed(epoch),
		ProjectDir: t.TempDir(), RemoteURL: func(string) (string, error) { return "git@github.com:example-org/repo.git", nil },
		ManagedPaths: []string{filepath.Join(t.TempDir(), "none.toml")},
	}
}

func orgConfig(extra string) string {
	return "[policy]\ndiscover = \"org\"\n[policy.digests]\nexample-org = \"" + digest([]byte(orgBody)) + "\"\n" + extra
}

func TestOrgRef(t *testing.T) {
	f := newOrg(t)
	t.Run("derives the owner's .github policy", func(t *testing.T) {
		ref, ok, err := OrgRef(f.opts(t, orgConfig("")))
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, f.srv.URL+"/example-org/.github/HEAD/"+OrgPolicyFile, ref.Location)
		assert.Equal(t, digest([]byte(orgBody)), ref.Digest, "the pin comes from the user config")
		assert.True(t, ref.Remote)
	})
	t.Run("no project means nothing to derive", func(t *testing.T) {
		o := f.opts(t, "")
		o.ProjectDir = ""
		_, ok, err := OrgRef(o)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("an underivable owner is skipped when only the user config asked", func(t *testing.T) {
		o := f.opts(t, orgConfig(""))
		o.RemoteURL = func(string) (string, error) { return "https://gitlab.com/x/y", nil }
		_, ok, err := OrgRef(o)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("an underivable owner fails closed when --discover-org demanded it", func(t *testing.T) {
		o := f.opts(t, "")
		o.DiscoverOrg = true
		o.RemoteURL = func(string) (string, error) { return "", errors.New("the repository has no origin remote") }
		_, _, err := OrgRef(o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742")
	})
}

func TestLoadOrg(t *testing.T) {
	f := newOrg(t)
	load := func(t *testing.T, userConfig string, mutate func(*DiscoverOptions)) ([]Layer, error) {
		o := f.opts(t, userConfig)
		if mutate != nil {
			mutate(&o)
		}
		ref, ok, err := OrgRef(o)
		require.NoError(t, err)
		require.True(t, ok)
		return LoadOrg(o, ref)
	}
	t.Run("a pinned org policy loads as the org layer", func(t *testing.T) {
		layers, err := load(t, orgConfig(""), nil)
		require.NoError(t, err)
		require.Len(t, layers, 1)
		assert.Equal(t, OriginOrg, layers[0].Origin)
		assert.Equal(t, "example-org", layers[0].Name)
		assert.True(t, layers[0].Policy.Lock.Enforce)
	})
	t.Run("without a digest it is not loaded", func(t *testing.T) {
		_, err := load(t, "[policy]\ndiscover = \"org\"\n", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
		assert.Contains(t, err.Error(), "[policy.digests]")
	})
	t.Run("a changed org policy no longer matches the pin", func(t *testing.T) {
		f.body.Store(orgBody + "[guard]\ngenerated = true\n")
		t.Cleanup(func() { f.body.Store(orgBody) })
		_, err := load(t, orgConfig(""), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
	})
	t.Run("a pinned org policy that answers 404 fails closed", func(t *testing.T) {
		f.code.Store(http.StatusNotFound)
		t.Cleanup(func() { f.code.Store(http.StatusOK) })
		layers, err := load(t, orgConfig(""), nil)
		require.Error(t, err, "a pin means a policy must exist")
		assert.Contains(t, err.Error(), "AR742")
		assert.Empty(t, layers)
	})
	t.Run("a 404 stays AR742 when the cached copy is expired", func(t *testing.T) {
		o := f.opts(t, orgConfig(""))
		ref, _, err := OrgRef(o)
		require.NoError(t, err)
		_, err = LoadOrg(o, ref) // fills the cache
		require.NoError(t, err)
		f.code.Store(http.StatusNotFound)
		t.Cleanup(func() { f.code.Store(http.StatusOK) })
		o.MaxStale = "1h"
		o.Clock = ambient.Fixed(epoch.Add(48 * time.Hour))
		_, err = LoadOrg(o, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742")
	})
	t.Run("an unpinned owner with no policy file has no org policy", func(t *testing.T) {
		f.code.Store(http.StatusNotFound)
		t.Cleanup(func() { f.code.Store(http.StatusOK) })
		o := f.opts(t, "[policy]\ndiscover = \"org\"\n")
		o.TrustOnFirstUse, o.Interactive = true, true
		ref, _, err := OrgRef(o)
		require.NoError(t, err)
		layers, err := LoadOrg(o, ref)
		require.NoError(t, err)
		assert.Empty(t, layers)
	})
	t.Run("a server error fails closed", func(t *testing.T) {
		f.code.Store(http.StatusBadGateway)
		t.Cleanup(func() { f.code.Store(http.StatusOK) })
		_, err := load(t, orgConfig(""), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR742")
	})
	t.Run("trust on first use records the digest", func(t *testing.T) {
		o := f.opts(t, "[policy]\ndiscover = \"org\"\n")
		o.TrustOnFirstUse, o.Interactive = true, true
		ref, _, err := OrgRef(o)
		require.NoError(t, err)
		layers, err := LoadOrg(o, ref)
		require.NoError(t, err)
		assert.Len(t, layers, 1)
		o.TrustOnFirstUse, o.Interactive = false, false
		again, err := LoadOrg(o, ref)
		require.NoError(t, err, "the recorded digest pins it from now on")
		assert.Len(t, again, 1)
	})
}

func TestEnforcerLoadForAddsTheOrgLayerOncePerOwner(t *testing.T) {
	// Arrange: a flag policy, org discovery from the user config.
	f := newOrg(t)
	flag := writePolicy(t, t.TempDir(), "flag.toml", "policy_version = 1\nname = \"flag\"\n[lint.severity_floor]\nAR001 = \"error\"\n")
	base := f.opts(t, orgConfig(""))
	base.Flag = flag
	e := NewEnforcer(func() DiscoverOptions { return base })
	// Act
	first, err1 := e.LoadFor(t.TempDir())
	second, err2 := e.LoadFor(t.TempDir())
	plain, err3 := e.Load()
	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	require.Len(t, first.Layers, 2)
	assert.Equal(t, []string{OriginFlag, OriginOrg}, []string{first.Layers[0].Origin, first.Layers[1].Origin}, "the org layer is the weakest")
	assert.True(t, first.Policy.Lock.Enforce)
	assert.Equal(t, "error", first.Policy.Lint.SeverityFloor["AR001"])
	assert.Equal(t, int32(1), f.hits.Load(), "one fetch for two repositories of the same owner")
	assert.Len(t, second.Layers, 2)
	assert.Len(t, plain.Layers, 1, "Load does not know the repository, so it has no org layer")
	assert.False(t, plain.Policy.Lock.Enforce)
}

func TestEnforcerEnforcesTheOrgLayerOnTheRepository(t *testing.T) {
	// Arrange
	f := newOrg(t)
	base := f.opts(t, orgConfig(""))
	e := NewEnforcer(func() DiscoverOptions { return base })
	cfg := &config.Config{BaseDir: t.TempDir(), Lock: &config.LockConfig{Enforce: boolPtr(false)}}
	// Act
	out, err := e.Enforce(context.Background(), cfg)
	// Assert
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Contains(t, codes(out), "AR740 lock.enforce")
	assert.True(t, *cfg.Lock.Enforce)
	assert.Equal(t, "org", out.Violations[0].Origin)
}

func TestEnforcerWithoutDiscoveryNeverTouchesTheNetwork(t *testing.T) {
	// Arrange: a remote that would resolve, but discovery is off.
	f := newOrg(t)
	base := f.opts(t, "")
	e := NewEnforcer(func() DiscoverOptions { return base })
	// Act
	res, err := e.LoadFor(t.TempDir())
	// Assert
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Zero(t, f.hits.Load())
}

func TestEnforcerOrgFailureFailsClosed(t *testing.T) {
	// Arrange: demanded on the command line, but the digest is missing.
	f := newOrg(t)
	base := f.opts(t, "")
	base.DiscoverOrg = true
	e := NewEnforcer(func() DiscoverOptions { return base })
	// Act
	_, err := e.Enforce(context.Background(), &config.Config{BaseDir: t.TempDir()})
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR741")
}

func TestOriginURLReadsTheRealOriginRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	// Arrange
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:example-org/repo.git"}} {
		cmd := gitutil.CommandNoContext(dir, append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	// Act
	got, err := originURL(dir)
	_, noRemote := originURL(t.TempDir())
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:example-org/repo.git", got)
	require.Error(t, noRemote)
}
