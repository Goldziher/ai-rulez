package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// componentProject writes one agent or skill with the given frontmatter lines.
func componentProject(t *testing.T, kind, frontmatter string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{".ai-rulez/config.toml": baseConfig}
	switch kind {
	case kindAgent:
		files[".ai-rulez/agents/helper.md"] = "---\nname: helper\ndescription: Use when you need the helper agent to review things.\n" + frontmatter + "---\nBody.\n"
	case kindSkill:
		files[".ai-rulez/skills/helper/SKILL.md"] = "---\nname: helper\ndescription: Use when you need the helper skill to do things.\n" + frontmatter + "---\n# Helper\n"
	}
	for k, v := range extra {
		files[k] = v
	}
	writeFiles(t, root, files)
	gitAdd(t, root)
	return root
}

func componentFile(kind string) string {
	if kind == kindAgent {
		return "agents/helper.md"
	}
	return "skills/helper/SKILL.md"
}

func codesIn(fs []Finding, file string) map[string]int {
	out := map[string]int{}
	for _, f := range fs {
		if strings.HasSuffix(f.File, file) {
			out[f.Code]++
		}
	}
	return out
}

func TestFrontmatterHooksAreChecked(t *testing.T) {
	const hooks = "hooks:\n  PreToolUse:\n    - matcher: Bash\n      hooks:\n        - type: command\n          command: %s\n"
	tests := []struct {
		name, command string
		extra         map[string]string
		mode          os.FileMode
		want          map[string]int
		wantLine      int
	}{
		{"script under the project dir is missing", `"\"$CLAUDE_PROJECT_DIR\"/tools/missing.sh"`, nil, 0, map[string]int{CodeHookMissing: 1}, 8},
		{"script exists and is executable", `"${CLAUDE_PROJECT_DIR}/tools/hook.sh --flag"`, map[string]string{"tools/hook.sh": "#!/bin/sh\n"}, 0o755, map[string]int{}, 0},
		{"script exists but is not executable", `"${CLAUDE_PROJECT_DIR}/tools/hook.sh"`, map[string]string{"tools/hook.sh": "#!/bin/sh\n"}, 0o644, map[string]int{CodeHookNotExecutable: 1}, 8},
		{"relative script is missing", `./tools/nope.sh`, nil, 0, map[string]int{CodeHookMissing: 1}, 8},
		{"relative script is not executable", `./tools/hook.sh`, map[string]string{"tools/hook.sh": "#!/bin/sh\n"}, 0o644, map[string]int{CodeHookNotExecutable: 1}, 8},
		{"unpinned npx -y", `"npx -y some-linter --fix"`, nil, 0, map[string]int{CodeUnpinnedExec: 1}, 8},
		{"pinned npx -y", `"npx -y some-linter@1.2.3 --fix"`, nil, 0, map[string]int{}, 0},
	}
	for _, kind := range []string{kindAgent, kindSkill} {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				// Arrange
				root := componentProject(t, kind, strings.Replace(hooks, "%s", tt.command, 1), tt.extra)
				if tt.mode != 0 {
					require.NoError(t, os.Chmod(filepath.Join(root, "tools/hook.sh"), tt.mode))
					gitAdd(t, root) // the index records the mode lint reads
				}

				// Act
				fs := lintReport(t, root).Findings

				// Assert
				got := codesIn(fs, componentFile(kind))
				for code, n := range tt.want {
					assert.Equal(t, n, got[code], "%s in %v", code, got)
				}
				for _, code := range []string{CodeHookMissing, CodeHookNotExecutable, CodeUnpinnedExec, CodeHookSchema} {
					if _, expected := tt.want[code]; !expected {
						assert.Zero(t, got[code], "unexpected %s in %v", code, got)
					}
				}
				if tt.wantLine > 0 {
					for _, f := range fs {
						if f.Code != CodeHookSchema && strings.HasSuffix(f.File, componentFile(kind)) && tt.want[f.Code] > 0 {
							assert.Contains(t, []int{tt.wantLine, tt.wantLine + 1}, f.Line, f.Code)
						}
					}
				}
			})
		}
	}
}

func TestFrontmatterHookSchemaIsChecked(t *testing.T) {
	for _, kind := range []string{kindAgent, kindSkill} {
		t.Run(kind, func(t *testing.T) {
			root := componentProject(t, kind, "hooks:\n  PreToolUsee:\n    - hooks:\n        - type: command\n          command: echo hi\n  Stop:\n    - hooks:\n        - type: shell\n", nil)

			fs := lintReport(t, root).Findings

			var msgs []string
			for _, f := range fs {
				if f.Code == CodeHookSchema {
					msgs = append(msgs, f.Message)
				}
			}
			require.Len(t, msgs, 2, "%v", msgs)
			assert.Contains(t, strings.Join(msgs, "\n"), `"PreToolUsee" is not a Claude Code event`)
			assert.Contains(t, strings.Join(msgs, "\n"), `unknown type "shell"`)
		})
	}
}

func TestFrontmatterHookScriptIsADependency(t *testing.T) {
	root := componentProject(t, kindAgent, "hooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: \"${CLAUDE_PROJECT_DIR}/tools/hook.sh\"\n",
		map[string]string{"tools/hook.sh": "#!/bin/sh\n"})

	rep := lintReport(t, root)

	assert.Contains(t, rep.Deps[".ai-rulez/agents/helper.md"], "tools/hook.sh")
}

func TestFrontmatterMCPServersAreChecked(t *testing.T) {
	const front = "mcpServers:\n  linter:\n    command: %s\n    args: [%s]\n    env:\n      API_TOKEN: abcdefghij12345\n  remote:\n    type: http\n    url: https://example.com/mcp\n  broken:\n    type: stdio\n"
	for _, kind := range []string{kindAgent, kindSkill} {
		t.Run(kind, func(t *testing.T) {
			// Arrange
			root := componentProject(t, kind, strings.Replace(strings.Replace(front, "%s", "sh", 1), "%s", `"-c", "true"`, 1), nil)
			root2 := componentProject(t, kind, strings.Replace(strings.Replace(front, "%s", "definitely-not-a-real-binary-xyz", 1), "%s", `"-y", "some-server@1.0.0"`, 1), nil)

			// Act
			onPath := codesIn(lintReport(t, root).Findings, componentFile(kind))
			missing := codesIn(lintReport(t, root2).Findings, componentFile(kind))

			// Assert
			assert.Equal(t, 1, onPath[CodeSecretInConfig], "literal credential in env: %v", onPath)
			assert.Equal(t, 1, onPath[CodeMCPConfigInvalid], "the server without a command: %v", onPath)
			assert.Zero(t, onPath[CodeMCPCommandNotFound])
			assert.Equal(t, 1, missing[CodeMCPCommandNotFound], "command not on PATH: %v", missing)
		})
	}
}

func TestFrontmatterMCPPinsAndReferences(t *testing.T) {
	root := componentProject(t, kindAgent, "mcpServers:\n  - slack\n  - pinned:\n      command: npx\n      args: [\"-y\", \"some-server@1.0.0\"]\n  - floating:\n      command: npx\n      args: [\"-y\", \"some-server\"]\n", nil)

	fs := lintReport(t, root).Findings

	var unpinned []Finding
	for _, f := range fs {
		if f.Code == CodeMCPUnpinned {
			unpinned = append(unpinned, f)
		}
	}
	require.Len(t, unpinned, 1, "%v", findingKeys(fs))
	assert.Contains(t, unpinned[0].Message, `"floating"`)
	assert.Equal(t, 9, unpinned[0].Line, "the line of the server name in the file")
	assert.Zero(t, codesIn(fs, "agents/helper.md")[CodeMCPConfigInvalid], "a name reference is not a definition")
}

func TestFrontmatterComponentChecksRunUnderTheirAnalyzers(t *testing.T) {
	root := componentProject(t, kindAgent, "hooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: \"npx -y thing\"\nmcpServers:\n  floating:\n    command: npx\n    args: [\"-y\", \"some-server\"]\n", nil)
	full := lintReport(t, root)

	for _, name := range []string{AnalyzerSecurity, AnalyzerHooks, AnalyzerMCP} {
		sel := lintRoot(t, root, Options{Analyzers: []string{name}})
		want := &Report{Findings: append([]Finding(nil), full.Findings...)}
		FilterAnalyzers(want, []string{name})
		assert.Equal(t, findingKeys(want.Findings), findingKeys(sel.Findings), name)
	}
}
