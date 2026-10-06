package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The committed manifest is attacker-editable and proves nothing: only an in-file
// Content-Hash, the machine-local manifest or the current rendering may license
// taking something back or deleting it.

const trustConfig = `version = "4.0"
name = "trust"
presets = ["claude"]
`

func trustClean(t *testing.T, root string, opts CleanOptions) *CleanPlan {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	plan, err := NewGenerator(cfg).Clean("default", opts)
	require.NoError(t, err)
	return plan
}

func writeCommittedManifest(t *testing.T, root string, files []string, merged map[string][]jsonmerge.Claim,
	digests map[string]string) {
	t.Helper()
	require.NoError(t, writeManifestFileDirs(filepath.Join(root, ".ai-rulez", generatedManifestName),
		files, merged, digests, nil))
}

func TestGenerate_ForgedCommittedClaimKeepsHandWrittenDeny(t *testing.T) {
	// Arrange: a hand-written deny rule and a committed manifest that claims it.
	const settings = "{\n  \"env\": {\"A\": \"1\"},\n  \"permissions\": {\"deny\": [\"Read(./.env)\"], \"allow\": [\"Bash(ls)\"]}\n}\n"
	root := writeProject(t, trustConfig, map[string]string{".claude/settings.json": settings})
	writeCommittedManifest(t, root, nil, map[string][]jsonmerge.Claim{
		".claude/settings.json": {
			{Path: []string{"permissions", "deny"}, Elements: []any{"Read(./.env)"}},
			{Path: []string{"permissions"}, Sum: jsonmerge.Digest(map[string]any{
				"deny": []any{"Read(./.env)"}, "allow": []any{"Bash(ls)"}})},
			{Path: []string{"env"}, Sum: jsonmerge.Digest(map[string]any{"A": "1"})},
		},
	}, nil)

	// Act
	generateProject(t, root)

	// Assert
	assert.Equal(t, settings, readProjectFile(t, root, ".claude/settings.json"))
}

func TestClean_KeepsHandWrittenSettingsEqualToTheRendering(t *testing.T) {
	// Arrange: the user already had exactly the deny rule the config renders.
	const settings = "{\n  \"permissions\": {\n    \"deny\": [\n      \"Read(./.env)\"\n    ]\n  }\n}\n"
	root := writeProject(t, trustConfig+"\n[permissions]\ndeny = [\"Read(./.env)\"]\n",
		map[string]string{".claude/settings.json": settings})
	generateProject(t, root)
	generateProject(t, root)

	// Act
	trustClean(t, root, CleanOptions{})

	// Assert
	assert.Equal(t, settings, readProjectFile(t, root, ".claude/settings.json"))
}

func TestClean_DoesNotFollowSymlinkedOutputDirectory(t *testing.T) {
	// Arrange: .cursor/commands is a link to a folder outside the project.
	root := writeProject(t, "version = \"4.0\"\nname = \"t\"\npresets = [\"cursor\"]\n",
		map[string]string{".ai-rulez/commands/ship.md": "---\ndescription: ship\n---\nShip it.\n"})
	generateProject(t, root)
	outside := t.TempDir()
	cmds := filepath.Join(root, ".cursor", "commands")
	entries, err := os.ReadDir(cmds)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		data, rerr := os.ReadFile(filepath.Join(cmds, e.Name()))
		require.NoError(t, rerr)
		require.NoError(t, os.WriteFile(filepath.Join(outside, e.Name()), data, 0o644))
	}
	require.NoError(t, os.RemoveAll(cmds))
	testutil.SymlinkOrSkip(t, outside, cmds)

	// Act
	trustClean(t, root, CleanOptions{})

	// Assert
	for _, e := range entries {
		assert.FileExists(t, filepath.Join(outside, e.Name()))
	}
}

func TestGenerate_StaleRemovalDoesNotFollowSymlinkedOutputDirectory(t *testing.T) {
	// Arrange
	root := writeProject(t, "version = \"4.0\"\nname = \"t\"\npresets = [\"cursor\"]\n", map[string]string{
		".ai-rulez/commands/ship.md": "---\ndescription: ship\n---\nShip it.\n",
		".ai-rulez/commands/old.md":  "---\ndescription: old\n---\nOld.\n",
	})
	generateProject(t, root)
	outside := t.TempDir()
	cmds := filepath.Join(root, ".cursor", "commands")
	entries, err := os.ReadDir(cmds)
	require.NoError(t, err)
	for _, e := range entries {
		data, rerr := os.ReadFile(filepath.Join(cmds, e.Name()))
		require.NoError(t, rerr)
		require.NoError(t, os.WriteFile(filepath.Join(outside, e.Name()), data, 0o644))
	}
	require.NoError(t, os.RemoveAll(cmds))
	testutil.SymlinkOrSkip(t, outside, cmds)
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "commands", "old.md")))

	// Act
	_ = generateProjectErr(t, root) // the write through the link is refused

	// Assert
	for _, e := range entries {
		assert.FileExists(t, filepath.Join(outside, e.Name()))
	}
}

func TestGenerate_ForgedDigestDoesNotDeleteHandWrittenFile(t *testing.T) {
	// Arrange: .clinerules is a path a preset could write, with no Content-Hash.
	const body = "my own cline rules\n"
	root := writeProject(t, trustConfig, map[string]string{".clinerules": body})
	sum := sha256.Sum256([]byte(body))
	writeCommittedManifest(t, root, []string{".clinerules"}, nil, map[string]string{".clinerules": hex.EncodeToString(sum[:])})

	// Act
	generateProject(t, root)
	trustClean(t, root, CleanOptions{})

	// Assert
	assert.Equal(t, body, readProjectFile(t, root, ".clinerules"))
}

func TestGenerate_RepairsHandEditedGeneratedFile(t *testing.T) {
	// Arrange
	root := writeProject(t, trustConfig, nil)
	generateProject(t, root)
	path := filepath.Join(root, "CLAUDE.md")
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	edited := strings.Replace(string(original), "API_NOTES_BODY", "HAND EDIT", 1)
	require.NotEqual(t, string(original), edited)
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o644))

	// Act
	generateProject(t, root)

	// Assert
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(got))
}

func TestClean_KeepsEditedGeneratedFileUnlessForced(t *testing.T) {
	// Arrange
	root := writeProject(t, trustConfig, nil)
	generateProject(t, root)
	path := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte("\nmy notes\n")...), 0o644))

	// Act
	plan := trustClean(t, root, CleanOptions{})

	// Assert
	assert.FileExists(t, path)
	assert.NotContains(t, plan.Files, path)

	// Act: forced
	plan = trustClean(t, root, CleanOptions{RemoveEdited: true})

	// Assert
	assert.NoFileExists(t, path)
	assert.Contains(t, plan.Files, path)
}

func TestGenerateThenClean_RestoresMergedDocumentsByteForByte(t *testing.T) {
	mcp := "\n[[mcp_servers]]\nname = \"docs\"\ncommand = \"docs-mcp\"\n"
	tests := []struct {
		name     string
		config   string
		file     string
		original string
	}{
		{
			name:     "one-line JSON without a final newline",
			config:   "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + mcp + "\n[permissions]\ndeny = [\"Read(./.env)\"]\n",
			file:     ".claude/settings.json",
			original: `{"model":"opus"}`,
		},
		{
			name:     "strict JSON with empty containers",
			config:   "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[permissions]\ndeny = [\"Read(./.env)\"]\n",
			file:     ".claude/settings.json",
			original: "{\"hooks\":{},\"permissions\":{\"deny\":[]}}\n",
		},
		{
			name:     "TOML with an empty mcp_servers header",
			config:   "version = \"4.0\"\nname = \"t\"\npresets = [\"codex\"]\n" + mcp,
			file:     ".codex/config.toml",
			original: "model = \"gpt\"\n\n[mcp_servers]\n",
		},
		{
			name:     "YAML with an empty mcp_servers map",
			config:   "version = \"4.0\"\nname = \"t\"\npresets = [\"poolside\"]\n" + mcp,
			file:     ".poolside/settings.yaml",
			original: "mcp_servers: {}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := writeProject(t, tt.config, map[string]string{tt.file: tt.original})

			// Act
			for range 3 {
				generateProject(t, root)
			}
			trustClean(t, root, CleanOptions{})

			// Assert
			assert.Equal(t, tt.original, readProjectFile(t, root, tt.file))
		})
	}
}

func TestGenerateUser_GuardHookStaysOutOfUserScope(t *testing.T) {
	// Arrange
	home, gen := newUserHome(t, "version = \"4.0\"\nname = \"me\"\npresets = [\"claude\", \"copilot\", \"copilot-cli\"]\n"+
		"\n[guard]\ngenerated = true\n\n[[hooks]]\nevent = \"Stop\"\n[[hooks.hooks]]\ncommand = \"echo done\"\n", userFixture())
	require.NoError(t, gen.config.Validate())

	// Act
	_, err := gen.GenerateUser("")

	// Assert
	require.NoError(t, err)
	for _, rel := range filesUnder(t, home, ".config") {
		data, rerr := os.ReadFile(filepath.Join(home, rel))
		require.NoError(t, rerr)
		if strings.HasSuffix(rel, ".md") {
			continue
		}
		assert.NotContains(t, string(data), "ai-rulez@", rel)
		assert.NotContains(t, string(data), " guard", rel)
	}
}

func TestGenerate_CopilotAndCopilotCLIShareTheGuardWithoutConflict(t *testing.T) {
	// Arrange
	root := writeProject(t, `version = "4.0"
name = "g"
presets = ["copilot", "copilot-cli"]

[guard]
generated = true
command = ["ai-rulez"]
`, nil)

	// Act
	generateProject(t, root)

	// Assert
	assert.Contains(t, readProjectFile(t, root, ".github/hooks/ai-rulez.json"), "ai-rulez guard")
}
