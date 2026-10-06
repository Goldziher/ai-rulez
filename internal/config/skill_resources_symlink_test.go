package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkAt creates link -> target, creating the link's parent directory.
func linkAt(t *testing.T, target, link string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	symlinkOrSkip(t, target, link)
}

func TestProjectSkillResources_SymlinkPolicy(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, project, outside, skill string)
		wantRes      []string
		wantProblems int
	}{
		{
			name: "file link inside project is followed",
			setup: func(t *testing.T, project, _, skill string) {
				write(t, filepath.Join(project, "docs", "api.md"), "# api\n")
				linkAt(t, filepath.Join(project, "docs", "api.md"), filepath.Join(skill, "references", "api.md"))
			},
			wantRes: []string{"references/api.md"},
		},
		{
			name: "file link outside project is refused and reported",
			setup: func(t *testing.T, _, outside, skill string) {
				write(t, filepath.Join(outside, "key"), "SECRET")
				linkAt(t, filepath.Join(outside, "key"), filepath.Join(skill, "references", "key.md"))
			},
			wantProblems: 1,
		},
		{
			name: "directory link inside project is followed",
			setup: func(t *testing.T, project, _, skill string) {
				write(t, filepath.Join(project, "shared", "a.md"), "# a\n")
				linkAt(t, filepath.Join(project, "shared"), filepath.Join(skill, "references", "shared"))
			},
			wantRes: []string{"references/shared/a.md"},
		},
		{
			name: "kind directory link outside project is refused and reported",
			setup: func(t *testing.T, _, outside, skill string) {
				write(t, filepath.Join(outside, "scripts", "x.sh"), "SECRET")
				linkAt(t, filepath.Join(outside, "scripts"), filepath.Join(skill, "scripts"))
			},
			wantProblems: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project, outside := t.TempDir(), t.TempDir()
			skill := filepath.Join(project, ".ai-rulez", "skills", "s")
			write(t, filepath.Join(skill, "SKILL.md"), "---\nname: s\n---\nbody\n")
			tt.setup(t, project, outside, skill)
			var warned bytes.Buffer
			old := contentWarnWriter
			contentWarnWriter = &warned
			t.Cleanup(func() { contentWarnWriter = old })
			s := newProjectScanner(osView(project))

			// Act
			skills, err := s.skills(filepath.Join(project, ".ai-rulez", "skills"), nil)

			// Assert
			require.NoError(t, err)
			require.Len(t, skills, 1)
			var got []string
			for _, r := range skills[0].Resources {
				got = append(got, r.RelPath)
			}
			assert.ElementsMatch(t, tt.wantRes, got)
			assert.Len(t, s.problems, tt.wantProblems)
			if tt.wantProblems > 0 {
				assert.Contains(t, warned.String(), "refusing symlinked content")
				assert.NotContains(t, warned.String(), "SECRET")
			}
		})
	}
}

func TestIncludedSkillResources_NeverFollowSymlinks(t *testing.T) {
	project := t.TempDir()
	write(t, filepath.Join(project, "docs", "api.md"), "# api\n")
	skill := filepath.Join(project, "skill")
	write(t, filepath.Join(skill, "SKILL.md"), "x")
	linkAt(t, filepath.Join(project, "docs", "api.md"), filepath.Join(skill, "references", "api.md"))

	res, err := LoadSkillResources(skill)

	require.NoError(t, err)
	assert.Empty(t, res)
}
