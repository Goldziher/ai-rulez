package airulez_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// readTree reads every file below dir into a name -> content map.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err //nolint:wrapcheck // test helper
		}
		if rel, relErr := filepath.Rel(dir, p); relErr == nil && !strings.HasPrefix(filepath.ToSlash(rel), ".git/") {
			data, readErr := os.ReadFile(p) //nolint:gosec // test fixture
			require.NoError(t, readErr)
			out[filepath.ToSlash(rel)] = string(data)
		}
		return nil
	}))
	return out
}

func TestPlanReflectsTheExistingOutputsOfItsWorkspace(t *testing.T) {
	// Arrange: a project generated for three presets, then one that keeps only
	// claude, so a run would take the cursor and codex outputs back. The same tree
	// is a directory, an in-memory workspace and a commit.
	dir := t.TempDir()
	writeSources(t, dir)
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
	require.NoError(t, err)
	_, err = project.Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Write})
	require.NoError(t, err)
	settings := filepath.Join(dir, ".claude", "settings.json")
	doc, err := os.ReadFile(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settings, []byte(strings.Replace(string(doc), "{", "{\n  \"userKey\": true,", 1)), 0o644))
	trimmed := map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\nname = \"svc\"\npresets = [\"claude\"]\nagents_md = false\n"}
	for name, content := range trimmed {
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), nil, 0o644)) // keep the outputs in the commit
	rev := commit(t, dir)
	snapshot, err := airulez.GitSnapshot(t.Context(), dir, rev, nil)
	require.NoError(t, err)
	workspaces := map[string]airulez.Workspace{"disk": disk, "memory": memWith(readTree(t, dir)), "git snapshot": snapshot}

	// Act
	plans := map[string]*airulez.Plan{}
	for name, ws := range workspaces {
		p, loadErr := airulez.Load(t.Context(), airulez.Options{Workspace: ws})
		require.NoError(t, loadErr, name)
		plans[name], err = p.Plan(t.Context(), airulez.PlanOptions{})
		require.NoError(t, err, name)
	}

	// Assert
	var removed []string
	for _, r := range plans["disk"].Removals {
		removed = append(removed, r.Path)
	}
	assert.Contains(t, removed, ".cursor/rules/style.mdc", "the disk plan takes the cursor output back")
	assert.Contains(t, plans["memory"].Removals, airulez.PlanRemoval{Path: ".claude/settings.json", Reason: "unmerge"},
		"a document that still holds the user's key is taken back from, not deleted")
	want, err := plans["disk"].JSON()
	require.NoError(t, err)
	// A commit holds whatever its author chose, so the machine-local record in it
	// is not believed: the snapshot plan still takes back what is provably
	// generated (the cursor files carry their Content-Hash) but not the entries
	// only that record vouches for.
	snap := map[string]string{}
	for _, r := range plans["git snapshot"].Removals {
		snap[r.Path] = r.Reason
	}
	assert.Equal(t, "stale", snap[".cursor/rules/style.mdc"])
	assert.NotContains(t, snap, ".claude/settings.json")
	delete(plans, "git snapshot")
	for name, p := range plans {
		got, jsonErr := p.JSON()
		require.NoError(t, jsonErr, name)
		assert.JSONEq(t, string(want), string(got), "%s plan", name)
	}
}
