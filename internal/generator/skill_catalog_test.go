package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const servedSkillsConfig = `version = "5.0"
name = "served"
gitignore = false
presets = ["claude"]

[profiles]
backend = ["api"]
frontend = ["web"]
`

func servedSkillsProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"config.toml":                                   servedSkillsConfig,
		"skills/alpha/SKILL.md":                         "---\ndescription: Alpha skill\nkeywords: [first]\n---\nALPHA_BODY\n",
		"skills/alpha/references/r.md":                  "REFERENCE_BODY\n",
		"skills/alpha/scripts/run.sh":                   "#!/bin/sh\necho hi\n",
		"domains/api/skills/endpoints/SKILL.md":         "---\ndescription: API endpoints\n---\nENDPOINTS\n",
		"domains/web/skills/components/SKILL.md":        "---\ndescription: UI components\n---\nCOMPONENTS\n",
		"domains/web/skills/components/references/c.md": "C\n",
		"domains/api/rules/style.md":                    "# Style\n\nSTYLE\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return root
}

func TestServedSkills_MatchGeneratedBytesPerProfile(t *testing.T) {
	root := servedSkillsProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	tests := []struct {
		profile string
		want    []string
		domain  map[string]string
	}{
		{"backend", []string{"alpha", "endpoints"}, map[string]string{"alpha": "", "endpoints": "api"}},
		{"frontend", []string{"alpha", "components"}, map[string]string{"alpha": "", "components": "web"}},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			resolved, served, err := NewGenerator(cfg).ServedSkills(tt.profile, "")
			require.NoError(t, err)
			assert.Equal(t, "claude", resolved)

			// Serving must not write anything.
			_, statErr := os.Stat(filepath.Join(root, ".claude"))
			assert.True(t, os.IsNotExist(statErr), "ServedSkills must not write files")

			var got []string
			for _, s := range served {
				got = append(got, s.ID)
				assert.Equal(t, tt.domain[s.ID], s.Domain, s.ID)
			}
			assert.Equal(t, tt.want, got)

			// Generate for real and compare every served file byte for byte.
			cfg2, err := config.LoadConfig(context.Background(), root)
			require.NoError(t, err)
			require.NoError(t, NewGenerator(cfg2).Generate(tt.profile))
			for _, s := range served {
				for _, f := range s.Files {
					onDisk, err := os.ReadFile(filepath.Join(root, ".claude", "skills", s.ID, filepath.FromSlash(f.RelPath)))
					require.NoError(t, err, "%s/%s", s.ID, f.RelPath)
					assert.Equal(t, string(onDisk), string(f.Content), "%s/%s", s.ID, f.RelPath)
				}
			}
			require.NoError(t, os.RemoveAll(filepath.Join(root, ".claude")))
			require.NoError(t, os.RemoveAll(filepath.Join(root, "CLAUDE.md")))
		})
	}
}

func TestServedSkills_SupportingFilesAndKeywords(t *testing.T) {
	root := servedSkillsProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	_, served, err := NewGenerator(cfg).ServedSkills("backend", "claude")
	require.NoError(t, err)
	require.NotEmpty(t, served)
	alpha := served[0]
	require.Equal(t, "alpha", alpha.ID)
	var rels []string
	for _, f := range alpha.Files {
		rels = append(rels, f.RelPath)
	}
	assert.Equal(t, []string{"SKILL.md", "references/r.md", "scripts/run.sh"}, rels)
	assert.Equal(t, []string{"first"}, alpha.Keywords)
	assert.Equal(t, ".ai-rulez/skills/alpha/SKILL.md", alpha.Source)
}

func TestServedSkills_UnknownProfileFails(t *testing.T) {
	root := servedSkillsProject(t)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	_, _, err = NewGenerator(cfg).ServedSkills("nope", "claude")
	require.Error(t, err)
}

func TestIsCommitSHA(t *testing.T) {
	tests := map[string]bool{
		"0123456789abcdef0123456789abcdef01234567": true,
		"main": false, "v1.2.3": false, "": false,
		"0123456789ABCDEF0123456789ABCDEF01234567": false,
	}
	for ref, want := range tests {
		assert.Equal(t, want, isCommitSHA(ref), ref)
	}
}

func TestApplyIncludeOrigin(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "work", "proj")
	cache := filepath.Join(string(filepath.Separator), "home", "u", ".cache", "ai-rulez", "includes", "team-0123456789ab", "repo")
	cfg := &config.Config{BaseDir: base, Includes: []config.IncludeConfig{
		{Name: "team", Source: "https://github.com/acme/rules.git"},
		{Name: "local", Source: "../shared"},
		{Name: "inproj", Source: "./shared"},
		{Name: "whole", Source: "."},
	}}
	tests := []struct {
		name         string
		path         string
		wantImported bool
		wantSource   string
	}{
		{"authored skill even with an include rooted at the project", filepath.Join(base, ".ai-rulez", "skills", "a", "SKILL.md"), false, ""},
		{"local include inside the project", filepath.Join(base, "shared", ".ai-rulez", "skills", "b", "SKILL.md"), true, "include:inproj/skills/b/SKILL.md"},
		{"git include root skill", filepath.Join(cache, ".ai-rulez", "skills", "a", "SKILL.md"), true, "include:team/skills/a/SKILL.md"},
		{"git include domain skill", filepath.Join(cache, ".ai-rulez", "domains", "d", "skills", "a", "SKILL.md"), true, "include:team/domains/d/skills/a/SKILL.md"},
		{"local include outside the project", filepath.Join(filepath.Dir(base), "shared", ".ai-rulez", "skills", "b", "SKILL.md"), true, "include:local/skills/b/SKILL.md"},
		{"unknown file outside the project", filepath.Join(string(filepath.Separator), "elsewhere", "skills", "c", "SKILL.md"), true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			o := skillOwner{source: tt.path}

			// Act
			applyIncludeOrigin(cfg, nil, tt.path, &o)

			// Assert
			assert.Equal(t, tt.wantImported, o.imported)
			if tt.wantSource != "" {
				assert.Equal(t, tt.wantSource, o.source)
			}
		})
	}
}

func TestServedSkills_FlagsMalformedFrontmatter(t *testing.T) {
	// Arrange
	root := servedSkillsProject(t)
	broken := filepath.Join(root, ".ai-rulez", "skills", "broken", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(broken), 0o755))
	require.NoError(t, os.WriteFile(broken, []byte("---\nname: broken\ndescription: Use when x: y: z\n---\nBODY\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)

	// Act
	_, served, err := NewGenerator(cfg).ServedSkills("backend", "claude")

	// Assert
	require.NoError(t, err)
	flags := map[string]bool{}
	for _, s := range served {
		flags[s.ID] = s.MalformedFrontmatter
	}
	assert.True(t, flags["broken"])
	assert.False(t, flags["alpha"])
}
