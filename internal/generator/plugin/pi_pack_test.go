package plugin

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiOpenCodePackageSurvivesRealNPMPack(t *testing.T) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm is required for the real package integration test")
	}
	dir, dist := piOpenCodeDist(t)
	verification, err := publish.Verify(dir)
	require.NoError(t, err)
	require.True(t, verification.OK(), "%v", verification.Problems)
	cmd := exec.CommandContext(t.Context(), npm, "pack", "--ignore-scripts", "--json",
		"--pack-destination", dir, filepath.Join(dir, filepath.FromSlash(publish.NPMPackageDir)))
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	files := npmTarFiles(t, filepath.Join(dir, filepath.Base(dist.Plan.NPM.Tarball)))
	for _, path := range []string{
		"package/package.json", "package/.pi/skills/basemind/SKILL.md",
		"package/.pi/skills/basemind/references/usage.md", "package/.pi/prompts/check.md",
		"package/.opencode/plugins/basemind.js", "package/.opencode/ai-rulez-content.js",
	} {
		assert.NotEmpty(t, files[path], "npm must actually pack %s", path)
	}
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(files["package/package.json"], &pkg))
	assert.Contains(t, pkg, "pi")
	assert.Contains(t, pkg, "exports")
	assert.Equal(t, ".opencode/plugins/basemind.js", pkg["main"])
	assert.Contains(t, pkg["keywords"], "pi-package")
}

func piOpenCodeDist(t *testing.T) (string, *publish.Dist) {
	t.Helper()
	cfg, err := config.LoadConfig(t.Context(), fixtureDir)
	require.NoError(t, err)
	cfg.Plugin.Runtimes = []string{"pi", "opencode"}
	m, err := BuildManifest(cfg, cfg.Content)
	require.NoError(t, err)
	m.Commands = []config.ContentFile{{Name: "check", Content: "Check $ARGUMENTS."}}
	root := t.TempDir()
	outputs, err := Generate(m, root)
	require.NoError(t, err)
	var files []publish.File
	for _, out := range outputs {
		rel, err := filepath.Rel(root, out.Path)
		require.NoError(t, err)
		files = append(files, publish.File{Path: filepath.ToSlash(rel), Data: out.RawContent})
	}
	tree := publish.Digest([]byte("tree"))
	dist, err := publish.Build(publish.Input{
		Name: m.Name, Version: m.Version, AIRulezVersion: "5.0.0", Runtimes: m.Runtimes, Files: files,
		Target: publish.TargetNPM, NPM: publish.NPMOptions{Scope: "@goldziher"},
		Lock: []byte("version = 2\ntree = \"" + tree + "\"\n"), LockVersion: 2, LockTree: tree,
		Source: publish.Source{Repo: m.Repository, Commit: "0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e0f3e"},
	})
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, dist.Write(dir))
	return dir, dist
}

func npmTarFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, zr.Close()) })
	tr := tar.NewReader(zr)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		files[hdr.Name], err = io.ReadAll(tr)
		require.NoError(t, err)
	}
	return files
}
