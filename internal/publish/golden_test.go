package publish

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const archiveGoldenFile = "testdata/archive-digests.txt"

// goldenToolchain is the Go release line ("go1.27") the running test was built with.
func goldenToolchain() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return runtime.Version()
	}
	return "go" + parts[0] + "." + parts[1]
}

func readGolden(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	data, err := os.ReadFile(archiveGoldenFile)
	if os.IsNotExist(err) {
		return out
	}
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if tc, digest, ok := strings.Cut(line, " "); ok {
			out[tc] = digest
		}
	}
	return out
}

// TestBuildArchive_MatchesTheGoldenDigestOfThisToolchain pins the archive bytes
// of sampleInput. The gzip stream comes from compress/flate, so the digest is a
// property of the Go toolchain: a release line without an entry is skipped, not
// failed. To add or refresh the entry for the toolchain you run with:
//
//	UPDATE_GOLDEN=1 go test ./internal/publish -run TestBuildArchive_MatchesTheGoldenDigest
//
// and review the diff of testdata/archive-digests.txt: a changed digest for an
// unchanged toolchain line means the archive format itself changed.
func TestBuildArchive_MatchesTheGoldenDigestOfThisToolchain(t *testing.T) {
	// Arrange
	in := sampleInput()
	archive, err := BuildArchive(in.Files, in.Mtime)
	require.NoError(t, err)
	got := Digest(archive)
	golden := readGolden(t)
	tc := goldenToolchain()

	if os.Getenv("UPDATE_GOLDEN") != "" {
		golden[tc] = got
		var lines []string
		for k, v := range golden {
			lines = append(lines, k+" "+v)
		}
		sort.Strings(lines)
		require.NoError(t, os.MkdirAll(filepath.Dir(archiveGoldenFile), 0o750))
		require.NoError(t, os.WriteFile(archiveGoldenFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
		return
	}

	// Assert
	want, ok := golden[tc]
	if !ok {
		t.Skipf("no golden archive digest for %s; run with UPDATE_GOLDEN=1 to add one", tc)
	}
	assert.Equal(t, want, got, "archive bytes changed for %s; if intended, rerun with UPDATE_GOLDEN=1", tc)
}
