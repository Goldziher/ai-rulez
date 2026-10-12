package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// countingRunner runs the real command and counts `git ls-files` calls.
type countingRunner struct {
	lsFiles *atomic.Int32
}

func (c countingRunner) Run(ctx context.Context, spec runner.Spec) runner.Result {
	if slicesContains(spec.Argv, "ls-files") {
		c.lsFiles.Add(1)
	}
	return runner.Exec{}.Run(ctx, spec)
}

func slicesContains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

// #376: the skills and commands of every domain share one `git ls-files` listing
// taken at the configuration directory, not one per directory.
func TestScanContentTree_ListsGitFilesOncePerConfigDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	cfgDir := filepath.Join(root, ".ai-rulez")
	for _, domain := range []string{"a", "b", "c", "d"} {
		skill := filepath.Join(cfgDir, "domains", domain, "skills", "s-"+domain)
		require.NoError(t, os.MkdirAll(filepath.Join(skill, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: s-"+domain+"\ndescription: d\n---\n# s\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "references", "r.md"), []byte("# ref\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "references", "ignored.md"), []byte("# ig\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.md\n"), 0o644))

	calls := &atomic.Int32{}
	scanner := newProjectScanner(context.Background(), osView(cfgDir))
	scanner.git = gitutil.New(countingRunner{lsFiles: calls})
	scanner.snapshots.anchor = filepath.Clean(cfgDir)
	tree, err := scanContentTree(scanner, cfgDir, nil)
	require.NoError(t, err)

	assert.Equal(t, int32(1), calls.Load(), "one git ls-files for all domains")
	require.Len(t, tree.Domains, 4)
	for name, domain := range tree.Domains {
		require.Len(t, domain.Skills, 1, name)
		var got []string
		for _, r := range domain.Skills[0].Resources {
			got = append(got, filepath.Base(r.RelPath))
		}
		assert.Equal(t, []string{"r.md"}, got, "%s: the gitignored resource is left out", name)
	}
}
