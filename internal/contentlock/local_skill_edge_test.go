package contentlock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func localSkillItems(snap *Snapshot) []lockfile.Item {
	var out []lockfile.Item
	for _, it := range snap.Items {
		if it.Kind == KindLocalSkill {
			out = append(out, it)
		}
	}
	return out
}

func TestCompute_LocalInstalledSkillSourceSelection(t *testing.T) {
	tests := []struct {
		name      string
		entries   func(f *fixture) []config.InstalledSkillConfig
		env       func(f *fixture) ambient.Env
		skillAt   string // relative to the project root
		wantPaths []string
	}{
		{
			name: "a local_override entry is a development shortcut and is not pinned",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "vendor", LocalOverride: "vendor"}}
			},
			skillAt: "vendor/demo",
		},
		{
			name: "a git entry is pinned by commit, never as a local skill",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "git@example.com:o/r.git"}}
			},
			skillAt: "vendor/demo",
		},
		{
			name: "a tilde source without a home directory is skipped",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "~/skills"}}
			},
			env:     func(*fixture) ambient.Env { return ambient.MapEnv{} },
			skillAt: "home/skills/demo",
		},
		{
			name: "overlapping sources: the first entry that contains the file records it",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{
					{Name: "wide", Source: "vendor"},
					{Name: "narrow", Source: "vendor/skills"},
				}
			},
			skillAt:   "vendor/skills/demo",
			wantPaths: []string{"vendor/skills/demo"},
		},
		{
			name: "overlapping sources in the other order record the other prefix",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{
					{Name: "narrow", Source: "./vendor/skills"},
					{Name: "wide", Source: "./vendor"},
				}
			},
			skillAt:   "vendor/skills/demo",
			wantPaths: []string{"./vendor/skills/demo"},
		},
		{
			name: "a trailing slash on the source is not doubled",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "vendor/"}}
			},
			skillAt:   "vendor/demo",
			wantPaths: []string{"vendor/demo"},
		},
		{
			name: "a file outside every local source is not pinned",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "elsewhere"}}
			},
			skillAt: "vendor/demo",
		},
		{
			name: "a sibling directory sharing a name prefix is not inside the source",
			entries: func(*fixture) []config.InstalledSkillConfig {
				return []config.InstalledSkillConfig{{Name: "demo", Source: "vendor/skill"}}
			},
			skillAt: "vendor/skills/demo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t)
			dir := filepath.Join(f.root, filepath.FromSlash(tt.skillAt))
			writeAt(t, dir, "SKILL.md", "---\ndescription: demo\n---\n# Demo\n")
			f.cfg.InstalledSkills = tt.entries(f)
			f.cfg.Content.Skills = []config.ContentFile{{Name: "demo", Path: filepath.Join(dir, "SKILL.md")}}
			if tt.env != nil {
				f.cfg.Host = ambient.Host{Env: tt.env(f)}
			}
			// Act
			snap, err := Compute(f.cfg, Options{})
			// Assert
			require.NoError(t, err)
			var got []string
			for _, it := range localSkillItems(snap) {
				got = append(got, it.Path)
			}
			assert.Equal(t, tt.wantPaths, got)
		})
	}
}

func TestCompute_LocalInstalledSkillAtTheSourceRootRecordsTheSourceAsWritten(t *testing.T) {
	// Arrange: the source is the skill directory itself
	f := newFixture(t)
	dir := filepath.Join(f.root, "vendor", "demo")
	writeAt(t, dir, "SKILL.md", "# Demo\n")
	f.cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: "demo", Source: "vendor/demo", Path: "."}}
	f.cfg.Content.Skills = []config.ContentFile{{Name: "demo", Path: filepath.Join(dir, "SKILL.md")}}
	// Act
	snap, err := Compute(f.cfg, Options{})
	// Assert
	require.NoError(t, err)
	items := localSkillItems(snap)
	require.Len(t, items, 1)
	assert.Equal(t, "vendor/demo", items[0].Path)
	assert.Equal(t, "demo", items[0].ID)
}

func TestCompute_LocalInstalledSkillSourceVanishesAfterTheLock(t *testing.T) {
	// Arrange: lock a local skill
	f, dir := installedFixture(t, "vendor/skills", func(f *fixture) string { return filepath.Join(f.root, "vendor", "skills") })
	snap, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	var lock lockfile.File
	lock.Version = lockfile.Version
	Build(&lock, snap)
	require.True(t, Compare(&lock, snap).InSync)

	t.Run("the skill is no longer resolved", func(t *testing.T) {
		// Arrange: the load no longer yields the skill
		f.cfg.Content.Skills = nil
		// Act
		after, err := Compute(f.cfg, Options{})
		require.NoError(t, err)
		diff := Compare(&lock, after)
		// Assert
		assert.False(t, diff.InSync, "a pinned local skill that disappears is drift, not a pass")
		require.Len(t, diff.Changes, 1)
		assert.Equal(t, KindLocalSkill, diff.Changes[0].Kind)
		assert.Equal(t, "demo", diff.Changes[0].ID)
		assert.Equal(t, "removed", diff.Changes[0].Change)
	})

	t.Run("the skill file vanishes while still listed", func(t *testing.T) {
		// Arrange
		f.cfg.Content.Skills = []config.ContentFile{{Name: "demo", Path: filepath.Join(dir, "demo", "SKILL.md")}}
		require.NoError(t, os.RemoveAll(dir))
		// Act
		_, err := Compute(f.cfg, Options{})
		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read skill for the lock")
	})
}
