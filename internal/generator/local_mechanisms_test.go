package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// geminiContextNames reads context.fileName from the generated gemini settings.
func geminiContextNames(t *testing.T, root string) []string {
	t.Helper()
	var doc struct {
		Context struct {
			FileName []string `json:"fileName"`
		} `json:"context"`
	}
	require.NoError(t, json.Unmarshal([]byte(readAgentsMDFile(t, root, ".gemini/settings.json")), &doc))
	return doc.Context.FileName
}

// The gemini settings register GEMINI.local.md whenever the preset is on, with
// or without local content: the committed document must not depend on the machine.
func TestGeminiSettings_RegistersLocalContextFile(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		withLocal bool
		want      []string
	}{
		{name: "agents_md off, no local content", want: []string{"GEMINI.md", "GEMINI.local.md"}},
		{name: "agents_md off, local content", withLocal: true, want: []string{"GEMINI.md", "GEMINI.local.md"}},
		{name: "agents_md on, no local content", flag: "agents_md = true\n", want: []string{"AGENTS.md", "GEMINI.local.md"}},
		{name: "agents_md on, local content", flag: "agents_md = true\n", withLocal: true, want: []string{"AGENTS.md", "GEMINI.local.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsMDProject(t, root, tt.flag+agentsMDConfig([]string{"gemini"}, "", ""))
			if tt.withLocal {
				writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
			}

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			assert.Equal(t, tt.want, geminiContextNames(t, root))
			first := readAgentsMDFile(t, root, ".gemini/settings.json")
			runAgentsMDGenerate(t, root)
			assert.Equal(t, first, readAgentsMDFile(t, root, ".gemini/settings.json"), "idempotent")
		})
	}
}

func TestGeminiSettings_ToggleRewritesOwnedNames(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"gemini"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.Equal(t, []string{"AGENTS.md", "GEMINI.local.md"}, geminiContextNames(t, root))

	// Act
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", ""))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.Equal(t, []string{"GEMINI.md", "GEMINI.local.md"}, geminiContextNames(t, root))
}

func TestGeminiSettings_UserValueKeptAndWarned(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		existing string
		want     []string
		wantWarn bool
	}{
		{name: "off, user list without local warns", existing: `{"context":{"fileName":["CUSTOM.md","GEMINI.md"]}}`,
			want: []string{"CUSTOM.md", "GEMINI.md"}, wantWarn: true},
		{name: "off, user list with local is quiet", existing: `{"context":{"fileName":["GEMINI.md","GEMINI.local.md","X.md"]}}`,
			want: []string{"GEMINI.md", "GEMINI.local.md", "X.md"}},
		{name: "on, user list gets AGENTS.md only and warns", flag: "agents_md = true\n",
			existing: `{"context":{"fileName":["CUSTOM.md"]}}`, want: []string{"CUSTOM.md", "AGENTS.md"}, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			warned := captureWarnings(t)
			root := t.TempDir()
			writeAgentsMDProject(t, root, tt.flag+agentsMDConfig([]string{"gemini"}, "", ""))
			writeAgentsMDFile(t, root, ".gemini/settings.json", tt.existing)

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			assert.Equal(t, tt.want, geminiContextNames(t, root))
			assert.Equal(t, tt.wantWarn, countContaining(*warned, "GEMINI.local.md") > 0, *warned)
			assert.FileExists(t, root+"/.gemini/settings.json", "the user's document is never deleted")
		})
	}
}

// opencodeInstructions reads the instructions array of the generated opencode.json.
func opencodeInstructions(t *testing.T, root string) []string {
	t.Helper()
	var doc struct {
		Instructions []string `json:"instructions"`
	}
	require.NoError(t, json.Unmarshal([]byte(readAgentsMDFile(t, root, "opencode.json")), &doc))
	return doc.Instructions
}

// opencode.json is written for every opencode project, with or without MCP
// servers and local content, and is identical either way.
func TestOpencodeConfig_ListsLocalRootFile(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		withLocal bool
	}{
		{name: "no local content"},
		{name: "local content", withLocal: true},
		{name: "agents_md on, local content", flag: "agents_md = true\n", withLocal: true},
	}
	var documents []string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsMDProject(t, root, tt.flag+agentsMDConfig([]string{"opencode"}, "", ""))
			if tt.withLocal {
				writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
			}

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			assert.Equal(t, []string{"AGENTS.local.md"}, opencodeInstructions(t, root))
			documents = append(documents, readAgentsMDFile(t, root, "opencode.json"))
			if tt.withLocal {
				assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.local.md"), "LOCAL_BODY")
			}
		})
	}
	for _, doc := range documents[1:] {
		assert.Equal(t, documents[0], doc, "the committed document does not depend on local content")
	}
}

// captureLocalWarnings collects the dropped-local-content warnings as
// "preset: items" strings.
func captureLocalWarnings(t *testing.T) *[]string {
	t.Helper()
	var got []string
	previous := warnLocal
	warnLocal = func(_ string, args ...any) {
		fields := map[string]string{}
		for i := 0; i+1 < len(args); i += 2 {
			key, _ := args[i].(string)
			value, _ := args[i+1].(string)
			fields[key] = value
		}
		got = append(got, fields["preset"]+": "+fields["items"])
	}
	t.Cleanup(func() { warnLocal = previous })
	return &got
}

// Codex and Hermes (reading the AGENTS chain) load a machine-local
// AGENTS.override.md in place of AGENTS.md, so it carries the shared content too.
func TestAgentsOverride_ReplacesAgentsMDForCodexAndHermes(t *testing.T) {
	tests := []struct {
		name         string
		preset       string
		flag         string
		withLocal    bool
		wantOverride bool
	}{
		{name: "codex", preset: "codex", withLocal: true, wantOverride: true},
		{name: "codex with agents_md", preset: "codex", flag: "agents_md = true\n", withLocal: true, wantOverride: true},
		{name: "codex without local content", preset: "codex"},
		{name: "hermes with agents_md", preset: "hermes", flag: "agents_md = true\n", withLocal: true, wantOverride: true},
		{name: "hermes without agents_md", preset: "hermes", withLocal: true},
		{name: "opencode", preset: "opencode", withLocal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsMDProject(t, root, tt.flag+agentsMDConfig([]string{tt.preset}, "", ""))
			if tt.withLocal {
				writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
			}

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			override := filepath.Join(root, "AGENTS.override.md")
			assert.NoFileExists(t, filepath.Join(root, ".hermes.local.md"))
			if tt.preset != "opencode" {
				assert.NoFileExists(t, filepath.Join(root, "AGENTS.local.md"))
			}
			if !tt.wantOverride {
				assert.NoFileExists(t, override)
				return
			}
			got := readAgentsMDFile(t, root, "AGENTS.override.md")
			agents := readAgentsMDFile(t, root, "AGENTS.md")
			assert.Contains(t, got, "ALWAYS_BODY", "carries the shared content")
			assert.Contains(t, got, afterBanner(agents), "carries AGENTS.md as written")
			assert.Contains(t, got, "LOCAL_BODY")
			assert.NotContains(t, agents, "LOCAL_BODY")
			assert.Contains(t, got, "replaces AGENTS.md for Codex and Hermes")
			assert.Contains(t, manifestFiles(t, root, ".generated-manifest.local.json"), "AGENTS.override.md")
			assert.NotContains(t, sharedManifestFiles(t, root), "AGENTS.override.md")
		})
	}
}

func TestAgentsOverride_FollowsLocalContent(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, "AGENTS.override.md"))

	// Act: the local content goes away.
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".ai-rulez", "local")))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.NoFileExists(t, filepath.Join(root, "AGENTS.override.md"))
	assert.FileExists(t, filepath.Join(root, "AGENTS.md"))
}

// A tool with no local-file mechanism gets nothing written, and one warning
// naming it however many items it would have received.
func TestLocalRoot_WarnsOnceWhereNoMechanismExists(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		flag   string
		want   []string
	}{
		{name: "hermes without agents_md", preset: "hermes", want: []string{"hermes: rule r, context notes"}},
		{name: "amp", preset: "amp", want: []string{"amp: rule r, context notes"}},
		{name: "amp with agents_md", preset: "amp", flag: "agents_md = true\n", want: []string{"amp: rule r, context notes"}},
		{name: "hermes with agents_md has the override", preset: "hermes", flag: "agents_md = true\n"},
		{name: "codex has the override", preset: "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			warned := captureLocalWarnings(t)
			root := t.TempDir()
			writeAgentsMDProject(t, root, tt.flag+agentsMDConfig([]string{tt.preset}, "", ""))
			writeAgentsMDFile(t, root, ".ai-rulez/local/context/notes.md", "LOCAL_BODY\n")
			writeAgentsMDFile(t, root, ".ai-rulez/local/rules/r.md", "LOCAL_RULE\n")

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			assert.Equal(t, tt.want, *warned)
			for _, rel := range []string{".hermes.local.md", "AGENTS.local.md"} {
				assert.NoFileExists(t, filepath.Join(root, rel))
			}
		})
	}
}
