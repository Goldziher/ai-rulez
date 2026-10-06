package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const permissionsProjectConfig = `version = "4.0"
name = "perms"
presets = ["claude", "codex", "cursor", "copilot", "gemini", "devin", "opencode", "kilo", "mimocode", "codebuddy",
  "commandcode", "qoder", "qwen", "letta", "grok", "vibe", "poolside", "omp", "augment", "zoocode", "copilot-cli", "zed"]
gitignore = false

[permissions]
allow = ["Bash(npm run test:*)", "Read(./src/**)", "WebFetch(domain:example.com)"]
ask = ["Bash(git push:*)"]
deny = ["Bash(rm -rf:*)", "Read(./.env)", "WebFetch(domain:evil.com)"]
`

// permissionDocs are the documents each harness's permissions land in, with the
// text that proves a rule of the configuration is there.
var permissionDocs = map[string][]string{
	".claude/settings.json":         {"Bash(npm run test:*)", "Bash(rm -rf:*)"},
	".codex/rules/ai-rulez.rules":   {`pattern = ["npm","run","test"]`, `decision = "forbidden"`},
	".cursor/cli.json":              {"Read(src/**)", "Read(.env)", "WebFetch(evil.com)"},
	".vscode/settings.json":         {"chat.tools.terminal.autoApprove", "zoo-code.deniedCommands"},
	".gemini/settings.json":         {"run_shell_command(npm run test )", "run_shell_command(rm -rf)"},
	".devin/config.json":            {"Exec(rm -rf)", "Fetch(domain:evil.com)"},
	"opencode.json":                 {`"rm -rf *": "deny"`, `".env": "deny"`},
	"kilo.jsonc":                    {`"rm -rf *": "deny"`, `"npm run test *": "allow"`},
	".mimocode/mimocode.jsonc":      {`"rm -rf *": "deny"`},
	".codebuddy/settings.json":      {"Bash(rm -rf:*)", "WebFetch(domain:evil.com)"},
	".commandcode/settings.json":    {"Shell(rm -rf:*)", "Read(/.env)"},
	".qoder/settings.json":          {"Bash(rm -rf:*)", "Read(/.env)"},
	".qwen/settings.json":           {"Bash(rm -rf:*)"},
	".letta/settings.json":          {"alwaysAsk", "Bash(rm -rf:*)"},
	".grok/config.toml":             {"[permission]", "Bash(rm -rf *)"},
	".vibe/config.toml":             {"denylist", "rm -rf"},
	".poolside/settings.yaml":       {"rm -rf *", "path: .env"},
	".omp/config.yml":               {"approval: deny", "match: rm -rf"},
	".augment/settings.json":        {"toolPermissions", `rm -rf([\\s;\u0026|)` + "`" + `]|$)`},
	".github/copilot/settings.json": {"deniedUrls", "evil.com"},
}

func TestGenerate_PermissionsReachEveryHarness(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, permissionsProjectConfig, nil)

	generateProject(t, root)

	for doc, wants := range permissionDocs {
		body := readProjectFile(t, root, doc)
		for _, want := range wants {
			assert.Contains(t, body, want, doc)
		}
	}
	assert.NoFileExists(t, filepath.Join(root, ".zed", "settings.json"), "Zed reads tool permissions from the user settings only")

	// generate --check is clean right after generate.
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	drift, err := NewGenerator(cfg).CheckDrift("default")
	require.NoError(t, err)
	assert.Empty(t, drift)
}

var handAuthoredPermissionDocs = map[string]string{
	".grok/config.toml":       "# my grok\nmodel = \"grok-4\"\n\n[permission]\n# keep\nallow = [\"Bash(make)\"]\n",
	".poolside/settings.yaml": "# my pool\ntools:\n  shell:\n    allow:\n      - make # keep\n",
	".vscode/settings.json":   "{\n  // my editor\n  \"editor.tabSize\": 2,\n  \"chat.tools.terminal.autoApprove\": {\n    \"make\": true // keep\n  }\n}\n",
	".qwen/settings.json":     "{\n  \"model\": \"qwen\",\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(make)\"\n    ]\n  }\n}\n",
	".devin/config.json":      "{\n  // devin\n  \"permissions\": {\"allow\": [\"Exec(make)\"]}\n}\n",
}

func TestGenerate_PermissionsKeepHandAuthoredRulesAndCleanRestoresThem(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, permissionsProjectConfig, handAuthoredPermissionDocs)

	gen := generateProject(t, root)

	for doc, original := range handAuthoredPermissionDocs {
		body := readProjectFile(t, root, doc)
		assert.NotEqual(t, original, body, doc)
		assert.Contains(t, body, "make", doc)
	}
	assert.Contains(t, readProjectFile(t, root, ".grok/config.toml"), "# keep")
	assert.Contains(t, readProjectFile(t, root, ".vscode/settings.json"), "// my editor")

	// A second run changes nothing.
	before := map[string]string{}
	for doc := range permissionDocs {
		before[doc] = readProjectFile(t, root, doc)
	}
	generateProject(t, root)
	for doc, body := range before {
		assert.Equal(t, body, readProjectFile(t, root, doc), "%s is stable across runs", doc)
	}

	// clean takes back what ai-rulez wrote: byte-identical to the hand-authored file.
	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	for doc, original := range handAuthoredPermissionDocs {
		assert.Equal(t, original, readProjectFile(t, root, doc), "%s after clean", doc)
	}
	assert.NoFileExists(t, filepath.Join(root, ".codex", "rules", "ai-rulez.rules"))
}

func TestGenerate_DroppingPermissionsRemovesTheirRules(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, permissionsProjectConfig, handAuthoredPermissionDocs)
	generateProject(t, root)
	require.Contains(t, readProjectFile(t, root, ".qwen/settings.json"), "Bash(rm -rf:*)")

	trimmed := "version = \"4.0\"\nname = \"perms\"\npresets = [\"qwen\", \"grok\", \"codex\"]\ngitignore = false\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte(trimmed), 0o644))
	generateProject(t, root)

	assert.Equal(t, handAuthoredPermissionDocs[".qwen/settings.json"], readProjectFile(t, root, ".qwen/settings.json"))
	assert.Equal(t, handAuthoredPermissionDocs[".grok/config.toml"], readProjectFile(t, root, ".grok/config.toml"))
	assert.NoFileExists(t, filepath.Join(root, ".codex", "rules", "ai-rulez.rules"), "no expressible rule left, so no rules file")
}

const userPermissionsConfig = `version = "4.0"
name = "me"
presets = ["claude", "codex", "gemini", "opencode", "zed", "qwen", "grok", "kilo", "hermes", "kimi"]

[permissions]
allow = ["Bash(npm run test:*)"]
deny = ["Bash(rm -rf:*)"]
`

func TestUser_PermissionsLandInTheUserFiles(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userPermissionsConfig, nil)

	_, err := gen.GenerateUser("")
	require.NoError(t, err)

	for rel, want := range map[string]string{
		".claude/settings.json":          "Bash(rm -rf:*)",
		".codex/rules/ai-rulez.rules":    `"forbidden"`,
		".gemini/settings.json":          "run_shell_command(rm -rf)",
		".config/opencode/opencode.json": `"rm -rf *": "deny"`,
		".config/zed/settings.json":      "always_deny",
		".qwen/settings.json":            "Bash(rm -rf:*)",
		".grok/config.toml":              "Bash(rm -rf *)",
		".config/kilo/kilo.jsonc":        `"rm -rf *": "deny"`,
		".hermes/config.yaml":            "rm -rf *",
		".kimi-code/config.toml":         "Bash(rm -rf *)",
	} {
		assert.Contains(t, readFileString(t, filepath.Join(home, filepath.FromSlash(rel))), want, rel)
	}
	assert.NoFileExists(t, filepath.Join(home, ".zed", "settings.json"))
}

func TestGenerate_UserOnlyHarnessesWriteNothingIntoTheProject(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, "version = \"4.0\"\nname = \"p\"\npresets = [\"zed\", \"hermes\", \"kimi\"]\ngitignore = false\n"+
		"[permissions]\ndeny = [\"Bash(rm -rf:*)\"]\n", nil)

	generateProject(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".zed", "settings.json"))
	assert.NoFileExists(t, filepath.Join(root, ".hermes", "config.yaml"))
	assert.NoFileExists(t, filepath.Join(root, ".kimi-code", "config.toml"))
}

const cursorDenyOnlyConfig = "version = \"4.0\"\nname = \"p\"\npresets = [\"cursor\"]\ngitignore = false\n\n[permissions]\ndeny = [\"Bash(rm -rf:*)\"]\n"

// TestGenerate_CursorDenyOnlyWritesBothRequiredArrays pins that Cursor's cli.json
// always carries permissions.allow and permissions.deny (both are required by its
// schema), and that ai-rulez owns the empty array it created: repeated runs and
// clean leave nothing behind.
func TestGenerate_CursorDenyOnlyWritesBothRequiredArrays(t *testing.T) {
	quietWarnings(t)
	// Arrange
	root := writeProject(t, cursorDenyOnlyConfig, nil)

	// Act
	generateProject(t, root)
	generateProject(t, root)
	gen := generateProject(t, root)
	body := readProjectFile(t, root, ".cursor/cli.json")
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	claims := gen.readManifest(gen.localManifestPath()).Merged[".cursor/cli.json"]
	_, err := gen.Clean("default", CleanOptions{RemoveEdited: true})
	require.NoError(t, err)

	// Assert
	assert.Contains(t, body, `"allow": []`)
	assert.Equal(t, []string{"Shell(rm:-rf*)"}, doc.Permissions.Deny)
	for _, claim := range claims {
		assert.Empty(t, claim.Preexisting, "%v: a created array is not recorded as the user's", claim.Path)
	}
	var allowClaim bool
	for _, claim := range claims {
		if slices.Equal(claim.Path, []string{"permissions", "allow"}) {
			allowClaim = true
			assert.True(t, claim.HasElements(), "the created empty array is claimed")
		}
	}
	assert.True(t, allowClaim, "the allow array is recorded in the local manifest")
	assert.NoFileExists(t, filepath.Join(root, ".cursor", "cli.json"), "a document ai-rulez created is removed")
}

// TestGenerate_CursorKeepsAHandWrittenEmptyAllow pins that an empty allow array the
// user wrote is theirs: clean restores the file byte for byte.
func TestGenerate_CursorKeepsAHandWrittenEmptyAllow(t *testing.T) {
	quietWarnings(t)
	// Arrange
	original := "{\n  \"permissions\": {\n    \"allow\": []\n  }\n}\n"
	root := writeProject(t, cursorDenyOnlyConfig, map[string]string{".cursor/cli.json": original})

	// Act
	generateProject(t, root)
	generateProject(t, root)
	gen := generateProject(t, root)
	body := readProjectFile(t, root, ".cursor/cli.json")
	_, err := gen.Clean("default", CleanOptions{RemoveEdited: true})
	require.NoError(t, err)

	// Assert
	assert.Contains(t, body, "Shell(rm:-rf*)")
	assert.Equal(t, original, readProjectFile(t, root, ".cursor/cli.json"))
}
