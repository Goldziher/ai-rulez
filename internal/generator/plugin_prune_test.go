package generator

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePluginPrunesRemovedSkills(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "rename"}[rename], func(t *testing.T) {
			dir := newDomainsProject(t, "")
			gen := loadDomainsProject(t, dir)
			require.NoError(t, gen.GeneratePlugin(""))
			old := filepath.Join(dir, "skills/core-s/SKILL.md")
			handwritten := filepath.Join(dir, "skills/manual/SKILL.md")
			writeDomainsFile(t, handwritten, "hand-written\n")
			source := filepath.Join(dir, ".ai-rulez/skills/core-s")
			if rename {
				require.NoError(t, os.Rename(source, filepath.Join(dir, ".ai-rulez/skills/renamed")))
			} else {
				require.NoError(t, os.RemoveAll(source))
			}
			gen = loadDomainsProject(t, dir)
			lines, err := gen.DryRunPlugin("")
			require.NoError(t, err)
			assert.Contains(t, strings.Join(lines, "\n"), "delete-stale: skills/core-s/SKILL.md")
			require.FileExists(t, old, "dry run must not delete")
			require.NoError(t, gen.GeneratePlugin(""))
			assert.NoFileExists(t, old)
			assert.NoDirExists(t, filepath.Dir(old))
			data, err := os.ReadFile(handwritten)
			require.NoError(t, err)
			assert.Equal(t, "hand-written\n", string(data))
			if rename {
				assert.FileExists(t, filepath.Join(dir, "skills/renamed/SKILL.md"))
			}
			require.NoError(t, gen.VerifyPlugin(""))
			require.NoError(t, gen.GeneratePlugin(""))
		})
	}
}

func TestGeneratePluginPreservesEditedObsoleteOutput(t *testing.T) {
	dir := newDomainsProject(t, "")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	sidecar := filepath.Join(dir, ".ai-rulez-generated.json")
	before, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	old := filepath.Join(dir, "skills/core-s/SKILL.md")
	writeDomainsFile(t, old, "edited by user\n")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez/skills/core-s")))
	gen = loadDomainsProject(t, dir)
	err = gen.GeneratePlugin("")
	require.ErrorContains(t, err, "modified obsolete plugin output")
	data, err := os.ReadFile(old)
	require.NoError(t, err)
	assert.Equal(t, "edited by user\n", string(data))
	after, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestGeneratePluginPrunesDomainBundleOutputs(t *testing.T) {
	dir := newDomainsProject(t, "\n[marketplace]\nname = \"demo\"\noutput_dir = \"mkt\"\n\n[marketplace.from_domains]\nenabled = true\nruntimes = [\"claude\"]\n")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	root := filepath.Join(dir, "mkt/plugins/teama")
	old := filepath.Join(root, "skills/a-s/SKILL.md")
	require.FileExists(t, old)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez/domains/teamA/skills/a-s")))
	gen = loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoFileExists(t, old)
	assert.FileExists(t, filepath.Join(root, "commands/do-it.md"))
	assert.FileExists(t, filepath.Join(dir, "mkt/plugins/teamb/skills/b-s/SKILL.md"))
	require.NoError(t, gen.VerifyPlugin(""))
}

func TestGeneratePluginResourceLayoutTransitions(t *testing.T) {
	for _, directoryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "file-to-directory", true: "directory-to-file"}[directoryFirst], func(t *testing.T) {
			dir := newDomainsProject(t, "")
			source := filepath.Join(dir, ".ai-rulez/skills/core-s/asset")
			if directoryFirst {
				writeDomainsFile(t, filepath.Join(source, "old.json"), "{}\n")
			} else {
				writeDomainsFile(t, source, "old\n")
			}
			gen := loadDomainsProject(t, dir)
			require.NoError(t, gen.GeneratePlugin(""))
			require.NoError(t, os.RemoveAll(source))
			if directoryFirst {
				writeDomainsFile(t, source, "new\n")
			} else {
				writeDomainsFile(t, filepath.Join(source, "new.json"), "{}\n")
			}
			gen = loadDomainsProject(t, dir)
			require.NoError(t, gen.GeneratePlugin(""))
			require.NoError(t, gen.VerifyPlugin(""))
			if directoryFirst {
				assert.FileExists(t, filepath.Join(dir, "skills/core-s/asset"))
			} else {
				assert.FileExists(t, filepath.Join(dir, "skills/core-s/asset/new.json"))
			}
		})
	}
}

func TestGeneratePluginInventoryNamedResource(t *testing.T) {
	dir := newDomainsProject(t, "")
	fixture := ".ai-rulez-generated.json"
	source := filepath.Join(dir, ".ai-rulez/skills/core-s/assets", fixture)
	writeDomainsFile(t, source, "{\"test-fixture\":true}\n")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	require.NoError(t, gen.GeneratePlugin(""))
	_, err := gen.DryRunPlugin("")
	require.NoError(t, err)
	target := filepath.Join(dir, "skills/core-s/assets", fixture)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "{\"test-fixture\":true}\n", string(data))
	require.NoError(t, os.Remove(source))
	gen = loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoFileExists(t, target)
	require.NoError(t, gen.VerifyPlugin(""))
}

func TestGeneratePluginRetriesPartialLayoutTransition(t *testing.T) {
	for _, directoryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "file-to-directory", true: "directory-to-file"}[directoryFirst], func(t *testing.T) {
			dir := newDomainsProject(t, "")
			source := filepath.Join(dir, ".ai-rulez/skills/core-s/asset")
			if directoryFirst {
				writeDomainsFile(t, filepath.Join(source, "old.json"), "{}\n")
			} else {
				writeDomainsFile(t, source, "old\n")
			}
			gen := loadDomainsProject(t, dir)
			require.NoError(t, gen.GeneratePlugin(""))
			sidecar := filepath.Join(dir, ".ai-rulez-generated.json")
			before, err := os.ReadFile(sidecar)
			require.NoError(t, err)
			require.NoError(t, os.RemoveAll(source))
			if directoryFirst {
				writeDomainsFile(t, source, "new\n")
			} else {
				writeDomainsFile(t, filepath.Join(source, "new.json"), "{}\n")
			}
			writeDomainsFile(t, filepath.Join(dir, ".ai-rulez/skills/core-s/zzblocked"), "new\n")
			blocker := filepath.Join(dir, "skills/core-s/zzblocked")
			require.NoError(t, os.Mkdir(blocker, 0o750))
			gen = loadDomainsProject(t, dir)
			require.Error(t, gen.GeneratePlugin(""))
			after, err := os.ReadFile(sidecar)
			require.NoError(t, err)
			assert.Equal(t, before, after, "failed generation must retain ownership inventory")
			require.NoError(t, os.Remove(blocker))
			require.NoError(t, gen.GeneratePlugin(""))
			require.NoError(t, gen.VerifyPlugin(""))
		})
	}
}

func TestGeneratePluginPreservesEditedRemovedDomainBundle(t *testing.T) {
	dir := newDomainsProject(t, staleTail)
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	bundle := filepath.Join(dir, "mkt/plugins/demo-teamb")
	edited := filepath.Join(bundle, "skills/b-s/SKILL.md")
	writeDomainsFile(t, edited, "edited\n")
	rootInventory := filepath.Join(dir, "mkt/.ai-rulez-generated.json")
	before, err := os.ReadFile(rootInventory)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez/domains/teamB")))
	gen = loadDomainsProject(t, dir)
	_, err = gen.DryRunPlugin("")
	require.ErrorContains(t, err, "modified obsolete plugin output")
	require.ErrorContains(t, gen.GeneratePlugin(""), "modified obsolete plugin output")
	data, err := os.ReadFile(edited)
	require.NoError(t, err)
	assert.Equal(t, "edited\n", string(data))
	after, err := os.ReadFile(rootInventory)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.FileExists(t, filepath.Join(bundle, ".ai-rulez-generated.json"))
	require.NoError(t, os.Remove(edited))
	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoDirExists(t, bundle)
	require.NoError(t, gen.VerifyPlugin(""))
}

func TestPluginInventoryWriteFailurePreservesPreviousBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".ai-rulez-generated.json")
	original := []byte("previous inventory\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	if probe, err := os.CreateTemp(dir, "probe"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("directory write permissions are not enforced for this user")
	}
	err := writeRawOutput(path, false, config.OutputFile{Path: path, RawContent: []byte("new inventory\n"), PluginInventory: true})
	require.ErrorContains(t, err, "commit plugin inventory")
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, actual, "failed atomic commit must leave previous inventory intact")
}
