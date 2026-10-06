package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const userClaudeSettings = `{
  "permissions": {
    "allow": [
      "Bash(git status:*)"
    ]
  }
}
`

// mergedProject generates a claude project whose hand-written settings.json has an
// overlay-only http server merged into it, then deletes the overlay.
func mergedProject(t *testing.T) *driftProject {
	t.Helper()
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.writeFile(t, ".mcp.json", userClaudeSettings)
	p.overlay(t, overlayWithHTTPSecret)
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.Contains(t, p.read(t, ".mcp.json"), "HEADER-SECRET-1", "the overlay server is merged in")
	require.NoError(t, os.Remove(filepath.Join(p.dir, "config.local.toml")))
	return p
}

func TestClean_RemovesOwnedKeysFromHandAuthoredSettings(t *testing.T) {
	// Arrange
	p := mergedProject(t)

	// Act
	plan, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, userClaudeSettings, p.read(t, ".mcp.json"))
	assert.NotEmpty(t, plan.Unmerged)
}

func TestClean_DryRunLeavesMergedDocumentsAlone(t *testing.T) {
	// Arrange
	p := mergedProject(t)
	before := p.read(t, ".mcp.json")

	// Act
	plan, err := NewGenerator(p.load(t)).Clean("", CleanOptions{DryRun: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, before, p.read(t, ".mcp.json"))
	assert.Contains(t, plan.Unmerged, filepath.Join(p.base, ".mcp.json"))
}

func TestGenerate_StripsRemovedServerFromHandAuthoredSettings(t *testing.T) {
	// Arrange
	p := mergedProject(t)

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.Equal(t, userClaudeSettings, p.read(t, ".mcp.json"))
}

func TestGenerate_StripsPresetKeysWhenThePresetIsRemoved(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini", "claude"}, "", ""))
	writeAgentsMDFile(t, root, ".gemini/settings.json", `{"theme":"dark"}`+"\n")
	runAgentsMDGenerate(t, root)
	require.Contains(t, readAgentsMDFile(t, root, ".gemini/settings.json"), "GEMINI.local.md")

	// Act
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", ""))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.JSONEq(t, `{"theme":"dark"}`, readAgentsMDFile(t, root, ".gemini/settings.json"))
}

func TestGenerate_DeletesMergedDocumentWhenNothingUserAuthoredRemains(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini", "claude"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, ".gemini", "settings.json"))

	// Act
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", ""))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.NoFileExists(t, filepath.Join(root, ".gemini", "settings.json"))
}

func TestClean_FallsBackToKnownValuesWithoutARecord(t *testing.T) {
	// Arrange: a document merged by 4.23.0, which kept no record of its keys.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "agents_md = true\n", ""))
	writeAgentsMDFile(t, root, ".gemini/settings.json",
		`{"theme":"dark","context":{"fileName":["AGENTS.md"]},"mcpServers":{"ai-rulez":{"command":"npx","args":["-y","ai-rulez@latest","mcp"]},"mine":{"command":"x"}}}`)
	cfg := loadAgentsMDConfig(t, root)

	// Act
	_, err := NewGenerator(cfg).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(readAgentsMDFile(t, root, ".gemini/settings.json")), &doc))
	assert.NotContains(t, doc, "context", "an exact ai-rulez context.fileName value is removed")
	assert.Contains(t, string(doc["mcpServers"]), "mine", "the user's server stays")
	assert.NotContains(t, string(doc["mcpServers"]), "ai-rulez", "the identical self-registration is removed")
	assert.Contains(t, doc, "theme")
}

func loadAgentsMDConfig(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

func TestGemini_UserStringBecomesListAndCleanTakesOurNameBack(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", ""))
	writeAgentsMDFile(t, root, ".gemini/settings.json", `{"theme":"dark","context":{"fileName":"MY.md"}}`+"\n")
	runAgentsMDGenerate(t, root)
	require.Equal(t, []string{"MY.md", "GEMINI.local.md"}, geminiContextNames(t, root))

	// Act
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.JSONEq(t, `{"theme":"dark","context":{"fileName":["MY.md"]}}`, readAgentsMDFile(t, root, ".gemini/settings.json"))
}

func TestGemini_AgentsMDToggleMovesOnlyTheNamesWeAdded(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDFile(t, root, ".gemini/settings.json", `{"context":{"fileName":["MY.md"]}}`+"\n")
	steps := []struct {
		flag string
		want []string
	}{
		{"agents_md = true\n", []string{"MY.md", "AGENTS.md", "GEMINI.local.md"}},
		{"", []string{"MY.md", "GEMINI.local.md"}},
		{"agents_md = true\n", []string{"MY.md", "GEMINI.local.md", "AGENTS.md"}},
	}
	for _, step := range steps {
		// Act
		writeAgentsMDProject(t, root, step.flag+agentsMDConfig([]string{"gemini"}, "", ""))
		runAgentsMDGenerate(t, root)

		// Assert
		assert.Equal(t, step.want, geminiContextNames(t, root), "agents_md flag %q", step.flag)
	}
}

func TestGemini_UserAgentsMDEntryIsNotClaimed(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"gemini"}, "", ""))
	writeAgentsMDFile(t, root, ".gemini/settings.json", `{"context":{"fileName":["AGENTS.md","MY.md"]}}`+"\n")
	runAgentsMDGenerate(t, root)

	// Act
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", ""))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.Equal(t, []string{"AGENTS.md", "MY.md", "GEMINI.local.md"}, geminiContextNames(t, root),
		"an AGENTS.md the user wrote is theirs")
}

func TestOpencode_HandAuthoredDocumentKeepsItsShapeAndCleanRestoresIt(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"opencode"}, "", ""))
	writeAgentsMDFile(t, root, "opencode.json", `{"model":"x","instructions":["docs/a.md"]}`+"\n")

	// Act
	runAgentsMDGenerate(t, root)
	generated := readAgentsMDFile(t, root, "opencode.json")
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, generated, "$schema", "a hand-authored document gets no $schema")
	assert.JSONEq(t, `{"model":"x","instructions":["docs/a.md","AGENTS.local.md"]}`, generated)
	assert.JSONEq(t, `{"model":"x","instructions":["docs/a.md"]}`, readAgentsMDFile(t, root, "opencode.json"))
}

func TestOpencode_DocumentWeCreatedIsDeletedByClean(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"opencode"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.Contains(t, readAgentsMDFile(t, root, "opencode.json"), "$schema")
	runAgentsMDGenerate(t, root)
	require.Contains(t, readAgentsMDFile(t, root, "opencode.json"), "$schema", "kept on the next run")

	// Act
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, "opencode.json"))
}

func TestGenerate_CommentedConfigsKeepTheirCommentsAndGetTheLocalEntry(t *testing.T) {
	tests := []struct {
		name, path, doc string
		preset          string
		comment         string
		want            string
	}{
		{"opencode.json", "opencode.json", "{\n  // my model\n  \"model\": \"x\",\n}\n", "opencode", "// my model", "AGENTS.local.md"},
		{"gemini settings", ".gemini/settings.json", "{\n  /* theme */\n  \"theme\": \"dark\"\n}\n", "gemini", "/* theme */", "GEMINI.local.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			warned := captureWarnings(t)
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{tt.preset}, "", ""))
			writeAgentsMDFile(t, root, tt.path, tt.doc)

			// Act
			runAgentsMDGenerate(t, root)
			merged := readAgentsMDFile(t, root, tt.path)
			_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			assert.Contains(t, merged, tt.comment, "the comment survives the merge")
			assert.Contains(t, merged, tt.want, "the local entry is written into the commented document")
			assert.Equal(t, tt.doc, readAgentsMDFile(t, root, tt.path), "clean restores the user's file byte for byte")
			assert.Empty(t, *warned)
		})
	}
}

func TestGenerate_CommentedConfigWithMCPServersIsMergedAndUnmerged(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"opencode"}, "", agentsMDMCPServer))
	doc := "{\n  // keep\n  \"model\": \"x\"\n}\n"
	writeAgentsMDFile(t, root, "opencode.json", doc)

	// Act
	err := NewGenerator(loadAgentsMDConfig(t, root)).Generate("")
	merged := readAgentsMDFile(t, root, "opencode.json")
	_, cleanErr := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, merged, "// keep")
	assert.Contains(t, merged, `"mcp"`)
	require.NoError(t, cleanErr)
	assert.Equal(t, doc, readAgentsMDFile(t, root, "opencode.json"), "clean restores the commented file")
}

func TestClean_DoesNotWarnAboutGeminiContext(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", ""))
	writeAgentsMDFile(t, root, ".gemini/settings.json", `{"context":{"fileName":["AGENTS.md","X.md"]}}`+"\n")
	warned := captureWarnings(t)

	// Act
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, *warned)
}

func TestGenerate_IgnoresTheLocalManifestThatHoldsMergeRecords(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.writeFile(t, ".mcp.json", userClaudeSettings)
	p.overlay(t, overlayWithHTTPSecret)

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.True(t, p.checkIgnored(t, ".ai-rulez/.generated-manifest.local.json"))
}

func TestGenerate_IgnoresTheLocalManifestWithoutAnyLocalInput(t *testing.T) {
	// Arrange: the record exists only because a hand-authored document was merged into.
	root := t.TempDir()
	cfg := "version = \"4.0\"\nname = \"shared\"\npresets = [\"claude\"]\n" + agentsMDMCPServer
	writeAgentsMDProject(t, root, cfg)
	writeAgentsMDFile(t, root, ".mcp.json", userClaudeSettings)
	p := &driftProject{base: root, dir: filepath.Join(root, ".ai-rulez")}
	p.git(t, "init", "-q")

	// Act
	runAgentsMDGenerate(t, root)

	// Assert
	require.FileExists(t, filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json"))
	assert.True(t, p.checkIgnored(t, ".ai-rulez/.generated-manifest.local.json"))
}

func TestWriteFileAtomic_WritesThroughSymlink(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	target := filepath.Join(dir, "real", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	link := filepath.Join(dir, "settings.json")
	testutil.SymlinkOrSkip(t, target, link)

	// Act
	require.NoError(t, writeFileAtomic(link, []byte("new")))

	// Assert
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link survives")
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	stat, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(), "existing mode is kept")
}

func TestIsNestedAgentsMD(t *testing.T) {
	assert.False(t, isNestedAgentsMD("AGENTS.md"))
	assert.True(t, isNestedAgentsMD("src/web/AGENTS.md"))
	assert.False(t, isNestedAgentsMD("src/web/README.md"))
}
