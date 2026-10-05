package plugin

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEvalFixture(t *testing.T) (project string, skills []config.ContentFile) {
	t.Helper()
	project = t.TempDir()
	files := map[string]string{
		".ai-rulez/skills/alpha/SKILL.md":           "---\nname: alpha\n---\nbody\n",
		".ai-rulez/skills/alpha/references/r.md":    "ref\n",
		".ai-rulez/skills/alpha/evals/case.yaml":    "prompt: hi\n",
		".ai-rulez/skills/alpha/evals/graders/g.py": "print(1)\n",
		".ai-rulez/skills/beta/SKILL.md":            "---\nname: beta\n---\nbody\n",
		".ai-rulez/evals/alpha/extra.yaml":          "a: 1\n",
		".ai-rulez/evals/beta/extra.yaml":           "b: 1\n",
		".ai-rulez/evals/shared.yaml":               "shared: true\n",
	}
	for name, body := range files {
		path := filepath.Join(project, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	for _, name := range []string{"alpha", "beta"} {
		skills = append(skills, config.ContentFile{
			Name: name,
			Path: filepath.Join(project, ".ai-rulez", "skills", name, "SKILL.md"),
		})
	}
	return project, skills
}

func bundledPaths(t *testing.T, outputs []config.OutputFile, root string) []string {
	t.Helper()
	var paths []string
	for _, output := range outputs {
		rel, err := filepath.Rel(root, output.Path)
		require.NoError(t, err)
		paths = append(paths, filepath.ToSlash(rel))
	}
	sort.Strings(paths)
	return paths
}

func TestBundleContent_Evals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		edit     func(m *Manifest, skills []config.ContentFile)
		want     []string
		mustMiss []string
	}{
		{
			name: "default keeps evals out of the bundle",
			edit: func(*Manifest, []config.ContentFile) {},
			want: []string{
				"skills/alpha/SKILL.md", "skills/alpha/references/r.md", "skills/beta/SKILL.md",
			},
		},
		{
			name: "include_evals bundles skill cases and the project tree",
			edit: func(m *Manifest, _ []config.ContentFile) { m.IncludeEvals = true },
			want: []string{
				"evals/alpha/extra.yaml", "evals/beta/extra.yaml", "evals/shared.yaml",
				"skills/alpha/SKILL.md", "skills/alpha/evals/case.yaml", "skills/alpha/evals/graders/g.py",
				"skills/alpha/references/r.md", "skills/beta/SKILL.md",
			},
		},
		{
			name: "per-skill-only tree drops cases of skills that are not bundled",
			edit: func(m *Manifest, skills []config.ContentFile) {
				m.IncludeEvals = true
				m.EvalsPerSkillOnly = true
				m.Skills = skills[:1]
			},
			want: []string{
				"evals/alpha/extra.yaml",
				"skills/alpha/SKILL.md", "skills/alpha/evals/case.yaml", "skills/alpha/evals/graders/g.py",
				"skills/alpha/references/r.md",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			project, skills := writeEvalFixture(t)
			m := &Manifest{
				Skills:    skills,
				EvalsDir:  filepath.Join(project, ".ai-rulez", "evals"),
				SourceDir: project,
			}
			tt.edit(m, skills)

			outputs, err := bundleContent(m, filepath.Join(project, "out"), contentLayout{Skills: true})
			require.NoError(t, err)
			assert.Equal(t, tt.want, bundledPaths(t, outputs, filepath.Join(project, "out")))
		})
	}
}

func TestBundleContent_EvalsProvenanceCoversCases(t *testing.T) {
	t.Parallel()

	project, skills := writeEvalFixture(t)
	out := filepath.Join(project, "out")
	m := &Manifest{Skills: skills, IncludeEvals: true, EvalsDir: filepath.Join(project, ".ai-rulez", "evals")}
	outputs, err := bundleContent(m, out, contentLayout{Skills: true})
	require.NoError(t, err)
	decorated, err := AddProvenance(outputs, out)
	require.NoError(t, err)
	for _, output := range decorated {
		require.NoError(t, os.MkdirAll(filepath.Dir(output.Path), 0o750))
		require.NoError(t, os.WriteFile(output.Path, output.RawContent, 0o600))
	}
	require.NoError(t, VerifyProvenance(out))

	// Tampering with a case must now fail verification.
	require.NoError(t, os.WriteFile(filepath.Join(out, "skills", "alpha", "evals", "case.yaml"), []byte("tampered\n"), 0o600))
	assert.Error(t, VerifyProvenance(out), "a modified eval case must be detected by verify --plugin")
}

func TestBundleContent_MissingEvalTreeIsNotAnError(t *testing.T) {
	t.Parallel()

	project, skills := writeEvalFixture(t)
	m := &Manifest{Skills: skills, IncludeEvals: true, EvalsDir: filepath.Join(project, "nope")}
	_, err := bundleContent(m, filepath.Join(project, "out"), contentLayout{Skills: true})
	require.NoError(t, err)
}
