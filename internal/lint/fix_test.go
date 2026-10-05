package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureSkill = "---\nname: Deploy_Helper\ndescription: Use when deploying things to the staging environment.\nallowed_tools: Read\n---\n# Deploy\n"

func fixProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":                         baseConfig,
		".ai-rulez/skills/deploy-helper/SKILL.md":       fixtureSkill,
		".ai-rulez/skills/deploy-helper/scripts/run.sh": "#!/bin/sh\necho hi\n",
		".ai-rulez/skills/other/SKILL.md":               "---\nname: other\ndescription: Use when you need some other skill to do things.\n---\nDone.\n",
	})
	require.NoError(t, os.Chmod(filepath.Join(root, ".ai-rulez/skills/deploy-helper/scripts/run.sh"), 0o644))
	gitAdd(t, root)
	return root
}

func fixOptions(root string) FixOptions {
	return FixOptions{EditRoot: gitutil.Resolve(filepath.Join(root, ".ai-rulez"))}
}

func fixesOf(fs []Finding, code string) []Finding {
	var out []Finding
	for i := range fs {
		if fs[i].Code == code {
			out = append(out, fs[i])
		}
	}
	return out
}

func TestFindingsCarryFixes(t *testing.T) {
	fs := lintReport(t, fixProject(t)).Findings
	key := fixesOf(fs, CodeFrontmatterKey)
	require.Len(t, key, 1)
	require.True(t, key[0].Fixable())
	assert.Equal(t, FixSafe, key[0].Meta.Fix.Confidence)
	assert.Equal(t, "allowed-tools: Read", key[0].Meta.Fix.Edits[0].New)

	name := fixesOf(fs, CodeSkillNameInvalid)
	require.NotEmpty(t, name)
	assert.Equal(t, FixUnsafe, name[0].Meta.Fix.Confidence)
	assert.Equal(t, "name: deploy-helper", name[0].Meta.Fix.Edits[0].New)

	exe := fixesOf(fs, CodeScriptNotExecutable)
	require.Len(t, exe, 1)
	assert.Equal(t, FixSafe, exe[0].Meta.Fix.Confidence)
	assert.Len(t, exe[0].Meta.Fix.Chmods, 1)
}

func TestApplySafeFixesOnlyByDefault(t *testing.T) {
	root := fixProject(t)
	rep := lintReport(t, root)
	res, err := ApplyFixes(rep.Findings, fixOptions(root))
	require.NoError(t, err)

	codes := map[string]bool{}
	for _, a := range res.Applied {
		codes[a.Code] = true
	}
	assert.Equal(t, map[string]bool{CodeFrontmatterKey: true, CodeScriptNotExecutable: true}, codes)
	var unsafeSkipped bool
	for _, s := range res.Skipped {
		unsafeSkipped = unsafeSkipped || (s.Code == CodeSkillNameInvalid && strings.Contains(s.Reason, "--fix-unsafe"))
	}
	assert.True(t, unsafeSkipped, "AR804 needs --fix-unsafe: %+v", res.Skipped)

	data, err := os.ReadFile(filepath.Join(root, ".ai-rulez/skills/deploy-helper/SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "allowed-tools: Read")
	assert.Contains(t, string(data), "name: Deploy_Helper", "the unsafe rename did not run")
	info, err := os.Stat(filepath.Join(root, ".ai-rulez/skills/deploy-helper/scripts/run.sh"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o100, "the script is executable now")
}

func TestApplyUnsafeFixesAndIdempotence(t *testing.T) {
	root := fixProject(t)
	o := fixOptions(root)
	o.Unsafe = true
	res, err := ApplyFixes(lintReport(t, root).Findings, o)
	require.NoError(t, err)
	assert.Len(t, res.Applied, 3)

	after := lintReport(t, root).Findings
	assert.Empty(t, fixesOf(after, CodeFrontmatterKey))
	assert.Empty(t, fixesOf(after, CodeSkillNameInvalid))
	assert.Empty(t, fixesOf(after, CodeScriptNotExecutable))

	// Running again finds nothing to do and rewrites nothing.
	again, err := ApplyFixes(after, o)
	require.NoError(t, err)
	assert.Empty(t, again.Applied)
	assert.Empty(t, again.Diff)
}

func TestDryRunChangesNothingAndPrintsDiff(t *testing.T) {
	root := fixProject(t)
	path := filepath.Join(root, ".ai-rulez/skills/deploy-helper/SKILL.md")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	o := fixOptions(root)
	o.Unsafe, o.DryRun = true, true

	res, err := ApplyFixes(lintReport(t, root).Findings, o)
	require.NoError(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.Contains(t, res.Diff, "--- a/")
	assert.Contains(t, res.Diff, "-allowed_tools: Read\n+allowed-tools: Read")
	assert.Contains(t, res.Diff, "-name: Deploy_Helper\n+name: deploy-helper")
	assert.Contains(t, res.Diff, "chmod 0644 -> 0755")
	assert.Len(t, res.Applied, 3, "a dry run reports what it would apply")
}

func TestFixesRefuseGeneratedAndForeignFiles(t *testing.T) {
	root := fixProject(t)
	rep := lintReport(t, root)
	o := fixOptions(root)
	o.Unsafe = true
	o.Refuse = func(abs string) string {
		if strings.HasSuffix(abs, "run.sh") {
			return "generated"
		}
		return ""
	}
	res, err := ApplyFixes(rep.Findings, o)
	require.NoError(t, err)
	for _, a := range res.Applied {
		assert.NotEqual(t, CodeScriptNotExecutable, a.Code)
	}

	o.EditRoot = filepath.Join(root, "elsewhere")
	res, err = ApplyFixes(lintReport(t, root).Findings, o)
	require.NoError(t, err)
	assert.Empty(t, res.Applied, "text edits outside the authored root are refused")
}

func TestFixesNeverApplyToSecurityFindings(t *testing.T) {
	f := finding(CodeSecretDetected, "a.md", 1, "s")
	f.meta().Fix = &Fix{Description: "redact", Confidence: FixSafe, Edits: []Edit{{File: "/nope", Line: 1, Old: "a", New: "b"}}}
	res, err := ApplyFixes([]Finding{f}, FixOptions{Unsafe: true})
	require.NoError(t, err)
	assert.Empty(t, res.Applied)
	require.Len(t, res.Skipped, 1)
	assert.Contains(t, res.Skipped[0].Reason, "security")
}

func TestStaleEditIsSkippedNotApplied(t *testing.T) {
	root := fixProject(t)
	rep := lintReport(t, root)
	path := filepath.Join(root, ".ai-rulez/skills/deploy-helper/SKILL.md")
	edited := strings.Replace(fixtureSkill, "allowed_tools: Read", "allowed_tools: Read, Grep", 1)
	require.NoError(t, os.WriteFile(path, []byte(edited), 0o644))

	res, err := ApplyFixes(rep.Findings, fixOptions(root))
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, edited, string(data), "a line that changed since the lint run is left alone")
	var skipped bool
	for _, s := range res.Skipped {
		skipped = skipped || s.Code == CodeFrontmatterKey
	}
	assert.True(t, skipped)
}

func TestFixPreservesCRLFAndPermissions(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":          baseConfig,
		".ai-rulez/skills/crlf/SKILL.md": "---\r\nname: crlf\r\ndescription: Use when you need the CRLF skill for tests.\r\nallowed_tools: Read\r\n---\r\nbody\r\n",
	})
	gitAdd(t, root)
	path := filepath.Join(root, ".ai-rulez/skills/crlf/SKILL.md")
	require.NoError(t, os.Chmod(path, 0o640))

	res, err := ApplyFixes(lintReport(t, root).Findings, fixOptions(root))
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "---\r\nname: crlf\r\ndescription: Use when you need the CRLF skill for tests.\r\nallowed-tools: Read\r\n---\r\nbody\r\n", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestKeyAndNameLineRewriting(t *testing.T) {
	got, ok := renameKeyLine("  allowed_tools : Read", "allowed_tools", "allowed-tools")
	assert.True(t, ok)
	assert.Equal(t, "  allowed-tools : Read", got)
	_, ok = renameKeyLine("description: allowed_tools", "allowed_tools", "allowed-tools")
	assert.False(t, ok, "only a line that starts with the key")
	got, ok = renameNameLine(`name: "Deploy_Helper"  # the name`, "deploy-helper")
	assert.True(t, ok)
	assert.Equal(t, `name: "deploy-helper"  # the name`, got)
	_, ok = renameNameLine("name: 'mixed\"", "x")
	assert.False(t, ok)

	assert.Equal(t, "deploy-helper", normalizeSkillName("Deploy_Helper"))
	assert.Equal(t, "a-b", normalizeSkillName("  a -- b!! "))
	assert.Equal(t, "", normalizeSkillName("___"))
	assert.LessOrEqual(t, len(normalizeSkillName(strings.Repeat("ab-", 40))), maxSkillNameLen)
}

func TestUnifiedDiffHunks(t *testing.T) {
	old := strings.Split("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl", "\n")
	neu := append([]string(nil), old...)
	neu[1], neu[10] = "B", "K"
	d := unifiedDiff("f.md", old, neu)
	assert.Equal(t, 2, strings.Count(d, "@@ -"), "distant edits make two hunks")
	assert.Contains(t, d, "-b\n+B")
	assert.Contains(t, d, "-k\n+K")
	assert.Empty(t, unifiedDiff("f", old, old))
}

func TestHookScriptFixesStageTheExecutableBit(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig + "\n[[hooks]]\nevent = \"PreToolUse\"\n[[hooks.hooks]]\nscript = \"tools/plain.sh\"\n",
		".claude/settings.json": `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/tools/direct.sh"}]}]}}`,
		"tools/plain.sh":        "#!/bin/sh\n",
		"tools/direct.sh":       "#!/bin/sh\n",
	})
	gitAdd(t, root)
	fs := lintReport(t, root).Findings
	require.Len(t, fixesOf(fs, CodeHookNotExecutable), 1)
	require.Len(t, fixesOf(fs, CodeHookSourceNotExec), 1)

	res, err := ApplyFixes(fs, fixOptions(root))
	require.NoError(t, err)
	assert.Len(t, res.Applied, 2)
	for _, f := range []string{"tools/plain.sh", "tools/direct.sh"} {
		info, err := os.Stat(filepath.Join(root, f))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o100, f)
	}

	// The finding is read from the index mode, so staging the bit is what clears it.
	after := lintReport(t, root).Findings
	assert.Empty(t, fixesOf(after, CodeHookNotExecutable))
	assert.Empty(t, fixesOf(after, CodeHookSourceNotExec))
	again, err := ApplyFixes(after, fixOptions(root))
	require.NoError(t, err)
	assert.Empty(t, again.Applied)
}
