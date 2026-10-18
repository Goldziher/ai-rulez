package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePiSkill writes a skill's SKILL.md under root/.ai-rulez/skills/<name>/ and
// returns the content file the manifest carries.
func writePiSkill(t *testing.T, root, name, body string) config.ContentFile {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", "skills", name, skillManifestName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return config.ContentFile{Name: name, Path: path}
}

// hasOutputSuffix reports whether any output's path ends with suffix.
func hasOutputSuffix(outputs []config.OutputFile, suffix string) bool {
	for _, o := range outputs {
		if hasSuffix(filepath.ToSlash(o.Path), suffix) {
			return true
		}
	}
	return false
}

func TestRenderPiSkillsDir(t *testing.T) {
	tests := []struct {
		name       string
		skillsDir  string
		wantSkill  string
		wantPi     string
		missSecond string
	}{
		{
			name:      "default writes under .pi/skills",
			wantSkill: ".pi/skills/basemind/SKILL.md",
			wantPi:    "./.pi/skills/basemind",
		},
		{
			name:       "skills_dir reuses the shared tree",
			skillsDir:  "skills",
			wantSkill:  "skills/basemind/SKILL.md",
			wantPi:     "./skills/basemind",
			missSecond: ".pi/skills/basemind/SKILL.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			skill := writePiSkill(t, root, "basemind", "---\nname: basemind\n---\nbody\n")
			m := &Manifest{
				Name: "basemind", Version: "1.0.0", SourceDir: root,
				PiSkillsDir: tt.skillsDir, Skills: []config.ContentFile{skill},
			}

			// Act
			outputs, err := renderPi(m, t.TempDir())
			require.NoError(t, err)

			// Assert
			assert.NotEmpty(t, outputByPath(t, outputs, tt.wantSkill).RawContent)
			if tt.missSecond != "" {
				assert.False(t, hasOutputSuffix(outputs, tt.missSecond),
					"the skill must be written once, not copied under .pi/skills too")
			}
			var pkg map[string]any
			require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
			assert.Equal(t, []any{tt.wantPi}, pkg["pi"].(map[string]any)["skills"])
			assert.Contains(t, pkg["files"], tt.wantSkill)
		})
	}
}

func TestRenderPiPackageJSONMode(t *testing.T) {
	existing := `{"name":"authored","devDependencies":{"x":"1"},"scripts":{"t":"x"},"packageManager":"pnpm@9"}`
	tests := []struct {
		name       string
		mode       string
		preserved  []string
		wantAuthor bool
		wantName   string
	}{
		{name: "generate replaces the file", mode: "", wantAuthor: true, wantName: "@acme/pi-basemind"},
		{name: "merge keeps unknown keys", mode: config.PiPackageJSONMerge, preserved: []string{"devDependencies", "scripts", "packageManager"}, wantAuthor: true, wantName: "authored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(existing), 0o644))
			m := &Manifest{
				Name: "basemind", Version: "1.0.0", Description: "d", SourceDir: root,
				PiPackageJSON: tt.mode, Author: &config.Author{Name: "Jane"},
				Repository: "https://github.com/acme/basemind",
			}

			// Act
			outputs, err := renderPi(m, root)
			require.NoError(t, err)

			// Assert
			var pkg map[string]any
			require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
			assert.Equal(t, tt.wantName, pkg["name"])
			assert.Contains(t, pkg["keywords"], "pi-package")
			assert.Contains(t, pkg, "pi")
			for _, key := range tt.preserved {
				assert.Contains(t, pkg, key, "%s must be preserved in merge mode", key)
			}
			if tt.mode != config.PiPackageJSONMerge {
				assert.NotContains(t, pkg, "devDependencies", "generate mode replaces the file")
			}
			if tt.wantAuthor {
				assert.Equal(t, "Jane", pkg["author"].(map[string]any)["name"])
			}
		})
	}
}

// TestGeneratePiMergeVerifyChecksOwnedKeys covers the verify side of
// package_json = "merge": the sidecar hash covers the owned keys alone, so an
// author's edits to other keys do not read as drift while an owned key does.
func TestGeneratePiMergeVerifyChecksOwnedKeys(t *testing.T) {
	// Arrange
	root := t.TempDir()
	skill := writePiSkill(t, root, "basemind", "---\nname: basemind\n---\nbody\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"),
		[]byte(`{"name":"authored","devDependencies":{"x":"1"}}`), 0o644))
	m := &Manifest{
		Name: "basemind", Version: "1.0.0", Description: "d", SourceDir: root,
		PiPackageJSON: config.PiPackageJSONMerge, Skills: []config.ContentFile{skill},
		Runtimes: []string{config.PluginRuntimePi},
	}
	outputs, err := Generate(m, root)
	require.NoError(t, err)
	for _, out := range outputs {
		require.NoError(t, os.MkdirAll(filepath.Dir(out.Path), 0o755))
		require.NoError(t, os.WriteFile(out.Path, out.RawContent, 0o644))
	}

	// Act + Assert: a clean bundle verifies.
	require.NoError(t, VerifyProvenance(root))

	// An edit to a key ai-rulez does not own is not drift.
	pkgPath := filepath.Join(root, "package.json")
	generated, err := os.ReadFile(pkgPath)
	require.NoError(t, err)
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(generated, &pkg))
	pkg["devDependencies"] = map[string]any{"x": "2"}
	edited, err := json.Marshal(pkg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pkgPath, edited, 0o644))
	assert.NoError(t, VerifyProvenance(root), "an unowned key must not read as drift")

	// An edit to an owned key is drift.
	pkg["name"] = "tampered"
	tampered, err := json.Marshal(pkg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pkgPath, tampered, 0o644))
	assert.Error(t, VerifyProvenance(root), "an owned key must be detected by verify --plugin")
}

func TestRenderPiPackageName(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		existing string
		pkgName  string
		want     string
	}{
		{name: "default derives from the repository", want: "@acme/pi-basemind"},
		{name: "explicit name wins", pkgName: "ghost-library-skills", want: "ghost-library-skills"},
		{name: "merge keeps the authored name", mode: config.PiPackageJSONMerge,
			existing: `{"name":"authored"}`, want: "authored"},
		{name: "merge without authored name derives it", mode: config.PiPackageJSONMerge,
			existing: `{"private":true}`, want: "@acme/pi-basemind"},
		{name: "merge with explicit name overrides", mode: config.PiPackageJSONMerge,
			existing: `{"name":"authored"}`, pkgName: "chosen", want: "chosen"},
		{name: "generate ignores the authored name", existing: `{"name":"authored"}`, want: "@acme/pi-basemind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(tt.existing), 0o644))
			}
			m := &Manifest{
				Name: "basemind", Version: "1.0.0", SourceDir: root, PiPackageJSON: tt.mode,
				PiPackageName: tt.pkgName, Repository: "https://github.com/acme/basemind",
			}

			// Act
			outputs, err := renderPi(m, root)
			require.NoError(t, err)

			// Assert
			var pkg map[string]any
			require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
			assert.Equal(t, tt.want, pkg["name"])
		})
	}
}

func TestRenderPiSkillsEntryDirectory(t *testing.T) {
	tests := []struct {
		name      string
		entry     string
		skillsDir string
		wantPi    []any
		wantFiles []any
	}{
		{name: "each lists every skill", entry: config.PiSkillsEntryEach, skillsDir: "skills",
			wantPi:    []any{"./skills/alpha", "./skills/beta"},
			wantFiles: []any{".pi/prompts/go.md", "skills/alpha/SKILL.md", "skills/beta/SKILL.md"}},
		{name: "directory lists the tree once", entry: config.PiSkillsEntryDirectory, skillsDir: "skills",
			wantPi:    []any{"./skills"},
			wantFiles: []any{".pi/prompts/go.md", "skills/"}},
		{name: "directory under the default dir", entry: config.PiSkillsEntryDirectory,
			wantPi:    []any{"./.pi/skills"},
			wantFiles: []any{".pi/prompts/go.md", ".pi/skills/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			alpha := writePiSkill(t, root, "alpha", "---\nname: alpha\n---\nbody\n")
			beta := writePiSkill(t, root, "beta", "---\nname: beta\n---\nbody\n")
			cmdPath := filepath.Join(root, ".ai-rulez", "commands", "go.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(cmdPath), 0o755))
			require.NoError(t, os.WriteFile(cmdPath, []byte("---\ndescription: d\n---\nrun\n"), 0o644))
			m := &Manifest{
				Name: "basemind", Version: "1.0.0", SourceDir: root, PiSkillsDir: tt.skillsDir,
				PiSkillsEntry: tt.entry, Skills: []config.ContentFile{alpha, beta},
				Commands: []config.ContentFile{{Name: "go", Path: cmdPath}},
			}

			// Act
			outputs, err := renderPi(m, t.TempDir())
			require.NoError(t, err)

			// Assert
			var pkg map[string]any
			require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
			assert.Equal(t, tt.wantPi, pkg["pi"].(map[string]any)["skills"])
			assert.Equal(t, tt.wantFiles, pkg["files"], "files must list only paths that exist in the bundle")
		})
	}
}

func TestRenderPiFilesListExistingTopLevelPaths(t *testing.T) {
	// Arrange
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# r\n"), 0o644))
	skill := writePiSkill(t, root, "alpha", "---\nname: alpha\n---\nbody\n")
	m := &Manifest{Name: "basemind", Version: "1.0.0", SourceDir: root, Skills: []config.ContentFile{skill}}

	// Act
	outputs, err := renderPi(m, root)
	require.NoError(t, err)

	// Assert
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
	assert.Contains(t, pkg["files"], "README.md")
	assert.NotContains(t, pkg["files"], "assets/", "assets/ does not exist in the bundle")
}

func TestGeneratePiMergeShouldRefreshOpenCodeMetadata(t *testing.T) {
	for _, runtimes := range [][]string{{"pi", "opencode"}, {"opencode", "pi"}} {
		root := t.TempDir()
		existing := `{"name":"authored","type":"commonjs","main":"old.js","exports":{".":"./old.js"},` +
			`"dependencies":{"@opencode/plugin":"^2.0.20","authored":"1"},"scripts":{"test":"example"}}`
		require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(existing), 0o644))
		m := &Manifest{
			Name: "probe", Version: "1.0.0", SourceDir: root,
			Repository: "https://github.com/acme/probe", Runtimes: runtimes,
			PiPackageJSON: config.PiPackageJSONMerge, PiPackageName: "@acme/probe",
		}
		outputs, err := Generate(m, root)
		require.NoError(t, err)
		var pkg map[string]any
		require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &pkg))
		assert.Equal(t, "@acme/probe", pkg["name"])
		assert.Equal(t, ".opencode/plugins/probe.js", pkg["main"])
		assert.Equal(t, "module", pkg["type"])
		assert.Equal(t, "^2.0.26", pkg["dependencies"].(map[string]any)["@opencode/plugin"])
		assert.Equal(t, "1", pkg["dependencies"].(map[string]any)["authored"])
		assert.Equal(t, map[string]any{"test": "example"}, pkg["scripts"])
		for _, out := range outputs {
			require.NoError(t, os.MkdirAll(filepath.Dir(out.Path), 0o755))
			require.NoError(t, os.WriteFile(out.Path, out.RawContent, 0o644))
		}
		require.NoError(t, VerifyProvenance(root))
		pkg["main"] = "broken.js"
		data, err := json.Marshal(pkg)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), data, 0o644))
		require.Error(t, VerifyProvenance(root))
	}
}

func TestRebasePiPromptLinks(t *testing.T) {
	tests := []struct {
		name      string
		skillsDir string
		body      string
		want      string
	}{
		{name: "root skills tree is two levels up", skillsDir: "skills",
			body: "Load [add](../skills/add/SKILL.md) first.",
			want: "Load [add](../../skills/add/SKILL.md) first."},
		{name: "default dir keeps the link", skillsDir: ".pi/skills",
			body: "Load [add](../skills/add/SKILL.md) first.",
			want: "Load [add](../skills/add/SKILL.md) first."},
		{name: "custom skills dir", skillsDir: "packages/skills",
			body: "[a](../skills/a/SKILL.md)",
			want: "[a](../../packages/skills/a/SKILL.md)"},
		{name: "other root files follow the prompt depth", skillsDir: "skills",
			body: "See [readme](../README.md) and [docs](../docs/guide.md#top).",
			want: "See [readme](../../README.md) and [docs](../../docs/guide.md#top)."},
		{name: "fragment and title survive", skillsDir: "skills",
			body: `[a](../skills/a/SKILL.md#usage "the skill")`,
			want: `[a](../../skills/a/SKILL.md#usage "the skill")`},
		{name: "links that do not climb are untouched", skillsDir: "skills",
			body: "[a](references/x.md) [b](https://example.com/../x) [c](#anchor) [d](/abs/path)",
			want: "[a](references/x.md) [b](https://example.com/../x) [c](#anchor) [d](/abs/path)"},
		{name: "code spans and fences are left as written", skillsDir: "skills",
			body: "Write `[demo](../skills/demo/SKILL.md)` like this:\n\n```md\n[demo](../skills/demo/SKILL.md)\n```\n\nThen [demo](../skills/demo/SKILL.md).",
			want: "Write `[demo](../skills/demo/SKILL.md)` like this:\n\n```md\n[demo](../skills/demo/SKILL.md)\n```\n\nThen [demo](../../skills/demo/SKILL.md)."},
		{name: "angle-bracket destinations", skillsDir: "skills",
			body: "[demo](<../skills/demo/SKILL.md>)",
			want: "[demo](<../../skills/demo/SKILL.md>)"},
		{name: "reference definitions", skillsDir: "skills",
			body: "See [demo].\n\n[demo]: ../skills/demo/SKILL.md \"Demo\"\n[home]: <../README.md#top>\n",
			want: "See [demo].\n\n[demo]: ../../skills/demo/SKILL.md \"Demo\"\n[home]: <../../README.md#top>\n"},
		{name: "reference definitions inside a fence are left alone", skillsDir: "skills",
			body: "```\n[demo]: ../skills/demo/SKILL.md\n```\n",
			want: "```\n[demo]: ../skills/demo/SKILL.md\n```\n"},
		{name: "reserved characters in the skills dir are escaped", skillsDir: "packages/skills#v1",
			body: "[a](../skills/a/SKILL.md#usage)",
			want: "[a](../../packages/skills%23v1/a/SKILL.md#usage)"},
		{name: "spaces in the skills dir are escaped", skillsDir: "my skills",
			body: "[a](../skills/a/SKILL.md?x=1)",
			want: "[a](../../my%20skills/a/SKILL.md?x=1)"},
		{name: "a link leaving the bundle root is kept", skillsDir: "skills",
			body: "[a](../../outside.md)",
			want: "[a](../../outside.md)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, rebasePiPromptLinks(tt.body, tt.skillsDir))
		})
	}
}

func TestRenderPiPromptLinksResolveToTheSkills(t *testing.T) {
	tests := []struct {
		name      string
		skillsDir string
		skillPath string
	}{
		{name: "root skills tree", skillsDir: "skills", skillPath: "skills/alpha/SKILL.md"},
		{name: "default skills tree", skillPath: ".pi/skills/alpha/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			alpha := writePiSkill(t, root, "alpha", "---\nname: alpha\n---\nbody\n")
			cmdPath := filepath.Join(root, ".ai-rulez", "commands", "go.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(cmdPath), 0o755))
			require.NoError(t, os.WriteFile(cmdPath, []byte("---\ndescription: d\n---\nLoad [alpha](../skills/alpha/SKILL.md).\n"), 0o644))
			m := &Manifest{
				Name: "basemind", Version: "1.0.0", SourceDir: root, PiSkillsDir: tt.skillsDir,
				Skills: []config.ContentFile{alpha}, Commands: []config.ContentFile{{Name: "go", Path: cmdPath, Content: "Load [alpha](../skills/alpha/SKILL.md).\n"}},
			}
			dest := t.TempDir()

			// Act
			outputs, err := renderPi(m, dest)
			require.NoError(t, err)

			// Assert: the link, taken from where the prompt is written, reaches the skill.
			prompt := outputByPath(t, outputs, ".pi/prompts/go.md")
			link := regexp.MustCompile(`\]\(([^)]+)\)`).FindStringSubmatch(string(prompt.RawContent))
			require.Len(t, link, 2, "prompt: %s", prompt.RawContent)
			resolved := filepath.Join(filepath.Dir(prompt.Path), filepath.FromSlash(link[1]))
			want := filepath.Join(dest, filepath.FromSlash(tt.skillPath))
			assert.Equal(t, want, resolved)
			assert.True(t, hasOutputSuffix(outputs, tt.skillPath), "the skill is generated at %s", tt.skillPath)
		})
	}
}
