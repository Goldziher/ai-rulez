package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mcpServerS1 = "\n[[mcp_servers]]\nname = \"s1\"\ncommand = \"uvx\"\nargs = [\"one\"]\n"
	mcpServerS2 = "\n[[mcp_servers]]\nname = \"s2\"\ncommand = \"uvx\"\nargs = [\"two\"]\n"

	// userSettingsWithServer is a hand-written settings document whose server
	// shares its name with the one the config declares, with another value.
	userSettingsWithServer = `{
    "permissions": {
        "allow": ["Bash(git status:*)"]
    },
    "mcpServers": {
        "s1": {"command": "mine"}
    }
}
`
	// userSettingsOwnServer is a hand-written settings document whose server is not
	// in the config.
	userSettingsOwnServer = `{
    "permissions": {
        "allow": ["Bash(git status:*)"]
    },
    "mcpServers": {
        "mine": {"command": "mine"}
    }
}
`
)

// capturedMergeWarnings returns the recorder the merged-document warnings of a
// test go to, and the load option that routes the run's host logger there.
func capturedMergeWarnings() (*testutil.LogRecorder, config.LoadOption) {
	rec := &testutil.LogRecorder{}
	return rec, config.WithHost(ambient.Host{Log: rec})
}

func mcpServerNames(t *testing.T, doc string, path ...string) []string {
	t.Helper()
	var node any
	require.NoError(t, json.Unmarshal([]byte(doc), &node))
	for _, key := range path {
		obj, ok := node.(map[string]any)
		require.True(t, ok, "%s is not an object", key)
		node = obj[key]
	}
	obj, _ := node.(map[string]any)
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	return sortedStrings(names)
}

func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestGenerate_NeverTakesBackAHandWrittenServerWithoutARecord(t *testing.T) {
	// Arrange: the cursor preset does not write .claude/settings.json, so a server
	// there that shares a name with one in the config is the user's.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"cursor"}, "", mcpServerS1))
	writeAgentsMDFile(t, root, ".claude/settings.json", userSettingsWithServer)
	warned, host := capturedMergeWarnings()

	// Act
	runAgentsMDGenerate(t, root, host)
	runAgentsMDGenerate(t, root, host)
	_, err := NewGenerator(loadAgentsMDConfig(t, root, host)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, userSettingsWithServer, readAgentsMDFile(t, root, ".claude/settings.json"))
	assert.Empty(t, warned.Level("WARN"), "a document no manifest lists is the user's and is not even examined")
}

func TestClean_KeepsAHandWrittenServerEvenWhenItEqualsWhatClaudeWouldRender(t *testing.T) {
	// Arrange: the same server, with exactly the value the claude preset renders;
	// nothing shows ai-rulez wrote it, so it is the user's.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"cursor"}, "", mcpServerS1))
	writeAgentsMDFile(t, root, ".claude/settings.json", `{
    "permissions": {"allow": ["Bash"]},
    "mcpServers": {"s1": {"command": "uvx", "args": ["one"]}}
}
`)
	runAgentsMDGenerate(t, root)

	// Act
	_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.JSONEq(t, `{"permissions": {"allow": ["Bash"]}, "mcpServers": {"s1": {"command": "uvx", "args": ["one"]}}}`,
		readAgentsMDFile(t, root, ".claude/settings.json"))
}

func TestGenerate_HandWrittenServersSurviveAndCleanRestoresTheOriginal(t *testing.T) {
	tests := []struct {
		name, preset, path string
		servers            []string
	}{
		{"claude .mcp.json", "claude", ".mcp.json", nil},
		{"gemini settings", "gemini", ".gemini/settings.json", []string{"ai-rulez"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{tt.preset}, "", mcpServerS1+mcpServerS2))
			writeAgentsMDFile(t, root, tt.path, userSettingsOwnServer)

			// Act
			runAgentsMDGenerate(t, root)
			both := mcpServerNames(t, readAgentsMDFile(t, root, tt.path), "mcpServers")
			writeAgentsMDProject(t, root, agentsMDConfig([]string{tt.preset}, "", mcpServerS2))
			runAgentsMDGenerate(t, root)
			afterRemoval := mcpServerNames(t, readAgentsMDFile(t, root, tt.path), "mcpServers")
			_, err := NewGenerator(loadAgentsMDConfig(t, root)).Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, sortedStrings(append([]string{"mine", "s1", "s2"}, tt.servers...)), both)
			assert.Equal(t, sortedStrings(append([]string{"mine", "s2"}, tt.servers...)), afterRemoval)
			assert.Equal(t, userSettingsOwnServer, readAgentsMDFile(t, root, tt.path))
		})
	}
}

func TestClean_LeavesAServerTheUserEditedAndSaysSo(t *testing.T) {
	tests := []struct{ name, preset, path string }{
		{"claude .mcp.json", "claude", ".mcp.json"},
		{"gemini settings", "gemini", ".gemini/settings.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{tt.preset}, "", mcpServerS1))
			writeAgentsMDFile(t, root, tt.path, userSettingsOwnServer)
			runAgentsMDGenerate(t, root)
			edited := strings.Replace(readAgentsMDFile(t, root, tt.path), `"uvx"`, `"my-uvx"`, 1)
			writeAgentsMDFile(t, root, tt.path, edited)
			warned, host := capturedMergeWarnings()

			// Act
			_, err := NewGenerator(loadAgentsMDConfig(t, root, host)).Clean("", CleanOptions{})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, []string{"mine", "s1"}, withoutNames(mcpServerNames(t, readAgentsMDFile(t, root, tt.path), "mcpServers"), "ai-rulez"))
			assert.Equal(t, 1, countContaining(warned.Level("WARN"), "mcpServers.s1 in "+tt.path), "%v", warned.Level("WARN"))
		})
	}
}

func withoutNames(names []string, drop ...string) []string {
	var kept []string
	for _, name := range names {
		skip := false
		for _, d := range drop {
			skip = skip || name == d
		}
		if !skip {
			kept = append(kept, name)
		}
	}
	return kept
}

func TestGenerate_RemovedServerLeavesAGeneratedDocumentThatItOwnsWhole(t *testing.T) {
	// Arrange: .mcp.json written by the cursor preset alone, then a server is dropped.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"cursor"}, "", mcpServerS1+mcpServerS2))
	runAgentsMDGenerate(t, root)
	require.Equal(t, []string{"s1", "s2"}, mcpServerNames(t, readAgentsMDFile(t, root, ".mcp.json"), "mcpServers"))

	// Act
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"cursor"}, "", mcpServerS2))
	runAgentsMDGenerate(t, root)

	// Assert
	assert.Equal(t, []string{"s2"}, mcpServerNames(t, readAgentsMDFile(t, root, ".mcp.json"), "mcpServers"))
}

func TestGenerate_ServerOfTheSameNameIsOverwrittenByTheConfig(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", mcpServerS1))
	writeAgentsMDFile(t, root, ".mcp.json", userSettingsWithServer)

	// Act
	runAgentsMDGenerate(t, root)

	// Assert
	var doc struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(readAgentsMDFile(t, root, ".mcp.json")), &doc))
	assert.Equal(t, "uvx", doc.MCPServers["s1"].Command, "the config wins on a name clash")
}

func TestGuardClaims(t *testing.T) {
	path := []string{"mcpServers", "s1"}
	tests := []struct {
		name    string
		claims  []jsonmerge.Claim
		current []jsonmerge.Claim
		want    []jsonmerge.Claim
	}{
		{
			name:    "an unguarded claim takes the value the current config renders",
			claims:  []jsonmerge.Claim{{Path: path}},
			current: []jsonmerge.Claim{{Path: path, Sum: "abc"}},
			want:    []jsonmerge.Claim{{Path: path, Sum: "abc"}},
		},
		{
			name:   "an unguarded claim with nothing rendered now is dropped",
			claims: []jsonmerge.Claim{{Path: path}},
			want:   []jsonmerge.Claim{},
		},
		{
			name:    "a guarded claim keeps its own value",
			claims:  []jsonmerge.Claim{{Path: path, Sum: "old"}},
			current: []jsonmerge.Claim{{Path: path, Sum: "new"}},
			want:    []jsonmerge.Claim{{Path: path, Sum: "old"}},
		},
		{
			name:   "an element claim needs no guard",
			claims: []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"a"}}},
			want:   []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"a"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := guardClaims(tt.claims, tt.current)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerate_CommentedXumDocumentKeepsItsCommentsWhenTheServersAreDropped(t *testing.T) {
	// Arrange: the servers were merged, the user added a comment, then the config dropped them.
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"xum"}, "", mcpServerS1))
	runAgentsMDGenerate(t, root)
	commented := "// my notes\n" + readAgentsMDFile(t, root, ".xum/mcp.jsonc")
	writeAgentsMDFile(t, root, ".xum/mcp.jsonc", commented)
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"xum"}, "", ""))
	warned, host := capturedMergeWarnings()

	// Act
	runAgentsMDGenerate(t, root, host)

	// Assert
	got := readAgentsMDFile(t, root, ".xum/mcp.jsonc")
	assert.Contains(t, got, "// my notes", "the user's comment survives the unmerge")
	assert.NotContains(t, got, "s1", "the dropped server is taken back out of the commented file")
	assert.Empty(t, warned.Level("WARN"), "a commented document is no longer a problem")
}

func TestReadManifest_ReadsEachManifestOncePerRun(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", ""))
	g := NewGenerator(loadAgentsMDConfig(t, root))
	path := g.manifestPath()
	writeAgentsMDFile(t, root, ".ai-rulez/.generated-manifest.json", "not json")

	// Act
	first := g.readManifest(path)
	writeAgentsMDFile(t, root, ".ai-rulez/.generated-manifest.json", `{"version":"1","files":["a"]}`)
	second := g.readManifest(path)
	g.beginRun()
	third := g.readManifest(path)

	// Assert
	assert.Empty(t, first.Files)
	assert.Empty(t, second.Files, "the first read of the run is the one every consumer sees")
	assert.Equal(t, []string{"a"}, third.Files)
}

func TestWriteFileAtomic_ReplacesTheFileAndKeepsItsMode(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	// Act
	err := writeFileAtomic(path, []byte("new"))

	// Assert
	require.NoError(t, err)
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "new", string(got))
	info, statErr := os.Stat(path)
	require.NoError(t, statErr)
	if runtime.GOOS != "windows" { // Windows has no Unix permission bits
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	entries, dirErr := os.ReadDir(filepath.Dir(path))
	require.NoError(t, dirErr)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}
