package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestBuildAggregate_HoldsTheIndexAndEmitterFilesAndVerifiesBySums(t *testing.T) {
	x := Extras{
		Channel: "stable", Pin: pinFor(claudeIndex), PinCommit: commit40,
		Emit: &EmitRequest{Names: []string{"port"}, Experimental: true},
		Plugins: []emit.Plugin{{
			Name: "acme", Version: "1.4.0",
			Files: []emit.File{{Path: "skills/deploy/SKILL.md", Data: []byte("---\nname: deploy\n---\n")}},
		}},
	}

	d, err := BuildAggregate("acme-market", x, nil)

	require.NoError(t, err)
	assert.Contains(t, d.Files, "marketplace/stable/.claude-plugin/marketplace.json")
	assert.Contains(t, d.Files, "emit/port/index.json")
	assert.Equal(t, "acme-market", d.Plan.Name)
	assert.NotEmpty(t, d.Warnings)
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
	dir := filepath.Join(t.TempDir(), "aggregate")
	require.NoError(t, d.Write(dir))
	res, err := VerifySums(dir)
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
	appendTo(t, filepath.Join(dir, "emit", "port", "index.json"), " ")
	res, err = VerifySums(dir)
	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestBuildAggregate_NeedsSomethingToAggregate(t *testing.T) {
	_, err := BuildAggregate("m", Extras{}, nil)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeConfig, pe.Code)
}

func TestPreviousTag(t *testing.T) {
	describe := []string{"git", "-C", "/p", "describe", "--tags", "--abbrev=0"}
	tests := []struct {
		name    string
		exclude string
		result  runner.Result
		want    string
		argv    []string
	}{
		{"closest tag", "v1.4.0", runner.Result{Status: runner.StatusOK, Stdout: []byte("v1.3.0\n")}, "v1.3.0",
			append(append([]string{}, describe...), "--exclude", "v1.4.0", "HEAD")},
		{"no tags", "", runner.Result{Status: runner.StatusExit, ExitCode: 128}, "",
			append(append([]string{}, describe...), "HEAD")},
		{"option-looking tag is dropped", "", runner.Result{Status: runner.StatusOK, Stdout: []byte("--force\n")}, "",
			append(append([]string{}, describe...), "HEAD")},
		{"unsafe exclude is not passed", "-x", runner.Result{Status: runner.StatusOK, Stdout: []byte("v1\n")}, "v1",
			append(append([]string{}, describe...), "HEAD")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result { return tt.result }}

			got := PreviousTag(context.Background(), fake, "/p", tt.exclude)

			assert.Equal(t, tt.want, got)
			require.Len(t, fake.Calls(), 1)
			assert.Equal(t, tt.argv, fake.Calls()[0].Argv)
		})
	}
}

func TestBuildAggregate_RecordsThePluginsOfTheRelease(t *testing.T) {
	refs := []PluginRef{
		{Name: "beta", Version: "1.0.0", Dir: "plugins/beta", ManifestDigest: Digest([]byte("b")), BundleDigest: Digest([]byte("bb"))},
		{Name: "alpha", Version: "1.0.0", Dir: "plugins/alpha", ManifestDigest: Digest([]byte("a")), BundleDigest: Digest([]byte("aa"))},
	}

	d, err := BuildAggregate("m", Extras{}, refs)

	require.NoError(t, err, "a release with neither an index nor an emitter still names its plugins")
	assert.Contains(t, string(d.Files[PluginsFile]), `"name": "alpha"`)
	assert.Less(t, strings.Index(string(d.Files[PluginsFile]), "alpha"), strings.Index(string(d.Files[PluginsFile]), "beta"), "sorted by name")
	assert.Contains(t, string(d.Files[SumsFile]), PluginsFile)
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
}

// multiRoot writes plugins/<name> dists and an aggregate that lists them.
func multiRoot(t *testing.T, names ...string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "dist")
	var refs []PluginRef
	for _, n := range names {
		in := committedInput()
		in.Name = n
		d, err := Build(in)
		require.NoError(t, err)
		require.NoError(t, d.Write(filepath.Join(root, PluginsDir, n)))
		refs = append(refs, NewPluginRef(d))
	}
	agg, err := BuildAggregate("m", Extras{}, refs)
	require.NoError(t, err)
	require.NoError(t, agg.Write(filepath.Join(root, "aggregate")))
	return root
}

func TestVerifyPluginList(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(t *testing.T, root string)
		want   string
	}{
		{"intact", func(*testing.T, string) {}, ""},
		{"a plugin directory removed", func(t *testing.T, root string) {
			require.NoError(t, os.RemoveAll(filepath.Join(root, "plugins", "beta")))
		}, "its manifest is missing"},
		{"a plugin swapped for another release", func(t *testing.T, root string) {
			replaceIn(t, filepath.Join(root, "plugins", "beta", "beta-1.4.0.manifest.json"), `"AR`, `"XX`)
			appendTo(t, filepath.Join(root, "plugins", "beta", "beta-1.4.0.manifest.json"), " ")
		}, "digest is"},
		{"an unlisted plugin directory", func(t *testing.T, root string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins", "extra"), 0o750))
		}, "not listed"},
		{"the list removed", func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, "aggregate", PluginsFile)))
		}, "does not name the plugins"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := multiRoot(t, "alpha", "beta")
			tt.tamper(t, root)

			problems := VerifyPluginList(root)

			if tt.want == "" {
				assert.Empty(t, problems)
				return
			}
			var all []string
			for _, p := range problems {
				all = append(all, p.Path+": "+p.Message)
			}
			assert.Contains(t, strings.Join(all, "\n"), tt.want)
		})
	}
}
