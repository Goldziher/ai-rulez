package includes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const locateSkillBody = "---\nname: myskill\n---\n# My Skill\nOriginal content.\n"

// locateSource is a SkillGitSource for the single skill skills/myskill of repoDir.
func locateSource(t *testing.T, repoDir string, rec *testutil.LogRecorder) *SkillGitSource {
	t.Helper()
	s := &SkillGitSource{
		name:        "myskill",
		repoURL:     normalizeGitURL("file://" + repoDir),
		originalURL: "file://" + repoDir,
		path:        "skills/myskill",
		cacheDir:    filepath.Join(t.TempDir(), "cache"),
	}
	if rec != nil {
		s.log = rec
	}
	return s
}

// pinOf pins s to what its cache currently holds.
func pinOf(t *testing.T, s *SkillGitSource) *pin {
	t.Helper()
	root, _ := s.cachedRoot()
	require.NotEmpty(t, root, "the cache must hold the skill before it is pinned")
	digest, err := contentlock.DigestDir(contentlock.KindInstalledSkill, root)
	require.NoError(t, err)
	meta, err := readCacheMeta(s.cacheDir)
	require.NoError(t, err)
	require.NotNil(t, meta)
	return &pin{entry: lockfile.Entry{Name: s.name, Commit: meta.RemoteHEADSHA, Digest: digest}, kind: lockfile.KindSkill, name: s.name}
}

func TestSkillGitSourceLocate_RemoteUnreachable(t *testing.T) {
	tests := []struct {
		name      string
		populate  bool
		wantErr   string
		wantWarn  string
		wantFound bool
	}{
		{name: "falls back to the cache and says so", populate: true, wantWarn: "ls-remote failed, using cached skill", wantFound: true},
		{name: "fails without a cache", wantErr: "failed to get remote HEAD for skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := initLocalRepo(t)
			commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
			rec := &testutil.LogRecorder{}
			s := locateSource(t, repo, rec)
			if tt.populate {
				_, err := s.Fetch(t.Context())
				require.NoError(t, err)
			}
			gone := "file://" + filepath.Join(t.TempDir(), "gone")
			s.originalURL, s.repoURL = gone, normalizeGitURL(gone)

			// Act
			skills, err := s.DiscoverSkills(t.Context(), nil, nil, "")

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, skills, 1)
			assert.Contains(t, skills[0].Content, "Original content.")
			assert.Contains(t, rec.String(), tt.wantWarn)
		})
	}
}

func TestSkillGitSourceLocate_OfflineReadsOnlyTheCache(t *testing.T) {
	// Arrange
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
	s := locateSource(t, repo, nil)
	offline := config.WithOfflineIncludes(t.Context())

	// Act
	_, coldErr := s.DiscoverSkills(offline, nil, nil, "")
	_, fetchErr := s.Fetch(t.Context())
	// the remote moves on; offline must not see it
	commitFile(t, repo, "skills/myskill/SKILL.md", "---\nname: myskill\n---\n# My Skill\nNewer content.\n")
	warm, warmErr := s.DiscoverSkills(offline, nil, nil, "")

	// Assert
	require.ErrorIs(t, coldErr, ErrNotCached)
	require.NoError(t, fetchErr)
	require.NoError(t, warmErr)
	require.Len(t, warm, 1)
	assert.Contains(t, warm[0].Content, "Original content.")
}

func TestSkillGitSourceLocate_CloneFailureLeavesNoPartialCache(t *testing.T) {
	// Arrange: a full SHA is used verbatim, so the failure is the clone's, not ls-remote's
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
	s := locateSource(t, repo, nil)
	s.ref = "0123456789012345678901234567890123456789"

	// Act
	_, err := s.DiscoverSkills(t.Context(), nil, nil, "")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to clone skill "myskill"`)
	assert.NoDirExists(t, s.cacheDir)
	entries, readErr := os.ReadDir(filepath.Dir(s.cacheDir))
	require.NoError(t, readErr)
	assert.Empty(t, entries, "the temporary clone directory is removed")
}

func TestSkillGitSourceLocate_RepairsADamagedCacheOnce(t *testing.T) {
	// Arrange: pin the good content, then damage the cached file but leave its meta valid
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
	s := locateSource(t, repo, nil)
	_, err := s.Fetch(t.Context())
	require.NoError(t, err)
	s.pin = pinOf(t, s)
	cached := filepath.Join(s.cacheDir, "skills", "myskill", "SKILL.md")
	require.NoError(t, os.WriteFile(cached, []byte("# tampered\n"), 0o644))

	// Act
	skills, err := s.DiscoverSkills(t.Context(), nil, nil, "")

	// Assert
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Contains(t, skills[0].Content, "Original content.")
	restored, readErr := os.ReadFile(cached)
	require.NoError(t, readErr)
	assert.Equal(t, locateSkillBody, strings.ReplaceAll(string(restored), "\r\n", "\n"), "git may check the file out with CRLF (core.autocrlf)")
}

func TestSkillGitSourceLocate_DamagedCacheIsNotRepairedOffline(t *testing.T) {
	// Arrange
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
	s := locateSource(t, repo, nil)
	_, err := s.Fetch(t.Context())
	require.NoError(t, err)
	s.pin = pinOf(t, s)
	cached := filepath.Join(s.cacheDir, "skills", "myskill", "SKILL.md")
	require.NoError(t, os.WriteFile(cached, []byte("# tampered\n"), 0o644))

	// Act
	_, err = s.DiscoverSkills(config.WithOfflineIncludes(t.Context()), nil, nil, "")

	// Assert
	require.ErrorIs(t, err, config.ErrLockViolation)
	assert.ErrorIs(t, err, errDigestMismatch)
	damaged, readErr := os.ReadFile(cached)
	require.NoError(t, readErr)
	assert.Equal(t, "# tampered\n", string(damaged), "offline never touches the network or the cache")
}

func TestSkillGitSourceLocate_RemoteServingOtherBytesForThePinnedCommitFails(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(p *pin)
		wantSubstr string
		wantDigest bool
	}{
		{"digest differs", func(p *pin) { p.entry.Digest = "sha256:deadbeef" }, "does not match the lock", true},
		{"commit differs", func(p *pin) { p.entry.Commit = "0123456789012345678901234567890123456789" }, "fetched commit", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := initLocalRepo(t)
			commitFile(t, repo, "skills/myskill/SKILL.md", locateSkillBody)
			s := locateSource(t, repo, nil)
			_, err := s.Fetch(t.Context())
			require.NoError(t, err)
			s.pin = pinOf(t, s)
			tt.mutate(s.pin)

			// Act
			_, err = s.DiscoverSkills(t.Context(), nil, nil, "")

			// Assert
			require.ErrorIs(t, err, config.ErrLockViolation)
			assert.Contains(t, err.Error(), tt.wantSubstr)
			assert.Equal(t, tt.wantDigest, errors.Is(err, errDigestMismatch))
		})
	}
}

func TestSkillGitSourceLocate_ASymlinkedSkillMarkerInTheRemoteIsRefused(t *testing.T) {
	// Arrange: the repository commits SKILL.md as a symlink
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/myskill/real.md", "# real\n")
	link := filepath.Join(repo, "skills", "myskill", "SKILL.md")
	testutil.SymlinkOrSkip(t, "real.md", link)
	for _, args := range [][]string{{"add", "skills/myskill/SKILL.md"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "link"}} {
		cmd := gitutil.CommandNoContext(repo, args...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	s := locateSource(t, repo, nil)

	// Act
	_, err := s.DiscoverSkills(context.Background(), nil, nil, "")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a symlink, and symlinks are not followed")
	assert.Contains(t, err.Error(), filepath.Join("skills", "myskill", "SKILL.md"))
}

func TestSkillGitSourceLocate_MissingSkillDirectoryNamesTheExpectedPath(t *testing.T) {
	// Arrange
	repo := initLocalRepo(t)
	commitFile(t, repo, "skills/other/SKILL.md", locateSkillBody)
	s := locateSource(t, repo, nil)

	// Act
	_, err := s.DiscoverSkills(t.Context(), nil, nil, "")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill directory not found (expected SKILL.md at skills/myskill)")
}
