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

func dependencyManifest(t *testing.T, setting string) *Manifest {
	t.Helper()
	var authoring config.PluginAuthoring
	require.NoError(t, json.Unmarshal([]byte(`{"name":"probe","version":"1.0.0",`+
		`"opencode_dependency":"^2.1.4"`+setting+`}`), &authoring))
	m, err := BuildManifest(&config.Config{BaseDir: t.TempDir(), Plugin: &authoring}, nil)
	require.NoError(t, err)
	return m
}

func TestOpenCodeDependencyEnabledPreservesDefaultAndRange(t *testing.T) {
	for _, setting := range []string{"", `,"opencode_dependency_enabled":true`, `,"opencode_dependency_enabled":false`} {
		t.Run("setting="+setting, func(t *testing.T) {
			m := dependencyManifest(t, setting)
			outputs, err := renderOpenCode(m, t.TempDir())
			require.NoError(t, err)
			require.Len(t, outputs, 2)
			var pkg map[string]any
			require.NoError(t, json.Unmarshal(outputs[1].RawContent, &pkg))
			want := map[string]any{"@opencode/plugin": "^2.1.4"}
			if setting == `,"opencode_dependency_enabled":false` {
				want = map[string]any{}
			}
			assert.Equal(t, want, pkg["dependencies"])
		})
	}
}

func composeDependencyPackage(t *testing.T, m *Manifest, dir string, piFirst bool) map[string]any {
	t.Helper()
	openCode, err := renderOpenCode(m, dir)
	require.NoError(t, err)
	pi, err := renderPi(m, dir)
	require.NoError(t, err)
	outputs := append(openCode, pi...)
	if piFirst {
		outputs = append(pi, openCode...)
	}
	merged, err := composeNPMPackages(outputs, dir, true)
	require.NoError(t, err)
	packages := 0
	var pkg map[string]any
	for _, out := range merged {
		if out.Path == filepath.Join(dir, "package.json") {
			packages++
			require.NoError(t, json.Unmarshal(out.RawContent, &pkg))
		}
	}
	require.Equal(t, 1, packages)
	return pkg
}

func TestOpenCodeDependencyOmissionPreservesAuthoredDependencies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sdk, piFirst bool
	}{
		{"without SDK/OpenCode first", false, false},
		{"without SDK/Pi first", false, true},
		{"with SDK/OpenCode first", true, false},
		{"with SDK/Pi first", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := dependencyManifest(t, `,"opencode_dependency_enabled":false`)
			m.PiPackageJSON = config.PiPackageJSONMerge
			m.Runtimes = []string{config.PluginRuntimeOpenCode, config.PluginRuntimePi}
			dir := t.TempDir()
			dependencies := map[string]any{"authored": "1.2.3"}
			if tc.sdk {
				dependencies["@opencode/plugin"] = "^2.2.0"
			}
			pkg := npmOutput(t, dir, map[string]any{"name": "opencode-probe", "dependencies": dependencies})
			require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), pkg.RawContent, 0o644))
			got := composeDependencyPackage(t, m, dir, tc.piFirst)
			assert.Equal(t, dependencies, got["dependencies"])
			assert.Equal(t, ".opencode/plugins/probe.js", got["main"])
			assert.Contains(t, got, "pi")
		})
	}
}

func TestOpenCodeDefaultDependencyPreservesSDKImport(t *testing.T) {
	m := dependencyManifest(t, "")
	source := "import \"@opencode/plugin\";\nexport default { id: \"probe\", async setup() {} };\n"
	path := filepath.Join(m.SourceDir, openCodeSourcePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(source), 0o644))
	outputs, err := renderOpenCode(m, t.TempDir())
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	assert.Equal(t, source, string(outputs[0].RawContent))
	var pkg map[string]any
	require.NoError(t, json.Unmarshal(outputs[1].RawContent, &pkg))
	assert.Equal(t, map[string]any{"@opencode/plugin": "^2.1.4"}, pkg["dependencies"])
}
