package includes

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckLockShouldPreserveIndependentProjectCaches(t *testing.T) {
	for _, different := range []string{"commit", "path"} {
		t.Run(different, func(t *testing.T) {
			f := newLockFixture(t)
			first := testutil.Git(t, f.remote, "rev-parse", "HEAD")
			f.advance(t)
			writeTestFile(t, filepath.Join(f.remote, "alternate", ".ai-rulez", "rules", "shared.md"), "# Shared\n\nother include\n")
			writeTestFile(t, filepath.Join(f.remote, "skills", "alternate", "SKILL.md"), "---\nname: foo\ndescription: Use when foo.\n---\nother skill\n")
			testutil.Git(t, f.remote, "add", "-A")
			testutil.Git(t, f.remote, "commit", "-qm", "alternate paths")
			second := testutil.Git(t, f.remote, "rev-parse", "HEAD")
			refs := [2]string{first, second}
			if different == "path" {
				refs[0] = second
			}
			projects := [2]string{f.project, t.TempDir()}
			var configs [2]*config.Config
			var locks [2]*lockfile.File
			for i, project := range projects {
				includePath, skillPath := "", "skills/foo"
				if different == "path" && i == 1 {
					includePath, skillPath = "alternate", "skills/alternate"
				}
				writeProjectCacheConfig(t, project, f.remote, refs[i], includePath, skillPath)
				var err error
				configs[i], err = loadWithResolvers(context.Background(), project, config.WithoutLocal())
				require.NoError(t, err)
				var problems []string
				locks[i], problems = BuildLock(configs[i], nil)
				require.Empty(t, problems)
				require.Len(t, locks[i].Include, 1)
				require.Len(t, locks[i].Skill, 1)
				require.NoError(t, lockfile.Save(configs[i].ConfigDir, locks[i]))
			}
			lockPolicy.Mode, lockPolicy.Offline = LockFrozen, true
			for i := range projects {
				loaded, err := loadWithResolvers(context.Background(), projects[i], config.WithoutLocal())
				require.NoError(t, err, "project %d must reload its frozen cache offline", i)
				configs[i] = loaded
				problems, cached := CheckLock(configs[i], locks[i])
				assert.Equal(t, 2, cached, "project %d must verify both cached sources", i)
				assert.Empty(t, problems, "project %d must retain its own cached content", i)
			}
		})
	}
}

func writeProjectCacheConfig(t *testing.T, project, remote, ref, includePath, skillPath string) {
	t.Helper()
	body := `version = "5.0"
name = "independent"
presets = ["claude"]
[[includes]]
name = "shared"
source = "file://REMOTE"
ref = "REF"
path = "INCLUDE_PATH"
[[installed_skills]]
name = "foo"
source = "file://REMOTE"
ref = "REF"
path = "SKILL_PATH"
`
	body = strings.NewReplacer("REMOTE", filepath.ToSlash(remote), "REF", ref, "INCLUDE_PATH", includePath, "SKILL_PATH", skillPath).Replace(body)
	writeTestFile(t, filepath.Join(project, ".ai-rulez", "config.toml"), body)
}

func TestGitCacheIdentityShouldPreserveNormalizedRemotesAndNames(t *testing.T) {
	for _, factory := range []string{"include", "skill"} {
		t.Run(factory, func(t *testing.T) {
			create := func(url, path, ref string) string {
				t.Helper()
				if factory == "skill" {
					source, err := NewSkillGitSource("shared", url, path, ref, "")
					require.NoError(t, err)
					return source.cacheDir
				}
				source, err := NewGitSource("shared", url, path, ref, t.TempDir(), nil, "")
				require.NoError(t, err)
				return source.cacheDir
			}
			first := create("https://github.com/owner/repo.git", "bundle", strings.Repeat("a", 40))
			assert.Equal(t, first, create("git@github.com:owner/repo.git", "bundle", strings.Repeat("a", 40)))
			assert.Equal(t, first, create("git+https://github.com/owner/repo.git/", "bundle", strings.Repeat("a", 40)))
			assert.NotEqual(t, first, create("https://github.com/owner/repo.git", "other", strings.Repeat("a", 40)))
			assert.NotEqual(t, first, create("https://github.com/owner/repo.git", "bundle", strings.Repeat("b", 40)))
			assert.True(t, CacheDirMatches(filepath.Base(first), "shared"))
			assert.False(t, CacheDirMatches(filepath.Base(first), "shared-extra"))
		})
	}
}
