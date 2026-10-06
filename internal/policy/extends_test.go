package policy

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func extendsOpts(t *testing.T, flag string) DiscoverOptions {
	t.Helper()
	return DiscoverOptions{
		Flag: flag, Env: ambient.MapEnv{Vars: map[string]string{}, Home: t.TempDir()},
		ManagedPaths: []string{filepath.Join(t.TempDir(), "none.toml")},
	}
}

func chainFile(t *testing.T, dir, name string, extends []string, body string) string {
	t.Helper()
	list := make([]string, len(extends))
	for i, e := range extends {
		list[i] = fmt.Sprintf("%q", e)
	}
	return writePolicy(t, dir, name, fmt.Sprintf("policy_version = 1\nname = %q\nextends = [%s]\n%s", strings.TrimSuffix(name, ".toml"), strings.Join(list, ", "), body))
}

func TestExtendsFoldsTheChainTightenOnly(t *testing.T) {
	// Arrange: team extends org (relative path); each adds restrictions.
	dir := t.TempDir()
	chainFile(t, dir, "org.toml", nil, "[lint.severity_floor]\nAR001 = \"error\"\n[sources]\nallowed_hosts = [\"github.com\"]\n")
	team := chainFile(t, dir, "team.toml", []string{"org.toml"}, "[lint.severity_floor]\nAR008 = \"warning\"\n[sources]\nallowed_hosts = [\"github.com/example-org\"]\n")

	// Act
	layers, err := Discover(extendsOpts(t, team))

	// Assert
	require.NoError(t, err)
	require.Len(t, layers, 2)
	assert.Equal(t, []string{OriginFlag, OriginExtends}, []string{layers[0].Origin, layers[1].Origin})
	assert.Equal(t, []string{filepath.Join(dir, "org.toml")}, layers[0].Extends)
	res := Resolve(layers)
	assert.Equal(t, map[string]string{"AR001": "error", "AR008": "warning"}, res.Policy.Lint.SeverityFloor)
	assert.Equal(t, List{Set: true, Items: []string{"github.com/example-org"}}, res.Policy.Sources.Allowed, "the child narrows the parent's list")
	assert.Equal(t, "extends", res.Provenance["lint.severity_floor.AR001"])
	assert.Equal(t, "flag", res.Provenance["lint.severity_floor.AR008"])
	var text strings.Builder
	BuildReport(res, nil).WriteText(&text)
	assert.Contains(t, text.String(), "extends "+filepath.Join(dir, "org.toml"))
}

func TestExtendsRejectsAChildThatLoosensItsParent(t *testing.T) {
	tests := []struct {
		name         string
		parent, kid  string
		wantContains string
	}{
		{"a lower severity floor", "[lint.severity_floor]\nAR001 = \"error\"\n", "[lint.severity_floor]\nAR001 = \"warning\"\n", "lint.severity_floor.AR001"},
		{"an allowlist entry the parent does not cover", "[sources]\nallowed_hosts = [\"github.com/example-org\"]\n", "[sources]\nallowed_hosts = [\"github.com\"]\n", "sources.allowed_hosts"},
		{"a higher load budget", "[lint.load_budgets]\nclaude-skill-listing = 1000\n", "[lint.load_budgets]\nclaude-skill-listing = 2000\n", "lint.load_budgets.claude-skill-listing"},
		{"a weaker scan level", "[lint.security]\nscan_imports = \"error\"\n", "[lint.security]\nscan_imports = \"warn\"\n", "lint.security.scan_imports"},
		{"a shorter release age", "[sources]\nmin_release_age = \"7d\"\n", "[sources]\nmin_release_age = \"1d\"\n", "sources.min_release_age"},
		{"fewer approvers", "[governance]\nmin_approvers = 2\n", "[governance]\nmin_approvers = 1\n", "governance.min_approvers"},
		{"a higher ceiling", "[lint.max_findings]\nAR001 = 1\n", "[lint.max_findings]\nAR001 = 5\n", "lint.max_findings.AR001"},
		{"a reviewer the parent did not name", "[governance]\napprovers = [\"a@x.org\"]\n", "[governance]\napprovers = [\"a@x.org\", \"b@x.org\"]\n", "governance.approvers"},
		{"an MCP command the parent forbids", "[mcp]\nallowed_commands = [\"npx\"]\n", "[mcp]\nallowed_commands = [\"npx\", \"bash\"]\n", "mcp.allowed_commands"},
		{"a weaker log mode", "[signing]\ntlog = \"required\"\n", "[signing]\ntlog = \"optional\"\n", "signing.tlog"},
		{"older signatures", "[signing]\nmax_age = \"30d\"\n", "[signing]\nmax_age = \"90d\"\n", "signing.max_age"},
		{"a larger size budget", "[lint.budgets.skill]\nmax_tokens = 1000\n", "[lint.budgets.skill]\nmax_tokens = 2000\n", "lint.budgets.skill.max_tokens"},
		{"a weaker scanner preset", "[lint.scanner_policy]\npreset = \"strict\"\n", "[lint.scanner_policy]\npreset = \"baseline\"\n", "lint.scanner_policy.preset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			chainFile(t, dir, "org.toml", nil, tt.parent)
			team := chainFile(t, dir, "team.toml", []string{"org.toml"}, tt.kid)
			// Act
			_, err := Discover(extendsOpts(t, team))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), "loosens")
			assert.Contains(t, err.Error(), tt.wantContains)
		})
	}
}

func TestExtendsAcceptsTighteningChildren(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	chainFile(t, dir, "org.toml", nil, "[lint.severity_floor]\nAR001 = \"warning\"\n[sources]\nmin_release_age = \"3d\"\n[governance]\nmin_approvers = 1\n[lint.load_budgets]\nclaude-skill-listing = 1000\n")
	team := chainFile(t, dir, "team.toml", []string{"org.toml"}, "[lint.severity_floor]\nAR001 = \"error\"\n[sources]\nmin_release_age = \"7d\"\n[governance]\nmin_approvers = 2\n[lint.load_budgets]\nclaude-skill-listing = 500\n")
	// Act
	layers, err := Discover(extendsOpts(t, team))
	// Assert
	require.NoError(t, err)
	assert.Len(t, layers, 2)
}

func TestExtendsCycleAndDepth(t *testing.T) {
	t.Run("a cycle", func(t *testing.T) {
		dir := t.TempDir()
		a := chainFile(t, dir, "a.toml", []string{"b.toml"}, "")
		chainFile(t, dir, "b.toml", []string{"a.toml"}, "")
		_, err := Discover(extendsOpts(t, a))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR743")
		assert.Contains(t, err.Error(), "cycle")
	})
	t.Run("a policy extending itself", func(t *testing.T) {
		dir := t.TempDir()
		a := chainFile(t, dir, "a.toml", []string{"a.toml"}, "")
		_, err := Discover(extendsOpts(t, a))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle")
	})
	chain := func(t *testing.T, hops int) string {
		dir := t.TempDir()
		for i := hops; i >= 0; i-- {
			var ext []string
			if i < hops {
				ext = []string{fmt.Sprintf("p%d.toml", i+1)}
			}
			chainFile(t, dir, fmt.Sprintf("p%d.toml", i), ext, "[lock]\nenforce = true\n")
		}
		return filepath.Join(dir, "p0.toml")
	}
	t.Run("five hops are fine", func(t *testing.T) {
		layers, err := Discover(extendsOpts(t, chain(t, maxExtendsDepth)))
		require.NoError(t, err)
		assert.Len(t, layers, maxExtendsDepth+1)
	})
	t.Run("six hops are not", func(t *testing.T) {
		_, err := Discover(extendsOpts(t, chain(t, maxExtendsDepth+1)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR743")
		assert.Contains(t, err.Error(), "nested deeper than 5")
	})
}

func TestExtendsLimitsFanOut(t *testing.T) {
	// Arrange: one root with eight parents that each have eight parents: 73 policies.
	dir := t.TempDir()
	var top []string
	for i := range maxExtendsPerFile {
		var mid []string
		for j := range maxExtendsPerFile {
			name := fmt.Sprintf("leaf-%d-%d.toml", i, j)
			chainFile(t, dir, name, nil, "")
			mid = append(mid, name)
		}
		name := fmt.Sprintf("mid-%d.toml", i)
		chainFile(t, dir, name, mid, "")
		top = append(top, name)
	}
	root := chainFile(t, dir, "root.toml", top, "")
	// Act
	_, err := Discover(extendsOpts(t, root))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than 32 policies")

	tooMany := make([]string, maxExtendsPerFile+1)
	for i := range tooMany {
		tooMany[i] = "x.toml"
	}
	_, perr := Discover(extendsOpts(t, chainFile(t, dir, "wide.toml", tooMany, "")))
	require.Error(t, perr)
	assert.Contains(t, perr.Error(), "at most 8")
}

func TestExtendsDiamondLoadsEachPolicyOnce(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	chainFile(t, dir, "base.toml", nil, "[lock]\nenforce = true\n")
	chainFile(t, dir, "left.toml", []string{"base.toml"}, "")
	chainFile(t, dir, "right.toml", []string{"base.toml"}, "")
	top := chainFile(t, dir, "top.toml", []string{"left.toml", "right.toml"}, "")
	// Act
	layers, err := Discover(extendsOpts(t, top))
	// Assert
	require.NoError(t, err)
	var names []string
	for _, l := range layers {
		names = append(names, l.Name)
	}
	assert.Equal(t, []string{"top", "left", "base", "right"}, names)
}

func TestExtendsFailsClosedOnAMissingParent(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	team := chainFile(t, dir, "team.toml", []string{"gone.toml"}, "")
	// Act
	_, err := Discover(extendsOpts(t, team))
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR742")
	assert.Contains(t, err.Error(), "extends")
}

func TestExtendsAcrossURLs(t *testing.T) {
	// Arrange: a URL policy that extends a pinned URL parent.
	parentBody := "policy_version = 1\nname = \"org\"\n[lock]\nenforce = true\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/org.toml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(parentBody)) })
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	parentRef := srv.URL + "/org.toml@" + digest([]byte(parentBody))
	childBody := fmt.Sprintf("policy_version = 1\nname = \"team\"\nextends = [%q]\n[guard]\ngenerated = true\n", parentRef)
	mux.HandleFunc("/team.toml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(childBody)) })
	opts := func(flag string) DiscoverOptions {
		o := extendsOpts(t, flag)
		o.HTTPClient = srv.Client()
		return o
	}

	t.Run("a pinned URL chain loads", func(t *testing.T) {
		layers, err := Discover(opts(srv.URL + "/team.toml@" + digest([]byte(childBody))))
		require.NoError(t, err)
		require.Len(t, layers, 2)
		res := Resolve(layers)
		assert.True(t, res.Policy.Lock.Enforce)
		assert.True(t, res.Policy.Guard.Generated)
		assert.Equal(t, []string{srv.URL + "/org.toml"}, layers[0].Extends)
	})
	t.Run("an unpinned URL parent is refused", func(t *testing.T) {
		body := fmt.Sprintf("policy_version = 1\nextends = [%q]\n", srv.URL+"/org.toml")
		mux.HandleFunc("/loose.toml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
		_, err := Discover(opts(srv.URL + "/loose.toml@" + digest([]byte(body))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
	})
	t.Run("a parent that changed fails its pin", func(t *testing.T) {
		body := fmt.Sprintf("policy_version = 1\nextends = [%q]\n", srv.URL+"/org.toml@sha256:"+strings.Repeat("0", 64))
		mux.HandleFunc("/moved.toml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
		_, err := Discover(opts(srv.URL + "/moved.toml@" + digest([]byte(body))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR741")
	})
	t.Run("a URL policy cannot reach for a local file", func(t *testing.T) {
		body := "policy_version = 1\nextends = [\"/etc/ai-rulez/policy.toml\"]\n"
		mux.HandleFunc("/local.toml", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
		_, err := Discover(opts(srv.URL + "/local.toml@" + digest([]byte(body))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot extend the local file")
	})
}

func TestExtendsRejectsAnEntryThatIsNotAPolicy(t *testing.T) {
	for _, entry := range []string{"", "  ", "http://x.example/p.toml", "file:///etc/p.toml"} {
		t.Run(entry, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			team := chainFile(t, dir, "team.toml", []string{entry}, "")
			// Act
			_, err := Discover(extendsOpts(t, team))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
		})
	}
}

func TestExtendedLayersAreDeduplicatedAgainstOtherAnchors(t *testing.T) {
	// Arrange: the managed policy is also the team's parent.
	dir := t.TempDir()
	managed := chainFile(t, dir, "org.toml", nil, "[lock]\nenforce = true\n")
	team := chainFile(t, dir, "team.toml", []string{"org.toml"}, "")
	o := extendsOpts(t, team)
	o.ManagedPaths = []string{managed}
	// Act
	layers, err := Discover(o)
	// Assert
	require.NoError(t, err)
	assert.Len(t, layers, 2, "org.toml is one layer even though two anchors reach it")
}

func TestLoosensIsEmptyForAMergedChild(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	for i := 0; i < 3000; i++ {
		parent, child := randomPolicy(rng), randomPolicy(rng)
		merged := Merge(parent, child)
		assert.Empty(t, Loosens(parent, merged), "the effective policy of a chain is never weaker than its parent: %+v %+v", parent, child)
		assert.Empty(t, Loosens(parent, parent), "a policy does not loosen itself")
	}
}

func TestLoosensReportsWhatMergeWouldIgnore(t *testing.T) {
	rng := rand.New(rand.NewSource(29))
	reported := 0
	for i := 0; i < 3000; i++ {
		parent, child := randomPolicy(rng), randomPolicy(rng)
		if len(Loosens(parent, child)) == 0 {
			continue
		}
		reported++
		assert.NotEqual(t, child, Merge(parent, child), "a reported loosening must be neutralised by the fold: %+v %+v", parent, child)
	}
	assert.Positive(t, reported, "the generator must exercise the loosening paths")
}

func TestExtendsEnvAndManagedAnchorsExtendToo(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	chainFile(t, dir, "org.toml", nil, "[lock]\nenforce = true\n")
	managed := chainFile(t, dir, "managed.toml", []string{"org.toml"}, "")
	o := extendsOpts(t, "")
	o.ManagedPaths = []string{managed}
	// Act
	layers, err := Discover(o)
	// Assert
	require.NoError(t, err)
	require.Len(t, layers, 2)
	assert.Equal(t, OriginManaged, layers[0].Origin)
	assert.Equal(t, OriginExtends, layers[1].Origin)
	_, statErr := os.Stat(filepath.Join(dir, "org.toml"))
	require.NoError(t, statErr)
}
