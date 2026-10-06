package lint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keyFixture(t *testing.T) (resolvedScanner, string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "scan")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // a test script
	sc := resolvedScanner{}
	sc.Name, sc.Command, sc.Format = "k", []string{bin}, "sarif"
	sc.EnvPass = []string{"AR_KEY_TEST_VALUE"}
	return sc, bin
}

func TestScanCacheKeyCoversEnvValuesAndIsolation(t *testing.T) {
	// Arrange
	sc, bin := keyFixture(t)
	keyFor := func(isolation string) string {
		key, ok := scanKeyFor(sc, bin, "sha256:tree", false, isolation)
		require.True(t, ok)
		return key
	}
	t.Setenv("AR_KEY_TEST_VALUE", "one")
	one := keyFor("auto/none")

	// Act / Assert
	assert.Equal(t, one, keyFor("auto/none"), "the same inputs give the same key")
	for _, iso := range []string{"none/none", "require/sandbox-exec", "require/bwrap", "auto/sandbox-exec"} {
		assert.NotEqual(t, one, keyFor(iso), iso)
	}
	t.Setenv("AR_KEY_TEST_VALUE", "two")
	assert.NotEqual(t, one, keyFor("auto/none"), "a changed env_pass value must miss")
}

func TestCachedResultsOfLauncherScannersExpire(t *testing.T) {
	tests := []struct {
		name    string
		command string
		age     time.Duration
		expired bool
	}{
		{"uvx fresh", "/usr/bin/uvx", time.Hour, false},
		{"uvx stale", "/usr/bin/uvx", 25 * time.Hour, true},
		{"npx stale", "npx", 48 * time.Hour, true},
		{"npx windows spelling", `C:\tools\npx.cmd`, 48 * time.Hour, true},
		{"a pinned binary never expires", "/usr/local/bin/agnix", 1000 * time.Hour, false},
		{"unknown age (old entry)", "uvx", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			at := time.Unix(2_000_000_000, 0)
			sc := resolvedScanner{}
			sc.Command = []string{tt.command}
			hit := cachedScan{Stored: at.Add(-tt.age).Unix()}
			if tt.age < 0 {
				hit.Stored = 0
			}
			// Act / Assert
			assert.Equal(t, tt.expired, cacheExpired(sc, hit, at))
		})
	}
}

func TestStagedRunReScansWhenALauncherScannersCacheExpires(t *testing.T) {
	// Arrange: a scanner named npx is a launcher, whose version moves without the binary changing.
	counter := filepath.Join(t.TempDir(), "count")
	bin := fakeBin(t, "npx", `if [ "$1" = "--version" ]; then echo "npx 1.0.0"; exit 0; fi
echo run >> `+counter+`
echo '{}'
`)
	p, cache := cachedProject(t, bin)
	opts := Options{Scanner: ScannerOptions{Cache: cache}}
	clock := time.Unix(2_000_000_000, 0)
	old := now
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = old })

	// Act
	p.run(opts)
	p.run(opts)
	hits := runs(t, counter)
	clock = clock.Add(25 * time.Hour)
	p.run(opts)

	// Assert
	assert.Equal(t, 1, hits, "inside the TTL the result is served from the cache")
	assert.Equal(t, 2, runs(t, counter), "past the TTL a launcher scanner is run again")
}
