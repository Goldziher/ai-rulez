package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeSetup_ViewKeyRoundTrips(t *testing.T) {
	tests := []struct {
		name  string
		setup ServeSetup
		want  string
	}{
		{"default view", ServeSetup{}, ""},
		{"role", ServeSetup{Role: "backend"}, "role:backend"},
		{"profile", ServeSetup{Profile: "team"}, "profile:team"},
		{"include-static", ServeSetup{IncludeStatic: true}, "static"},
		{"role with static", ServeSetup{Role: "backend", IncludeStatic: true}, "role:backend+static"},
		{"sources are sorted", ServeSetup{Sources: []string{"./b-skills", "./a-skills"}}, "source:cli-a-skills+source:cli-b-skills"},
		{"targets", ServeSetup{Preset: "cursor"}, "targets:cursor"},
		{"role with targets", ServeSetup{Role: "backend", Preset: "cursor"}, "role:backend+targets:cursor"},
		{"everything", ServeSetup{Profile: "p", Preset: "codex", IncludeStatic: true, Sources: []string{"./x"}}, "profile:p+targets:codex+static+source:cli-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			key := tt.setup.ViewKey()

			// Assert
			assert.Equal(t, tt.want, key)
			back, ok := ServeSetup{}.withView(key)
			if len(tt.setup.Sources) > 0 || key == "" {
				assert.False(t, ok, "a view with a --source, and the default view, are not rebuilt from a key")
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.setup.Role, back.Role)
			assert.Equal(t, tt.setup.Profile, back.Profile)
			assert.Equal(t, tt.setup.Preset, back.Preset)
			assert.Equal(t, tt.setup.IncludeStatic, back.IncludeStatic)
		})
	}
	assert.Equal(t, []string{"cli-a", "cli-b"}, ViewKeySources("role:r+source:cli-a+source:cli-b"))
}

const viewsConfig = baseConfig + `
[lock]
enforce = true

[[roles]]
name = "backend"
domains = ["backend"]

[[roles]]
name = "frontend"
domains = ["frontend"]
`

func viewsProject(t *testing.T) string {
	t.Helper()
	return project(t, viewsConfig, map[string]string{
		"skills/shared/SKILL.md":                    skillFile("shared", "Shared by everyone", "delivery: served\n"),
		"skills/static-one/SKILL.md":                skillFile("static-one", "Only in the static view", "delivery: static\n"),
		"domains/backend/skills/migrate/SKILL.md":   skillFile("migrate", "Run database migrations", "delivery: served\n"),
		"domains/frontend/skills/ui-kit/SKILL.md":   skillFile("ui-kit", "Use the component kit", "delivery: served\n"),
		"domains/frontend/skills/ui-audit/SKILL.md": skillFile("ui-audit", "Audit accessibility", "delivery: served\n"),
	})
}

func servedNames(entries []lockfile.Entry, view string) []string {
	var out []string
	for _, e := range entries {
		if e.View == view {
			out = append(out, e.Name)
		}
	}
	return out
}

func TestLockViews_OneSetOfServedPinsPerView(t *testing.T) {
	// Arrange
	root := viewsProject(t)
	setup := &ServeSetup{WorkDir: root, NoWatch: true, Offline: true, CacheDir: filepath.Join(t.TempDir(), "cache")}

	// Act
	res, err := setup.LockViews(context.Background(), []ServeSetup{{IncludeStatic: true}})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"", "role:backend", "role:frontend", "static"}, res.Views)
	assert.ElementsMatch(t, []string{"migrate", "shared", "ui-audit", "ui-kit"}, servedNames(res.Served, ""))
	assert.ElementsMatch(t, []string{"migrate", "shared"}, servedNames(res.Served, "role:backend"))
	assert.ElementsMatch(t, []string{"shared", "ui-audit", "ui-kit"}, servedNames(res.Served, "role:frontend"))
	assert.ElementsMatch(t, []string{"migrate", "shared", "static-one", "ui-audit", "ui-kit"}, servedNames(res.Served, "static"))
	assert.Empty(t, res.Refused)
}

func lockAll(t *testing.T, root string, extras ...ServeSetup) {
	t.Helper()
	setup := &ServeSetup{WorkDir: root, NoWatch: true, Offline: true}
	res, err := setup.LockViews(context.Background(), extras)
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	for _, e := range res.Served {
		lock.Set(lockfile.KindServed, e)
	}
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), lock))
}

func TestServeSetup_EnforceChecksThePinsOfTheViewTheServerStartsWith(t *testing.T) {
	// Arrange: the lock pins the default view, both roles and a static view.
	root := viewsProject(t)
	lockAll(t, root, ServeSetup{IncludeStatic: true})

	tests := []struct {
		name  string
		setup ServeSetup
		want  []string
	}{
		{"default view", ServeSetup{}, []string{"migrate", "shared", "ui-audit", "ui-kit"}},
		{"role view", ServeSetup{Role: "backend"}, []string{"migrate", "shared"}},
		{"static view", ServeSetup{IncludeStatic: true}, []string{"migrate", "shared", "static-one", "ui-audit", "ui-kit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := tt.setup
			setup.WorkDir, setup.Frozen = root, true

			// Act
			srv := newServerFor(t, &setup)

			// Assert
			assert.Equal(t, tt.want, catalogNames(srv.Catalog()))
		})
	}
}

func TestServeSetup_EnforceRefusesAViewTheLockDoesNotPin(t *testing.T) {
	// Arrange: only the default view and the roles are pinned, not the static view.
	root := viewsProject(t)
	lockAll(t, root)

	// Act
	srv := newServerFor(t, &ServeSetup{WorkDir: root, IncludeStatic: true})

	// Assert: static-one is in no pinned view, so enforcement refuses it.
	assert.NotContains(t, catalogNames(srv.Catalog()), "static-one")
	refusal, refused := srv.Catalog().Refusal("static-one")
	require.True(t, refused)
	assert.Equal(t, CodeServedLockMismatch, refusal.Code)
	assert.Equal(t, "static", refusal.View)
	assert.Contains(t, catalogNames(srv.Catalog()), "shared", "skills pinned in the default view are still served")
}

func TestServeSetup_LockWrittenBeforeViewsExistedStillCoversEveryView(t *testing.T) {
	// Arrange: a lock whose served pins have no view, as older releases wrote them.
	root := viewsProject(t)
	setup := &ServeSetup{WorkDir: root, NoWatch: true, Offline: true}
	res, err := setup.LockViews(context.Background(), nil)
	require.NoError(t, err)
	legacy := &lockfile.File{Version: lockfile.Version}
	for _, e := range res.Served {
		if e.View == "" {
			legacy.Set(lockfile.KindServed, e)
		}
	}
	require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), legacy))

	// Act
	srv := newServerFor(t, &ServeSetup{WorkDir: root, Role: "backend", Frozen: true})
	problems, err := setup.ServedProblems(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"migrate", "shared"}, catalogNames(srv.Catalog()))
	assert.Empty(t, problems)
}

func TestServedProblems_NamesTheViewThatDisagrees(t *testing.T) {
	// Arrange
	root := viewsProject(t)
	lockAll(t, root, ServeSetup{IncludeStatic: true})
	writeFile(t, root, ".ai-rulez/domains/backend/skills/migrate/SKILL.md", skillFile("migrate", "Run database migrations, edited", "delivery: served\n"))
	setup := &ServeSetup{WorkDir: root, NoWatch: true}

	// Act
	all, err := setup.ServedProblems(context.Background())
	onlyFrontend, onlyErr := (&ServeSetup{WorkDir: root, NoWatch: true}).ServedProblems(context.Background())

	// Assert
	require.NoError(t, err)
	require.NoError(t, onlyErr)
	assert.Equal(t, all, onlyFrontend)
	var text string
	for _, p := range all {
		text += p + "\n"
	}
	assert.Contains(t, text, "served migrate: digest")
	assert.Contains(t, text, "served migrate (view role:backend): digest")
	assert.Contains(t, text, "served migrate (view static): digest", "the view the lock recorded is checked too")
	assert.NotContains(t, text, "frontend")
}

func TestViewNamesARemovedRole(t *testing.T) {
	roles := []string{"dev", "ops"}
	tests := []struct {
		name string
		view ServeSetup
		want bool
	}{
		{"a defined role", ServeSetup{Role: "dev"}, false},
		{"a role that left the config", ServeSetup{Role: "gone"}, true},
		{"a profile view", ServeSetup{Profile: "gone"}, false},
		{"the static view", ServeSetup{IncludeStatic: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, namesRemovedRole(roles, tt.view))
		})
	}
}

// A lock run (ignoreLock) evaluates every view of one project: the config, the
// lock and the authored-skill digests are loaded once and shared by the views,
// not rebuilt for each.
func TestBuildAll_IgnoringTheLockLoadsTheProjectOnce(t *testing.T) {
	// Arrange
	root := viewsProject(t)
	lockAll(t, root, ServeSetup{IncludeStatic: true})
	setup := &ServeSetup{WorkDir: root, NoWatch: true, Offline: true}

	// Act
	views, err := setup.buildAll(context.Background(), buildOptions{admit: true, ignoreLock: true}, []ServeSetup{{IncludeStatic: true}})

	// Assert
	require.NoError(t, err)
	require.Len(t, views, 4)
	require.NotNil(t, views[0].lock)
	for _, v := range views[1:] {
		assert.Same(t, views[0].cfg, v.cfg, "view %q loaded the config again", v.view)
		assert.Same(t, views[0].lock, v.lock, "view %q read the lock again", v.view)
	}
}
