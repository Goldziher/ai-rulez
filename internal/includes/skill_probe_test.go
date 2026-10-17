package includes

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func resolvedNames(skills []config.ContentFile) []string {
	names := make([]string, len(skills))
	for i := range skills {
		names[i] = skills[i].Name
	}
	return names
}

func TestResolveInstalledSkills_RecursiveDiscovery(t *testing.T) {
	deep := strings.Repeat("d/", maxSkillDepth-1) // a skill at depth maxSkillDepth
	tooDeep := strings.Repeat("d/", maxSkillDepth)

	tests := []struct {
		name    string
		layout  []string // directories that hold a SKILL.md, relative to the source
		entry   config.InstalledSkillConfig
		want    []string
		wantErr string
	}{
		{
			name:   "skills below nested directories",
			layout: []string{"skills/a", "skills/group/b", "skills/group/sub/c"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "a skill directory is not searched further",
			layout: []string{"skills/a", "skills/a/inner"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			want:   []string{"a"},
		},
		{
			name:   "depth limit includes the last level",
			layout: []string{"skills/" + deep + "ok"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			want:   []string{"ok"},
		},
		{
			name:    "depth limit excludes deeper skills",
			layout:  []string{"skills/" + tooDeep + "lost"},
			entry:   config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			wantErr: "no SKILL.md",
		},
		{
			name:   "node_modules and hidden directories are skipped",
			layout: []string{"skills/a", "skills/node_modules/b", "skills/.cache/c"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			want:   []string{"a"},
		},
		{
			name:   "include, exclude and name_prefix",
			layout: []string{"skills/x/alpha", "skills/y/beta", "skills/y/alpha-wip"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*a"}, Exclude: []string{"*-wip"}, NamePrefix: "p-"},
			want:   []string{"p-alpha", "p-beta"},
		},
		{
			name:   "duplicate names keep the shallowest",
			layout: []string{"skills/z/dup", "skills/dup"},
			entry:  config.InstalledSkillConfig{Path: "skills", Include: []string{"*"}},
			want:   []string{"dup"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			for _, rel := range tt.layout {
				writeSkill(t, filepath.Join(project, filepath.FromSlash(rel)), path.Base(rel))
			}
			entry := tt.entry
			entry.Name, entry.Source = "entry", project
			cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{entry}}

			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedNames(skills))
		})
	}
}

func TestResolveInstalledSkills_DuplicateKeepsShallowestSkill(t *testing.T) {
	// Arrange: two skills called dup, one deeper.
	project := t.TempDir()
	writeSkill(t, filepath.Join(project, "skills", "a", "b", "dup"), "deep")
	writeSkill(t, filepath.Join(project, "skills", "dup"), "shallow")
	cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{
		Name: "e", Source: project, Path: "skills", Include: []string{"*"},
	}}}

	// Act
	skills, err := ResolveInstalledSkills(t.Context(), cfg, "")

	// Assert
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Contains(t, skills[0].Content, "# shallow")
}

func TestResolveInstalledSkills_NeverFollowsSymlinkedDirectories(t *testing.T) {
	// Arrange: a skill reachable only through a symlinked directory.
	root := t.TempDir()
	project := filepath.Join(root, "project")
	outside := filepath.Join(root, "outside")
	writeSkill(t, filepath.Join(outside, "evil"), "evil")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "skills"), 0o755))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(project, "skills", "link"))
	writeSkill(t, filepath.Join(project, "skills", "good"), "good")
	cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{
		Name: "e", Source: project, Path: "skills", Include: []string{"*"},
	}}}

	// Act
	skills, err := ResolveInstalledSkills(t.Context(), cfg, "")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"good"}, resolvedNames(skills))
}

func TestResolveInstalledSkills_ProbesConventionalDirectories(t *testing.T) {
	tests := []struct {
		name   string
		layout []string
		entry  config.InstalledSkillConfig
		want   []string
		err    string
	}{
		{"skills/", []string{"skills/other/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".claude/skills", []string{".claude/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".agents/skills", []string{".agents/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".codex/skills", []string{".codex/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".cursor/skills", []string{".cursor/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".gemini/skills", []string{".gemini/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".github/skills", []string{".github/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{".opencode/skills", []string{".opencode/skills/x"}, config.InstalledSkillConfig{Name: "x"}, []string{"x"}, ""},
		{
			"a root SKILL.md is one skill named after the entry",
			[]string{"."}, config.InstalledSkillConfig{Name: "solo", NamePrefix: "p-"}, []string{"p-solo"}, "",
		},
		{
			"include selects several across locations",
			[]string{".agents/skills/a", ".claude/skills/b", ".claude/skills/skip-me"},
			config.InstalledSkillConfig{Name: "all", Include: []string{"*"}, Exclude: []string{"skip-*"}}, []string{"a", "b"}, "",
		},
		{
			"mirrored locations keep one copy, the earlier location",
			[]string{".agents/skills/m", ".claude/skills/m"},
			config.InstalledSkillConfig{Name: "all", Include: []string{"*"}}, []string{"m"}, "",
		},
		{
			"the default skills/<name> location wins over probing",
			[]string{"skills/x", ".claude/skills/other"},
			config.InstalledSkillConfig{Name: "x"}, []string{"x"}, "",
		},
		{
			"a single-skill entry needs a directory with its name",
			[]string{"skills/a", "skills/b"},
			config.InstalledSkillConfig{Name: "missing"}, nil, "no skill directory called \"missing\"",
		},
		{
			"an explicit path disables probing",
			[]string{".claude/skills/x"},
			config.InstalledSkillConfig{Name: "x", Path: "nowhere"}, nil, "no SKILL.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			for _, rel := range tt.layout {
				writeSkill(t, filepath.Join(project, filepath.FromSlash(rel)), path.Base(rel))
			}
			entry := tt.entry
			entry.Source = project
			cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{entry}}

			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")

			// Assert
			if tt.err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedNames(skills))
		})
	}
}

func TestResolveInstalledSkills_GitProbesConventionalDirectories(t *testing.T) {
	// Arrange: a repository that keeps its skills under .claude/skills, nothing
	// at skills/<name>.
	repoDir := initLocalRepo(t)
	commitFile(t, repoDir, ".claude/skills/alpha/SKILL.md", "---\nname: alpha\n---\n# Alpha\n")
	commitFile(t, repoDir, ".claude/skills/beta/SKILL.md", "---\nname: beta\n---\n# Beta\n")
	testutil.IsolateHome(t) // keep the fetch cache out of the real home
	tests := []struct {
		name  string
		entry config.InstalledSkillConfig
		want  []string
	}{
		{"single skill by name", config.InstalledSkillConfig{Name: "alpha"}, []string{"alpha"}},
		{"several by include", config.InstalledSkillConfig{Name: "all", Include: []string{"*"}}, []string{"alpha", "beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			entry := tt.entry
			entry.Source = "file://" + repoDir
			cfg := &config.Config{BaseDir: t.TempDir(), InstalledSkills: []config.InstalledSkillConfig{entry}}

			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedNames(skills))
		})
	}
}

func TestConventionalSkillDirs_AreHarnessLayouts(t *testing.T) {
	// The list is a fresh copy each call, so a caller cannot reorder the probe order.
	first := ConventionalSkillDirs()
	first[0] = "changed"
	assert.Equal(t, "skills", ConventionalSkillDirs()[0])
}
