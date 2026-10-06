package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNPMPack_RealNPMWritesThePlannedTarball runs the planned `npm pack` with
// the real npm when it is installed (packing a directory needs no network; the
// publish step is never run). It guards the argv spelling: a bare "npm/package"
// is read by npm as a GitHub repository, and the tarball name rule.
func TestNPMPack_RealNPMWritesThePlannedTarball(t *testing.T) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm is not installed")
	}
	dir, d := writeBuilt(t, npmInput())
	require.Equal(t, "npm", d.Plan.Commands[0].Argv[0])
	argv := append([]string{npm}, d.Plan.Commands[0].Argv[1:]...)
	home := t.TempDir()
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the argv is the plan's, run against a temp dist directory
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "npm_config_cache=" + filepath.Join(home, "cache"),
		"npm_config_update_notifier=false", "npm_config_audit=false", "npm_config_fund=false",
	}

	out, err := cmd.CombinedOutput()

	require.NoError(t, err, string(out))
	tarball := filepath.Join(dir, filepath.FromSlash(d.Plan.NPM.Tarball))
	assert.FileExists(t, tarball)
	packed, err := os.ReadFile(tarball) //nolint:gosec // a temp dist
	require.NoError(t, err)
	snap, err := snapshotPackage(filepath.Join(dir, filepath.FromSlash(NPMPackageDir)))
	require.NoError(t, err)
	assert.NoError(t, snap.verifyTarball(packed), "what real npm packs matches the dist directory")
}
