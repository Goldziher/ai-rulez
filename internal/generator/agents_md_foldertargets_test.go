package generator

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An always-on item whose targets match only a rules folder path (no AGENTS.md
// owner, no root alias) is not in the shared AGENTS.md. As in the flag-off
// output, the folder preset writes it as a rule file, exactly once.
func TestAgentsMD_AlwaysOnItemTargetedAtRulesFolderIsWrittenThere(t *testing.T) {
	cases := []struct {
		preset, target, file string
	}{
		{"cursor", ".cursor/rules/", ".cursor/rules/folder-only.mdc"},
		{"cursor", ".cursor/rules/folder-only.mdc", ".cursor/rules/folder-only.mdc"},
		{"cursor", ".cursor/rules/*.mdc", ".cursor/rules/folder-only.mdc"},
		{"claude", ".claude/rules/folder-only.md", ".claude/rules/folder-only.md"},
		{"claude", ".claude/rules/", ".claude/rules/folder-only.md"},
		{"cursor", ".cursor/", ".cursor/rules/folder-only.mdc"},
		{"cursor", "folder-only.mdc", ".cursor/rules/folder-only.mdc"},
		{"cline", ".clinerules/", ".clinerules/folder-only.md"},
		{"devin", ".devin/", ".devin/rules/folder-only.md"},
		{"copilot", ".github/instructions/*.md", ".github/instructions/folder-only.instructions.md"},
		{"copilot", ".github/instructions/", ".github/instructions/folder-only.instructions.md"},
		{"devin", ".devin/rules/", ".devin/rules/folder-only.md"},
	}
	for _, tc := range cases {
		for _, kind := range []string{"rules", "context"} {
			for _, variant := range foldertargetVariants {
				t.Run(tc.preset+" "+kind+" "+variant.name, func(t *testing.T) {
					root := newAgentsMDFoldersProject(t, "agents_md = true\n", append([]string{tc.preset}, variant.presets...), variant.extra)
					writeAgentsMDFile(t, root, ".ai-rulez/"+kind+"/folder-only.md",
						"---\ntargets: ["+`"`+tc.target+`"`+"]\n---\nFOLDER_ONLY_BODY\n")
					runAgentsMDGenerate(t, root)

					file := tc.file
					if kind == "context" {
						file = strings.Replace(file, "folder-only", "context-folder-only", 1)
						if !strings.HasSuffix(tc.target, "/") {
							t.Skip("file-path targets name the rule file, not the context file")
						}
					}
					assert.Contains(t, readAgentsMDFile(t, root, file), "FOLDER_ONLY_BODY")
					total := 0
					for _, rel := range agentsMDPaths(t, root) {
						if strings.HasPrefix(rel, ".agents/skills") {
							continue
						}
						total += strings.Count(readAgentsMDFile(t, root, rel), "FOLDER_ONLY_BODY")
					}
					require.Equal(t, 1, total, "item appears exactly once")
					assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "FOLDER_ONLY_BODY")
				})
			}
		}
	}
}

var foldertargetVariants = []struct {
	name    string
	presets []string
	extra   string
}{
	{"alone", nil, ""},
	{"with codex", []string{"codex"}, ""},
	{"inline mode", nil, "\n[rules]\nmode = \"inline\"\n"},
	{"split mode", nil, "\n[rules]\nmode = \"split\"\n"},
}

// emptyDirs lists the empty directories below root, outside .ai-rulez.
func emptyDirs(t *testing.T, root string) []string {
	t.Helper()
	var empty []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".ai-rulez" {
			return filepath.SkipDir
		}
		entries, readErr := os.ReadDir(path)
		if readErr == nil && len(entries) == 0 {
			empty = append(empty, filepath.ToSlash(rel))
		}
		return readErr
	}))
	return empty
}

// With agents_md on and no item bound for a rules folder, the preset leaves no
// empty folder behind, and turning the flag on then generating again changes
// nothing (the folders are neither pruned nor recreated).
func TestAgentsMD_NoEmptyRulesFolders(t *testing.T) {
	rulesDirs := map[string]string{
		"cursor": ".cursor/rules", "claude": ".claude/rules", "copilot": ".github/instructions",
		"devin": ".devin/rules", "cline": ".clinerules", "junie": ".junie/rules",
	}
	for preset, dir := range rulesDirs {
		t.Run(preset, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{preset}, "", ""))
			runAgentsMDGenerate(t, root)

			setAgentsMDFlag(t, root, "agents_md = true\n", []string{preset})
			runAgentsMDGenerate(t, root)
			assert.NotContains(t, emptyDirs(t, root), dir)
			assert.NoDirExists(t, filepath.Join(root, filepath.FromSlash(dir)))
			firstFiles, firstEmpty := agentsMDSnapshot(t, root), emptyDirs(t, root)

			runAgentsMDGenerate(t, root)
			assert.Equal(t, firstFiles, agentsMDSnapshot(t, root))
			assert.Equal(t, firstEmpty, emptyDirs(t, root))
		})
	}
}
