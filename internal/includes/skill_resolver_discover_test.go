package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// writeSkill writes a minimal SKILL.md for name below dir.
func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: "+name+"\n---\n# "+name+"\n"), 0o644))
}

// A single entry pointing at a directory of skills resolves every skill below
// it, selected by include/exclude and named with name_prefix. A single-skill
// entry (a root that holds a SKILL.md) is unchanged.
func TestResolveInstalledSkills_MultiSkillEntry(t *testing.T) {
	// Arrange: one local repository holding several skills.
	repo := t.TempDir()
	skillsDir := filepath.Join(repo, "skills")
	for _, name := range []string{"alpha", "beta", "wip"} {
		writeSkill(t, filepath.Join(skillsDir, name), name)
	}
	// A subdirectory without a SKILL.md is not a skill.
	require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "not-a-skill"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "not-a-skill", "README.md"), []byte("x"), 0o644))
	// A single skill addressed directly, for the unchanged path.
	writeSkill(t, filepath.Join(repo, "solo"), "solo")

	tests := []struct {
		name       string
		entry      config.InstalledSkillConfig
		wantNames  []string
		wantPrefix string
	}{
		{
			name:      "all skills, sorted",
			entry:     config.InstalledSkillConfig{Name: "repo", Source: repo, Path: "skills"},
			wantNames: []string{"alpha", "beta", "wip"},
		},
		{
			name:      "include selects several with one entry",
			entry:     config.InstalledSkillConfig{Name: "repo", Source: repo, Path: "skills", Include: []string{"a*", "b*"}},
			wantNames: []string{"alpha", "beta"},
		},
		{
			name:      "exclude drops a match",
			entry:     config.InstalledSkillConfig{Name: "repo", Source: repo, Path: "skills", Include: []string{"*a*"}, Exclude: []string{"*wip"}},
			wantNames: []string{"alpha", "beta"},
		},
		{
			name:      "exclude wins over include",
			entry:     config.InstalledSkillConfig{Name: "repo", Source: repo, Path: "skills", Include: []string{"alpha", "wip"}, Exclude: []string{"wip"}},
			wantNames: []string{"alpha"},
		},
		{
			name:      "name_prefix renames every resolved skill",
			entry:     config.InstalledSkillConfig{Name: "repo", Source: repo, Path: "skills", NamePrefix: "team-"},
			wantNames: []string{"team-alpha", "team-beta", "team-wip"},
		},
		{
			name:      "a root with a SKILL.md is one skill named after the entry",
			entry:     config.InstalledSkillConfig{Name: "solo", Source: repo, Path: "solo"},
			wantNames: []string{"solo"},
		},
		{
			name:      "a single-skill entry takes the prefix too",
			entry:     config.InstalledSkillConfig{Name: "solo", Source: repo, Path: "solo", NamePrefix: "team-"},
			wantNames: []string{"team-solo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{BaseDir: repo, InstalledSkills: []config.InstalledSkillConfig{tt.entry}}

			// Act
			skills, err := ResolveInstalledSkills(context.Background(), cfg, "")

			// Assert
			require.NoError(t, err)
			names := make([]string, len(skills))
			for i := range skills {
				names[i] = skills[i].Name
				assert.Contains(t, skills[i].Content, "# "+tt.wantNames[i][len(tt.entry.NamePrefix):],
					"the body of %s is scanned from its own directory", tt.wantNames[i])
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

// The resolver stamps each entry's scope on every file it resolved.
func TestResolveInstalledSkills_StampsEntryScope(t *testing.T) {
	// Arrange
	repo := t.TempDir()
	writeSkill(t, filepath.Join(repo, "skills", "alpha"), "alpha")
	writeSkill(t, filepath.Join(repo, "skills", "beta"), "beta")
	cfg := &config.Config{
		BaseDir: repo,
		InstalledSkills: []config.InstalledSkillConfig{{
			Name: "repo", Source: repo, Path: "skills",
			Profiles: []string{"p"}, Domain: "d", Trust: config.TrustWarn,
		}},
	}

	// Act
	skills, err := ResolveInstalledSkills(context.Background(), cfg, "")

	// Assert
	require.NoError(t, err)
	require.Len(t, skills, 2)
	for i := range skills {
		assert.Equal(t, []string{"p"}, skills[i].Profiles)
		assert.Equal(t, "d", skills[i].Domain)
		assert.Equal(t, config.TrustWarn, skills[i].Trust)
	}
}
