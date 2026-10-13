package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillDirHookScriptIsResolved(t *testing.T) {
	const hooks = "hooks:\n  PreToolUse:\n    - matcher: Bash\n      hooks:\n        - type: command\n          command: %s\n"
	const script = ".ai-rulez/skills/helper/scripts/hook.sh"
	bundled := map[string]string{script: "#!/bin/sh\n"}
	elsewhere := map[string]string{"tools/hook.sh": "#!/bin/sh\n"}
	tests := []struct {
		name, command string
		extra         map[string]string
		exists        string
		mode          os.FileMode
		want          map[string]int
	}{
		{"braced variable, script missing", `"${CLAUDE_SKILL_DIR}/scripts/nope.sh"`, nil, "", 0, map[string]int{CodeHookMissing: 1}},
		{"plain variable, script missing", `"$CLAUDE_SKILL_DIR/scripts/nope.sh"`, nil, "", 0, map[string]int{CodeHookMissing: 1}},
		{"quoted variable, script missing", `"\"${CLAUDE_SKILL_DIR}\"/scripts/nope.sh"`, nil, "", 0, map[string]int{CodeHookMissing: 1}},
		{"braced variable, executable script", `"${CLAUDE_SKILL_DIR}/scripts/hook.sh --flag"`, bundled, script, 0o755, map[string]int{}},
		{"plain variable, executable script", `"$CLAUDE_SKILL_DIR/scripts/hook.sh"`, bundled, script, 0o755, map[string]int{}},
		{"script not executable", `"${CLAUDE_SKILL_DIR}/scripts/hook.sh"`, bundled, script, 0o644, map[string]int{CodeHookNotExecutable: 1}},
		{"launcher needs no exec bit", `"bash ${CLAUDE_SKILL_DIR}/scripts/hook.sh"`, bundled, script, 0o644, map[string]int{}},
		{"launcher, script missing", `"bash ${CLAUDE_SKILL_DIR}/scripts/nope.sh"`, nil, "", 0, map[string]int{CodeHookMissing: 1}},
		{"the project dir does not stand in for the skill dir", `"${CLAUDE_SKILL_DIR}/tools/hook.sh"`, elsewhere, "tools/hook.sh", 0o755, map[string]int{CodeHookMissing: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := componentProject(t, kindSkill, strings.Replace(hooks, "%s", tt.command, 1), tt.extra)
			if tt.mode != 0 {
				require.NoError(t, os.Chmod(filepath.Join(root, filepath.FromSlash(tt.exists)), tt.mode))
				gitAdd(t, root) // the index records the mode lint reads
				if tt.mode&0o111 != 0 {
					gitSetExec(t, root, tt.exists) // Windows cannot express the bit on disk
				}
			}

			// Act
			got := codesIn(lintReport(t, root).Findings, componentFile(kindSkill))

			// Assert
			for _, code := range []string{CodeHookMissing, CodeHookNotExecutable} {
				assert.Equal(t, tt.want[code], got[code], "%s in %v", code, got)
			}
		})
	}
}

func TestSkillDirHookScriptIsADependencyAndAgentsAreSkipped(t *testing.T) {
	const hook = "hooks:\n  PreToolUse:\n    - hooks:\n        - type: command\n          command: \"${CLAUDE_SKILL_DIR}/scripts/hook.sh\"\n"
	const script = ".ai-rulez/skills/helper/scripts/hook.sh"
	root := componentProject(t, kindSkill, hook, map[string]string{script: "#!/bin/sh\n"})
	require.NoError(t, os.Chmod(filepath.Join(root, filepath.FromSlash(script)), 0o755))
	gitAdd(t, root)
	gitSetExec(t, root, script)

	rep := lintReport(t, root)

	assert.Contains(t, rep.Deps[".ai-rulez/skills/helper/SKILL.md"], script)

	agent := componentProject(t, kindAgent, hook, nil)
	assert.Zero(t, codesIn(lintReport(t, agent).Findings, componentFile(kindAgent))[CodeHookMissing], "the variable is a skill's own")
}
