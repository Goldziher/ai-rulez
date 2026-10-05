package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const genericSidecarSpec = `name = "doctool"

[[sidecars]]
kind = "mcp"
path = "seeded.json"

[[sidecars]]
kind = "mcp"
path = ".cfg/seeded.jsonc"

[[sidecars]]
kind = "mcp"
dialect = "codex"
path = ".codex-like/config.toml"

[[sidecars]]
kind = "mcp"
dialect = "yaml-standard"
path = ".poolside-like/mcp.yaml"

[[sidecars]]
kind = "mcp"
path = ".fresh/mcp.json"
`

const genericSidecarConfig = `version = "4.0"
name = "generic-sidecars"
gitignore = true
presets = [{ name = "doctool", provider = ".ai-rulez/providers/doctool.toml" }]

[[mcp_servers]]
name = "s1"
command = "uvx"
args = ["one"]
`

// genericSidecarSeeds are the hand-authored documents the sidecars merge into.
var genericSidecarSeeds = map[string]string{
	"seeded.json": "{\n  \"theme\": \"dark\",\n  \"mcpServers\": {\n    \"mine\": {\"command\": \"mine\"}\n  }\n}\n",
	".cfg/seeded.jsonc": "// my editor settings\n{\n  /* keep */ \"theme\": \"dark\", // trailing note\n" +
		"  \"mcpServers\": {\n    // mine\n    \"mine\": {\"command\": \"mine\"},\n  },\n}\n",
	".codex-like/config.toml": "# my codex config\nmodel = \"gpt-5\" # pinned\n\n[profiles.fast]\n# fast profile\neffort = \"low\"\n\n" +
		"[mcp_servers.mine]\ncommand = \"mine\"\n",
	".poolside-like/mcp.yaml": "# my agent config\nmodel: big # pinned\nmcp_servers:\n  # mine\n  mine:\n    command: mine\n",
}

func TestGenerate_GenericSidecarsMergeAndCleanRestoreEveryFormat(t *testing.T) {
	// Arrange
	root := t.TempDir()
	initGitRepo(t, root)
	writeAgentsMDProject(t, root, genericSidecarConfig)
	writeAgentsMDFile(t, root, ".ai-rulez/providers/doctool.toml", genericSidecarSpec)
	for rel, body := range genericSidecarSeeds {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	// Act
	runAgentsMDGenerate(t, root)

	// Assert: owned keys are in, comments and unowned members are kept.
	merged := map[string]string{}
	for rel := range genericSidecarSeeds {
		merged[rel] = readAgentsMDFile(t, root, rel)
		assert.Contains(t, merged[rel], "s1", rel)
		assert.Contains(t, merged[rel], "mine", rel)
	}
	assert.Contains(t, merged["seeded.json"], `"theme": "dark"`)
	for _, want := range []string{"// my editor settings", "/* keep */", "// trailing note", "// mine"} {
		assert.Contains(t, merged[".cfg/seeded.jsonc"], want)
	}
	for _, want := range []string{"# my codex config", "# pinned", "[profiles.fast]", "# fast profile", "[mcp_servers.s1]"} {
		assert.Contains(t, merged[".codex-like/config.toml"], want)
	}
	for _, want := range []string{"# my agent config", "# pinned", "# mine", "model: big"} {
		assert.Contains(t, merged[".poolside-like/mcp.yaml"], want)
	}
	assert.Contains(t, readAgentsMDFile(t, root, ".fresh/mcp.json"), "s1")

	gitignore := readAgentsMDFile(t, root, ".gitignore")
	assert.Contains(t, gitignore, ".fresh/mcp.json", "a document ai-rulez wrote whole is gitignored")
	for rel := range genericSidecarSeeds {
		assert.NotContains(t, gitignore, rel, "a document shared with the user is not gitignored")
	}

	// Act: clean.
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert: seeded files come back byte for byte; the fresh one is gone.
	require.NoError(t, err)
	for rel, body := range genericSidecarSeeds {
		assert.Equal(t, body, readAgentsMDFile(t, root, rel), "clean restores %s", rel)
	}
	assert.NoFileExists(t, filepath.Join(root, ".fresh", "mcp.json"))
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(out)))
}
