package commands

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudgeChangedIncludesUnitsAffectedByDeletedConventions(t *testing.T) {
	const deletedText = "MUST keep the deleted convention marker."
	tests := []struct {
		name, deleted string
		files         map[string]string
		wantA, wantB  bool
	}{
		{
			name: "scoped rule", deleted: ".claude/rules/a.md",
			files: map[string]string{".claude/rules/a.md": "---\npaths: [\".claude/skills/a/**\"]\n---\n" + deletedText},
			wantA: true,
		},
		{
			name: "ancestor instructions", deleted: "AGENTS.md",
			files: map[string]string{"AGENTS.md": "# Instructions\n" + deletedText},
			wantA: true, wantB: true,
		},
		{
			name: "imported document", deleted: "docs/convention.md",
			files: map[string]string{"CLAUDE.md": "# Root\n@docs/convention.md\n", "docs/convention.md": "# Imported\n" + deletedText},
			wantA: true, wantB: true,
		},
		{
			name: "unrelated document", deleted: "docs/unrelated.md",
			files: map[string]string{"docs/unrelated.md": "# Unrelated\n" + deletedText},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.files[".claude/skills/a/SKILL.md"] = "---\nname: a\ndescription: Use when a.\n---\nBody a.\n"
			tt.files[".claude/skills/b/SKILL.md"] = "---\nname: b\ndescription: Use when b.\n---\nBody b.\n"
			dir := judgeProject(t, tt.files)
			crossGit(t, dir, "init", "-q", "-b", "main")
			crossGit(t, dir, "add", "-A")
			crossGit(t, dir, "commit", "-qm", "initial conventions")
			require.NoError(t, os.Remove(filepath.Join(dir, filepath.FromSlash(tt.deleted))))
			rig := newJudgeRig(t)

			stdout, code := rig.run(t, "--estimate", "--model", judgeTestModel, "--format", "json", "--changed", ".claude/skills/a", ".claude/skills/b")
			require.Equal(t, 0, code, rig.stderr.String())
			paths := parseJudgeDoc(t, stdout).paths()
			assert.Equal(t, tt.wantA, slices.Contains(paths, ".claude/skills/a"))
			assert.Equal(t, tt.wantB, slices.Contains(paths, ".claude/skills/b"))

			stdout, code = rig.run(t, "--estimate", "--model", judgeTestModel, "--format", "json", "--changed", "--impact=false", ".claude/skills/a", ".claude/skills/b")
			require.Equal(t, 0, code, rig.stderr.String())
			assert.NotContains(t, parseJudgeDoc(t, stdout).paths(), ".claude/skills/a")
			assert.NotContains(t, parseJudgeDoc(t, stdout).paths(), ".claude/skills/b")

			if tt.wantA {
				_, code = rig.run(t, append(append([]string{}, judgeRun...), "--changed", ".claude/skills/a", ".claude/skills/b")...)
				require.Equal(t, 0, code, rig.stderr.String())
				for _, call := range rig.fake.ChatCalls() {
					for _, message := range call.Messages {
						assert.NotContains(t, message.Content, deletedText, "deleted text only determines impact")
					}
				}
			}
		})
	}
}
