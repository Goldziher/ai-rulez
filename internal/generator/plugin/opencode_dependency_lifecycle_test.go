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

func writeDependencyOutputs(t *testing.T, outputs []config.OutputFile) {
	t.Helper()
	require.NotEmpty(t, outputs)
	for _, out := range outputs {
		require.NoError(t, os.MkdirAll(filepath.Dir(out.Path), 0o755))
		require.NoError(t, os.WriteFile(out.Path, out.RawContent, 0o644))
	}
}

func existingDependencyPackage(t *testing.T, change string) (*Manifest, string, map[string]any) {
	t.Helper()
	m := dependencyManifest(t, "")
	m.PiPackageJSON = config.PiPackageJSONMerge
	m.Runtimes = []string{config.PluginRuntimePi, config.PluginRuntimeOpenCode}
	dir := t.TempDir()
	authored := npmOutput(t, dir, map[string]any{
		"name": "opencode-probe", "dependencies": map[string]any{"authored": "1.2.3"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), authored.RawContent, 0o644))
	first, err := Generate(m, dir)
	require.NoError(t, err)
	writeDependencyOutputs(t, first)
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(outputByPath(t, first, "package.json").RawContent, &pkg))
	assert.Equal(t, "^2.1.4", pkg["dependencies"].(map[string]any)["@opencode/plugin"])
	if change == "missing sidecar" {
		require.NoError(t, os.Remove(filepath.Join(dir, provenanceFileName)))
	} else if change != "unchanged" {
		switch change {
		case "SDK edited":
			pkg["dependencies"].(map[string]any)["@opencode/plugin"] = "^2.2.0"
		case "version edited":
			pkg["version"], m.Version = "2.0.0", "2.0.0"
		case "main edited":
			pkg["main"] = "authored.js"
		}
		data, err := json.Marshal(pkg)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), data, 0o644))
	}
	return m, dir, pkg
}

func TestGenerateOpenCodeDependencyDisableReconcilesOnlyOwnedUneditedSDK(t *testing.T) {
	for _, change := range []string{"unchanged", "SDK edited", "main edited", "version edited", "missing sidecar"} {
		t.Run(change, func(t *testing.T) {
			m, dir, pkg := existingDependencyPackage(t, change)
			enabled := false
			m.OpencodeDependencyEnabled = &enabled
			second, err := Generate(m, dir)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(outputByPath(t, second, "package.json").RawContent, &got))
			deps := got["dependencies"].(map[string]any)
			if change == "unchanged" {
				assert.Equal(t, map[string]any{"authored": "1.2.3"}, deps)
			} else {
				assert.Equal(t, pkg["dependencies"], deps)
			}
		})
	}
}

func TestGenerateOpenCodeDependencyOmissionRetainsAuthoredSDKAcrossRegeneration(t *testing.T) {
	m := dependencyManifest(t, `,"opencode_dependency_enabled":false`)
	m.PiPackageJSON = config.PiPackageJSONMerge
	m.Runtimes = []string{config.PluginRuntimeOpenCode, config.PluginRuntimePi}
	dir := t.TempDir()
	dependencies := map[string]any{"@opencode/plugin": "^2.2.0", "authored": "1.2.3"}
	pkg := npmOutput(t, dir, map[string]any{"name": "opencode-probe", "dependencies": dependencies})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), pkg.RawContent, 0o644))
	for range 2 {
		outputs, err := Generate(m, dir)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(outputByPath(t, outputs, "package.json").RawContent, &got))
		assert.Equal(t, dependencies, got["dependencies"])
		writeDependencyOutputs(t, outputs)
		require.NoError(t, VerifyProvenance(dir))
	}
}

func TestGenerateOpenCodeDependencyOmissionRejectsInvalidOwnership(t *testing.T) {
	for _, tc := range []struct{ data, message string }{
		{"{", "parse plugin provenance"},
		{`{"schema_version":"future"}`, "unsupported plugin provenance schema"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			m, dir, _ := existingDependencyPackage(t, "unchanged")
			require.NoError(t, os.WriteFile(filepath.Join(dir, provenanceFileName), []byte(tc.data), 0o644))
			enabled := false
			m.OpencodeDependencyEnabled = &enabled
			_, err := Generate(m, dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}
