package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundleFilterExcluded(t *testing.T) {
	t.Parallel()
	f := newBundleFilter(gitutil.Git{}, nil, t.TempDir(), "SKILL.md", []string{"*.log", "scripts/build/", "assets/raw/*.psd"})

	tests := []struct {
		rel  string
		want bool
	}{
		{"references/api.md", false},
		{"scripts/run.sh", false},
		{"scripts/.venv/bin/python", true},
		{"scripts/.venv-tools/lib/x.py", true},
		{"scripts/venv/pyvenv.cfg", true},
		{"scripts/venvish/x.py", false},
		{"scripts/__pycache__/x.cpython-312.pyc", true},
		{"scripts/x.pyc", true},
		{"scripts/node_modules/left-pad/index.js", true},
		{"references/.git/config", true},
		{"references/debug.log", true},
		{"scripts/build/out.bin", true},
		{"scripts/buildtools/out.bin", false},
		{"assets/raw/a.psd", true},
		{"assets/raw/a.png", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, f.excluded(tt.rel))
		})
	}
}

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, rel := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x\n"), 0o644))
	}
}

func relPaths(resources []SkillResource) []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.RelPath)
	}
	sort.Strings(out)
	return out
}

func TestLoadResourcesSkipsBuildArtifacts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTree(t, dir,
		"SKILL.md",
		"references/api.md",
		"scripts/run.py",
		"scripts/.venv-x/lib/site.py",
		"scripts/.venv/pyvenv.cfg",
		"scripts/__pycache__/run.cpython-312.pyc",
		"scripts/stray.pyc",
		"scripts/node_modules/dep/index.js",
		"assets/logo.png",
		"assets/extra.tmp",
	)

	got, err := LoadResourcesWith(dir, ItemKindSkill, []string{"*.tmp"})
	require.NoError(t, err)
	assert.Equal(t, []string{"assets/logo.png", "references/api.md", "scripts/run.py"}, relPaths(got))

	// Without the extra pattern only the built-in list applies.
	got, err = LoadSkillResources(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"assets/extra.tmp", "assets/logo.png", "references/api.md", "scripts/run.py"}, relPaths(got))
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestLoadResourcesHonorsGitignore(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitInit(t, repo)
	root := filepath.Join(repo, ".ai-rulez", "skills", "demo")
	writeTree(t, root, "SKILL.md", "references/api.md", "scripts/run.py", "scripts/generated.json", "assets/cache/blob.bin")
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("generated.json\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("assets/cache/\n"), 0o644))

	got, err := LoadSkillResources(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"references/api.md", "scripts/run.py"}, relPaths(got))
}

func TestLoadResourcesKeepsEverythingWhenRootIsIgnored(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitInit(t, repo)
	root := filepath.Join(repo, ".ai-rulez", "skills", "demo")
	writeTree(t, root, "SKILL.md", "references/api.md", "scripts/.venv/pyvenv.cfg")
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".ai-rulez/\n"), 0o644))

	got, err := LoadSkillResources(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"references/api.md"}, relPaths(got))
}

func TestLoadResourcesCommandKindSkipsArtifacts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTree(t, dir, "COMMAND.md", "references/a.md", "scripts/__pycache__/a.pyc")
	got, err := LoadResources(dir, ItemKindCommand)
	require.NoError(t, err)
	assert.Equal(t, []string{"references/a.md"}, relPaths(got))
}

func TestBundleExcludeFromConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTree(t, dir,
		"config.toml",
		"skills/demo/SKILL.md",
		"skills/demo/references/a.md",
		"skills/demo/references/a.bak",
		"skills/demo/scripts/.venv-x/lib.py",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("version = \"4.0\"\nname = \"x\"\nbundle_exclude = [\"*.bak\"]\n"), 0o644))

	cfg, err := decodeConfigTOML([]byte("version = \"4.0\"\nname = \"x\"\nbundle_exclude = [\"*.bak\"]\ncodex_skills_dir = \".codex/skills\"\n"), "x")
	require.NoError(t, err)
	assert.Equal(t, []string{"*.bak"}, cfg.BundleExclude)
	assert.Equal(t, ".codex/skills", cfg.CodexSkillsDir)

	tree, err := ScanContentTreeWith(dir, cfg.BundleExclude)
	require.NoError(t, err)
	require.Len(t, tree.Skills, 1)
	assert.Equal(t, []string{"references/a.md"}, relPaths(tree.Skills[0].Resources))
}

func TestCodexSkillsDirOrDefault(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"nil config", nil, ".agents/skills"},
		{"unset", &Config{}, ".agents/skills"},
		{"legacy", &Config{CodexSkillsDir: ".codex/skills"}, ".codex/skills"},
		{"cleaned", &Config{CodexSkillsDir: ".codex\\skills/"}, ".codex/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.cfg.CodexSkillsDirOrDefault())
		})
	}
}

func TestValidateOutputSubdir(t *testing.T) {
	t.Parallel()
	for dir, ok := range map[string]bool{
		".agents/skills": true, ".codex/skills": true, "": false, ".": false,
		"/etc": false, "../x": false, "..": false, "C:/x": false,
	} {
		err := ValidateOutputSubdir("k", dir)
		assert.Equal(t, ok, err == nil, dir)
	}
}

func TestBundleFilterNegationReincludes(t *testing.T) {
	t.Parallel()
	f := newBundleFilter(gitutil.Git{}, nil, t.TempDir(), "SKILL.md", []string{"!references/venv", "!**/node_modules-fixtures"})
	assert.False(t, f.excluded("references/venv/notes.md"))
	assert.True(t, f.excluded("scripts/venv/pyvenv.cfg"), "other venv dirs stay excluded")
	assert.True(t, f.excluded("scripts/node_modules/x.js"))
}

func TestValidateOutputSubdirRejectsSourceTrees(t *testing.T) {
	t.Parallel()
	for dir, ok := range map[string]bool{
		".ai-rulez/skills": false, ".ai-rulez": false, ".config/ai-rulez/x": false, ".git/hooks": false,
		"a/../../esc": false, "a/b": true, ".ai-rulezish": true,
	} {
		assert.Equal(t, ok, ValidateOutputSubdir("k", dir) == nil, dir)
	}
}

func TestValidateOutputPathRejectsControlDirs(t *testing.T) {
	t.Parallel()
	for p, ok := range map[string]bool{
		".git/config": false, ".GIT/config": false, "a/.git/hooks/x": false, ".git./x": false, "GIT~1/config": false,
		".git": false, ".hg/hgrc": false, ".svn/x": false, ".ai-rulez/config.toml": false, ".AI-RULEZ/x": false,
		"a\\.git\\config": false, "x\x00y": false, "../escaped.md": false, "a/../../b": false, "/abs": false,
		"\\\\host\\share\\x": false, "C:\\x": false, "": false, ".": false,
		"docs/AI_GUIDE.md": true, ".github/copilot-instructions.md": true, ".gitignore": true, ".github/x": true,
		"a/.gitkeep": true, ".ai-rulezish/x": true,
	} {
		assert.Equal(t, ok, ValidateOutputPath("path", p) == nil, "%q", p)
	}
}

func TestCustomPresetPathIsValidated(t *testing.T) {
	t.Parallel()
	for _, p := range []string{".git/config", "../escaped.md", "/etc/passwd", ".ai-rulez/config.toml"} {
		cfg := &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{Name: "x", Type: PresetTypeMarkdown, Path: p, Template: "x"}},
		}
		err := cfg.Validate()
		require.Error(t, err, p)
		assert.Contains(t, err.Error(), "unsafe 'path'", p)
	}
}

func TestOKFDirRejectsControlDirs(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{".git/hooks", "a/.GIT/x", "/etc", "../x", ".hg/store"} {
		cfg := &Config{Version: "4.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}}, OKF: &OKFConfig{Dir: dir}}
		err := cfg.Validate()
		require.Error(t, err, dir)
		assert.Contains(t, err.Error(), "okf.dir", dir)
	}
}
