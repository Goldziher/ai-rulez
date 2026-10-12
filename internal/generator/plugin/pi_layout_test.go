package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	}{
		{name: "generate replaces the file", mode: "", wantAuthor: true},
		{name: "merge keeps unknown keys", mode: config.PiPackageJSONMerge, preserved: []string{"devDependencies", "scripts", "packageManager"}, wantAuthor: true},
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
			assert.Equal(t, "@acme/pi-basemind", pkg["name"])
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
