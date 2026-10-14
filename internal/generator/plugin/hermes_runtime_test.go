package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/require"
)

func TestHermesNativeRuntimeDiscoversPluginAndRegisteredSkills(t *testing.T) {
	python := os.Getenv("AI_RULEZ_HERMES_PYTHON")
	if python == "" {
		t.Skip("set AI_RULEZ_HERMES_PYTHON to a Python interpreter with Hermes Agent installed")
	}
	root := t.TempDir()
	writeFile(t, root, hermesSourcePath, `from pathlib import Path

def register(ctx):
    ctx.register_skill("example", Path(__file__).parent / "skills" / "example" / "SKILL.md")
`)
	skill := writePiSkill(t, root, "example", "Native Hermes skill body.")
	m := &Manifest{Name: "example", Version: "1.2.3", SourceDir: root, Skills: []config.ContentFile{skill}}
	outputs, err := renderHermes(m, root)
	require.NoError(t, err)
	writeBundle(t, outputs, root)
	home := t.TempDir()
	writeFile(t, home, "config.yaml", "plugins:\n  enabled: [example]\n")
	cmd := exec.CommandContext(t.Context(), python, "-c", hermesNativeDiscoveryProbe)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HERMES_HOME="+home, "HERMES_ENABLE_PROJECT_PLUGINS=true",
		"HERMES_BUNDLED_PLUGINS="+filepath.Join(home, "bundled"), "HERMES_SAFE_MODE=0")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "loaded=1 skills=1 without_manifest=0")
}

const hermesNativeDiscoveryProbe = `from pathlib import Path
from hermes_cli.plugins import PluginManager

manager = PluginManager()
manager.discover_and_load()
plugins = [p for p in manager.list_plugins() if p["name"] == "example"]
assert len(plugins) == 1, plugins
assert plugins[0]["enabled"] is True, plugins
assert plugins[0]["error"] is None, plugins
assert manager.list_plugin_skills("example") == ["example"]
skill = manager.find_plugin_skill("example:example")
assert skill is not None
assert "Native Hermes skill body." in skill.read_text()

Path(".hermes/plugins/example/plugin.yaml").unlink()
manager.discover_and_load(force=True)
assert [p for p in manager.list_plugins() if p["name"] == "example"] == []
assert manager.list_plugin_skills("example") == []
print("loaded=1 skills=1 without_manifest=0")
`
