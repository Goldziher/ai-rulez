package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPassthroughFileRejectsSymlinkEscape covers the bundling counterpart of the
// symlink defense in config.LoadResources: a passthrough source that resolves
// outside the project must not have its bytes copied into the plugin bundle.
// Every bundled file is published to consumers and, for a hook script, executed
// by them, so a source that escapes the project is an exfiltration channel.
func TestPassthroughFileRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	t.Run("should_reject_when_the_source_file_is_a_symlink_out_of_the_project", func(t *testing.T) {
		t.Parallel()

		outside := t.TempDir()
		secret := filepath.Join(outside, "id_rsa")
		require.NoError(t, os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600))

		sourceDir := t.TempDir()
		link := filepath.Join(sourceDir, "bootstrap.sh")
		require.NoError(t, os.Symlink(secret, link))

		_, err := passthroughFile(sourceDir, "bootstrap.sh", filepath.Join(t.TempDir(), "bootstrap.sh"))

		require.Error(t, err, "a symlink escaping the project must not be bundled")
		assert.NotContains(t, err.Error(), "PRIVATE KEY", "the error must not echo the target's bytes")
	})

	t.Run("should_reject_when_an_intermediate_directory_escapes_the_project", func(t *testing.T) {
		t.Parallel()

		outside := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(outside, "passwd"), []byte("root:x:0:0"), 0o644))

		sourceDir := t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(sourceDir, "vendor")))

		// No ".." and no absolute prefix, so the lexical guard never fires.
		_, err := passthroughFile(sourceDir, "vendor/passwd", filepath.Join(t.TempDir(), "passwd"))

		require.Error(t, err, "a symlinked directory component must not be traversed out of the project")
	})

	t.Run("should_bundle_a_symlink_that_stays_inside_the_project", func(t *testing.T) {
		t.Parallel()

		sourceDir := t.TempDir()
		real := filepath.Join(sourceDir, "scripts", "bootstrap.sh")
		require.NoError(t, os.MkdirAll(filepath.Dir(real), 0o755))
		require.NoError(t, os.WriteFile(real, []byte("#!/bin/sh\necho hi\n"), 0o755))
		require.NoError(t, os.Symlink(real, filepath.Join(sourceDir, "bootstrap.sh")))

		out, err := passthroughFile(sourceDir, "bootstrap.sh", filepath.Join(t.TempDir(), "bootstrap.sh"))

		require.NoError(t, err, "an intra-project symlink is a normal repository layout")
		assert.Equal(t, "#!/bin/sh\necho hi\n", string(out.RawContent))
	})

	t.Run("should_still_bundle_a_plain_file", func(t *testing.T) {
		t.Parallel()

		sourceDir := t.TempDir()
		source := filepath.Join(sourceDir, "bootstrap.sh")
		require.NoError(t, os.WriteFile(source, []byte("#!/bin/sh\n"), 0o755))
		// Compared against the source's own mode rather than a literal 0o755:
		// Windows has no executable bit, so Go reports 0o666 there.
		info, err := os.Stat(source)
		require.NoError(t, err)

		out, err := passthroughFile(sourceDir, "bootstrap.sh", filepath.Join(t.TempDir(), "bootstrap.sh"))

		require.NoError(t, err)
		assert.Equal(t, info.Mode().Perm(), out.Mode.Perm())
	})
}
