package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// TestPassthroughSkillSymlinkedResources pins the policy shared with plain
// generate (config.LoadResources): a symlinked skill resource is followed when
// its target is inside the project and refused, with a warning that names the
// link, otherwise (#407). The bundle walk used to drop every link silently.
func TestPassthroughSkillSymlinkedResources(t *testing.T) {
	tests := []struct {
		name      string
		link      string // path of the link, relative to the skill directory
		target    func(project, outside string) string
		wantFiles map[string]string // bundle-relative path -> content
		wantExec  []string          // bundle-relative paths that keep the executable bit
		wantWarn  bool
	}{
		{
			name:      "in-project file link is copied with the target's mode",
			link:      "references/lnk.py",
			target:    func(project, _ string) string { return filepath.Join(project, "scripts", "analyze.py") },
			wantFiles: map[string]string{"references/lnk.py": "print('hi')\n"},
			wantExec:  []string{"references/lnk.py"},
		},
		{
			name:   "in-project directory link is followed",
			link:   "references/shared",
			target: func(project, _ string) string { return filepath.Join(project, "docs") },
			wantFiles: map[string]string{
				"references/shared/a.md":     "# a\n",
				"references/shared/sub/b.md": "# b\n",
			},
		},
		{
			name:     "file link out of the project is refused with a warning naming it",
			link:     "references/lnk.py",
			target:   func(_, outside string) string { return filepath.Join(outside, "secret.txt") },
			wantWarn: true,
		},
		{
			name:     "directory link out of the project is refused with a warning naming it",
			link:     "references/shared",
			target:   func(_, outside string) string { return outside },
			wantWarn: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			project, outside := t.TempDir(), t.TempDir()
			skillDir := filepath.Join(project, ".ai-rulez", "skills", "analyze")
			writeModeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: analyze\n---\n", 0o644)
			writeModeFile(t, filepath.Join(skillDir, "references", "guide.md"), "# guide\n", 0o644)
			writeModeFile(t, filepath.Join(project, "scripts", "analyze.py"), "print('hi')\n", 0o755)
			writeModeFile(t, filepath.Join(project, "docs", "a.md"), "# a\n", 0o644)
			writeModeFile(t, filepath.Join(project, "docs", "sub", "b.md"), "# b\n", 0o644)
			writeModeFile(t, filepath.Join(outside, "secret.txt"), "TOP SECRET\n", 0o644)
			link := filepath.Join(skillDir, filepath.FromSlash(tc.link))
			testutil.SymlinkOrSkip(t, tc.target(project, outside), link)

			rec := &testutil.LogRecorder{}
			m := &Manifest{Name: "p", SourceDir: project,
				Config: &config.Config{BaseDir: project, Diag: diag.New(nil), Host: ambient.Host{Log: rec}}}
			content := &config.ContentFile{Name: "analyze", Path: filepath.Join(skillDir, "SKILL.md")}
			dest := filepath.Join(project, "out", "analyze")

			// Act
			outs, err := passthroughSkill(m, content, dest, false)

			// Assert
			require.NoError(t, err)
			byPath := outputsByPath(t, outs, dest)
			assert.Contains(t, byPath, "SKILL.md")
			assert.Contains(t, byPath, "references/guide.md")
			for rel, want := range tc.wantFiles {
				out, ok := byPath[rel]
				require.True(t, ok, "expected %s in the bundle, got %v", rel, byPath)
				assert.Equal(t, want, string(out.RawContent))
			}
			if runtime.GOOS != "windows" {
				for _, rel := range tc.wantExec {
					assert.NotZero(t, byPath[rel].Mode&0o111, "%s lost its executable bit", rel)
				}
			}
			assert.Len(t, byPath, 2+len(tc.wantFiles), "nothing beyond the expected files is bundled")
			assert.Equal(t, tc.wantWarn, strings.Contains(rec.String(), link), rec.String())
			for _, out := range outs {
				assert.NotContains(t, string(out.RawContent), "TOP SECRET")
			}
		})
	}
}

func writeModeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
}

// TestPassthroughSkillDirectoryLinkSkipsCredentials pins that a directory link
// admitted by the policy does not smuggle in the credential files and .git/
// entries behind it, whether the link names a subdirectory or the project root.
func TestPassthroughSkillDirectoryLinkSkipsCredentials(t *testing.T) {
	tests := []struct {
		name   string
		target func(project string) string
		want   []string // bundle-relative paths expected besides SKILL.md
	}{
		{
			name:   "subdirectory with a .env, a nested key and a .git directory",
			target: func(project string) string { return filepath.Join(project, "docs") },
			want:   []string{"references/ref/a.md"},
		},
		{
			name:   "project root",
			target: func(project string) string { return project },
			want:   []string{"references/ref/README.md", "references/ref/docs/a.md"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			skillDir := filepath.Join(project, ".ai-rulez", "skills", "analyze")
			writeModeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: analyze\n---\n", 0o644)
			writeModeFile(t, filepath.Join(project, "README.md"), "# readme\n", 0o644)
			writeModeFile(t, filepath.Join(project, ".env"), "TOKEN=TOP SECRET\n", 0o644)
			writeModeFile(t, filepath.Join(project, ".npmrc"), "TOP SECRET\n", 0o644)
			writeModeFile(t, filepath.Join(project, ".git", "config"), "TOP SECRET\n", 0o644)
			writeModeFile(t, filepath.Join(project, "docs", "a.md"), "# a\n", 0o644)
			writeModeFile(t, filepath.Join(project, "docs", ".env"), "TOKEN=TOP SECRET\n", 0o644)
			writeModeFile(t, filepath.Join(project, "docs", "sub", "id_rsa"), "TOP SECRET\n", 0o600)
			writeModeFile(t, filepath.Join(project, "docs", ".git", "config"), "TOP SECRET\n", 0o644)
			require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "references"), 0o755))
			testutil.SymlinkOrSkip(t, tc.target(project), filepath.Join(skillDir, "references", "ref"))
			m := &Manifest{Name: "p", SourceDir: project,
				Config: &config.Config{BaseDir: project, Diag: diag.New(nil), Host: ambient.Host{Log: &testutil.LogRecorder{}}}}
			content := &config.ContentFile{Name: "analyze", Path: filepath.Join(skillDir, "SKILL.md")}
			dest := filepath.Join(project, "out", "analyze")

			// Act
			outs, err := passthroughSkill(m, content, dest, false)

			// Assert
			require.NoError(t, err)
			byPath := outputsByPath(t, outs, dest)
			for _, rel := range tc.want {
				assert.Contains(t, byPath, rel)
			}
			for _, out := range outs {
				assert.NotContains(t, string(out.RawContent), "TOP SECRET", out.Path)
			}
		})
	}
}
