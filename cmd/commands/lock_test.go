package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

const lockProjectConfig = `version = "4.0"
name = "lock-test"
presets = ["claude"]
`

func lockProject(t *testing.T, extraConfig string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig+extraConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse tabs.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy the service. Use when releasing.\nowner: team-a\nversion: 1.0.0\n---\nDeploy.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "deploy", "references", "api.md"), "api\n")
	chdir(t, root)
	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		lockCheck, lockDiffFlag, lockContentOnly, lockFormat, lockProfile, lockRecursive, lockKind = false, false, false, "", "", false, ""
		generateLocked, generateFrozen = false, false
	})
	return root
}

// capture runs fn and returns what it wrote to stdout and stderr.
func capture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout, os.Stderr = outW, errW
	done := make(chan [2]string)
	go func() {
		var o, e bytes.Buffer
		finished := make(chan struct{})
		go func() { _, _ = io.Copy(&e, errR); close(finished) }() //nolint:errcheck // test helper
		_, _ = io.Copy(&o, outR)                                  //nolint:errcheck // test helper
		<-finished
		done <- [2]string{o.String(), e.String()}
	}()
	fn()
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	os.Stdout, os.Stderr = oldOut, oldErr
	res := <-done
	return res[0], res[1]
}

func TestLockWritesContentPinsDeterministically(t *testing.T) {
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	path := filepath.Join(root, ".ai-rulez", lockfile.FileName)
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	assert.Equal(t, lockfile.Version, lock.Version)
	assert.Equal(t, Version, lock.AIRulezVersion)
	assert.True(t, strings.HasPrefix(lock.Tree, "sha256:"))
	var ids []string
	for _, i := range lock.Item {
		ids = append(ids, i.Kind+"/"+i.ID)
		assert.True(t, strings.HasPrefix(i.Digest, "sha256:"))
	}
	assert.Equal(t, []string{"rule/style", "skill/deploy"}, ids)
	assert.Equal(t, "team-a", lock.Item[1].Owner)
	assert.Equal(t, "1.0.0", lock.Item[1].Version)
	assert.NotEmpty(t, lock.Output, "generated outputs are pinned")
	for _, o := range lock.Output {
		assert.NotContains(t, o.Path, "settings.json")
	}

	require.Equal(t, 0, writeLockAt("", "", nil))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "writing twice is byte-identical")
}

func TestLockCheckNamesSourceAndOutputChanges(t *testing.T) {
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	assert.Equal(t, 0, checkLockAt(""))

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "fresh.md"), "# Fresh\n")
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "skills", "deploy", "references", "api.md")))

	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "source changed")
	assert.Contains(t, stderr, "rule style")
	assert.Contains(t, stderr, "source added")
	assert.Contains(t, stderr, "rule fresh")
	assert.Contains(t, stderr, "skill deploy")
	assert.Contains(t, stderr, "output changed")
	assert.Contains(t, stderr, ".claude/rules/style.md")

	// --diff reports the same and exits 0; json is machine readable
	lockDiffFlag, lockFormat = true, formatJSON
	stdout, _ := capture(t, func() { code = diffLockAt("") })
	assert.Equal(t, 0, code)
	var diff contentlock.Diff
	require.NoError(t, json.Unmarshal([]byte(stdout), &diff))
	assert.False(t, diff.InSync)
	assert.Equal(t, contentlock.DiffSchemaVersion, diff.SchemaVersion)
	assert.NotEmpty(t, diff.Sources())
	assert.NotEmpty(t, diff.Outputs())
	validateAgainst(t, "../../schema/lock-diff.schema.json", []byte(stdout))

	// accepting the change makes the check pass again
	require.Equal(t, 0, writeLockAt("", "", nil))
	assert.Equal(t, 0, checkLockAt(""))
}

func TestLockCheckRejectsDowngradedLock(t *testing.T) {
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	assert.Equal(t, 0, checkLockAt(""))
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", lockfile.FileName), "version = 1\n")
	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code, "a downgraded lock must fail the check")
	assert.Contains(t, stderr, "no content pins")
}

func TestLockCheckWithLegacyLock(t *testing.T) {
	root := lockProject(t, "")
	legacy := filepath.Join(root, ".ai-rulez", lockfile.FileName)
	writeFile(t, legacy, "version = 1\n")
	var legacyCode int
	_, legacyErr := capture(t, func() { legacyCode = checkLockAt("") })
	assert.Equal(t, exitDrift, legacyCode, "a lock without content pins must not pass a check, enforce or not")
	assert.Contains(t, legacyErr, "no content pins")

	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	assert.False(t, lock.HasContentPins())

	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig+"[lock]\nenforce = true\n")
	var code int
	_, stderr := capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "no content pins")
}

func TestLockContentOnlyKeepsRemoteEntries(t *testing.T) {
	root := lockProject(t, "")
	dir := filepath.Join(root, ".ai-rulez")
	seed := &lockfile.File{}
	seed.Set(lockfile.KindInclude, lockfile.Entry{Name: "shared", Source: "https://example.com/r", Commit: "abc", Digest: "sha256:1"})
	require.NoError(t, lockfile.Save(dir, seed))
	lockContentOnly = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	got, err := lockfile.Load(dir)
	require.NoError(t, err)
	require.Len(t, got.Include, 1)
	assert.Equal(t, "abc", got.Include[0].Commit)
	assert.True(t, got.HasContentPins())
}

func TestGenerateLockedFailsOnSourceDrift(t *testing.T) {
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	cfg, err := loadForLock("")
	require.NoError(t, err)

	generateLocked = true
	require.NoError(t, enforceLockedContent(cfg))

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
	cfg, err = loadForLock("")
	require.NoError(t, err)
	err = enforceLockedContent(cfg)
	require.ErrorIs(t, err, errLockedSourceDrift)
	assert.Contains(t, err.Error(), "rule style")

	// generate never rewrites the lock
	before, err := os.ReadFile(filepath.Join(root, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)
	generateLocked = false
	cfg, err = loadForLock("")
	require.NoError(t, err)
	require.NoError(t, enforceLockedContent(cfg), "without --locked nothing is enforced")
	after, err := os.ReadFile(filepath.Join(root, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestLockDriftForNeedsEnforceAndLock(t *testing.T) {
	root := lockProject(t, "[lock]\nenforce = true\n")
	cfg, err := loadForLock("")
	require.NoError(t, err)
	drift := lockDriftFor(cfg)
	assert.Empty(t, drift, "no lock, no finding")

	require.Equal(t, 0, writeLockAt("", "", nil))
	drift = lockDriftFor(cfg)
	assert.Empty(t, drift)

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
	cfg, err = loadForLock("")
	require.NoError(t, err)
	drift = lockDriftFor(cfg)
	var sources, outputs int
	for _, d := range drift {
		if d.Output {
			outputs++
		} else {
			sources++
			assert.Equal(t, ".ai-rulez/rules/style.md", d.Path)
		}
	}
	assert.Equal(t, 1, sources)
	assert.Positive(t, outputs)

	// without enforce nothing is raised
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig)
	cfg, err = loadForLock("")
	require.NoError(t, err)
	drift = lockDriftFor(cfg)
	assert.Empty(t, drift)
}

func TestLockDriftForUnreadableLockIsAFindingUnderEnforce(t *testing.T) {
	root := lockProject(t, "[lock]\nenforce = true\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockPath := filepath.Join(root, ".ai-rulez", lockfile.FileName)
	good, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	cfg, err := loadForLock("")
	require.NoError(t, err)

	for name, content := range map[string]string{
		"corrupt":              string(good) + "garbage = [\n",
		"unsupported version":  "version = 99\n",
		"older version":        strings.Replace(string(good), "version = 1", "version = 0", 1),
		"content pins removed": "version = 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			writeFile(t, lockPath, content)
			drift := lockDriftFor(cfg)
			require.NotEmpty(t, drift, "an unverifiable lock must not pass silently")
			assert.Equal(t, ".ai-rulez/"+lockfile.FileName, drift[0].Path)
		})
	}

	// without enforce an unreadable lock is not this check's business
	writeFile(t, lockPath, "garbage = [\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig)
	cfg, err = loadForLock("")
	require.NoError(t, err)
	assert.Empty(t, lockDriftFor(cfg))
}

func TestLockDiffSkippedRemoteOutputsFailUnderEnforce(t *testing.T) {
	root := lockProject(t, "")
	cfg, err := loadForLock("")
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	hasCacheChange := func(d *contentlock.Diff) bool {
		for _, c := range d.Changes {
			if strings.Contains(c.Detail, "not in the local cache") {
				return true
			}
		}
		return false
	}
	diff, err := lockDiff(cfg, lock, "", true)
	require.NoError(t, err)
	assert.NotEmpty(t, diff.Notes, "without enforce the skipped outputs are a note")
	assert.False(t, hasCacheChange(diff))

	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), lockProjectConfig+"[lock]\nenforce = true\n")
	cfg, err = loadForLock("")
	require.NoError(t, err)
	diff, err = lockDiff(cfg, lock, "", true)
	require.NoError(t, err)
	assert.True(t, hasCacheChange(diff), "under enforce skipped outputs fail the check")
}

func TestLoadWithCacheFallback(t *testing.T) {
	ok := &config.Config{}
	miss := oops.Wrapf(includes.ErrNotCached, "no cached content")
	violation := oops.Wrapf(config.ErrLockViolation, "digest mismatch")

	t.Run("cache miss falls back", func(t *testing.T) {
		calls := 0
		cfg, skipped, err := loadWithCacheFallback(func(opts ...config.LoadOption) (*config.Config, error) {
			calls++
			if len(opts) == 0 {
				return nil, miss
			}
			return ok, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, calls)
		assert.Same(t, ok, cfg)
		assert.False(t, skipped, "no lockable includes in an empty config")
	})
	t.Run("a lock violation is returned, not swallowed", func(t *testing.T) {
		calls := 0
		_, _, err := loadWithCacheFallback(func(...config.LoadOption) (*config.Config, error) {
			calls++
			return ok, violation
		})
		require.ErrorIs(t, err, config.ErrLockViolation)
		assert.Equal(t, 1, calls, "no retry without remotes")
	})
	t.Run("a failing retry returns the original error", func(t *testing.T) {
		_, _, err := loadWithCacheFallback(func(opts ...config.LoadOption) (*config.Config, error) {
			if len(opts) == 0 {
				return nil, miss
			}
			return nil, errors.New("second failure")
		})
		require.ErrorIs(t, err, includes.ErrNotCached)
	})
}
