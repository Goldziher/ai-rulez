package lint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goodRule = "---\ndescription: a rule\n---\n# Title\n"

func markdownProject(t *testing.T, rule string) (root, file string) {
	t.Helper()
	root = t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/a.md": rule})
	gitAdd(t, root)
	return root, filepath.Join(root, ".ai-rulez", "rules", "a.md")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestMarkdownShapeRules(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		wantFix  string // expected file after --fix ("" when nothing is reported)
		wantCode []string
	}{
		{"clean file", goodRule, "", nil},
		{"unclosed backtick fence", goodRule + "```bash\nls\n", goodRule + "```bash\nls\n```\n", []string{CodeFenceUnclosed}},
		{"unclosed tilde fence of length four", goodRule + "~~~~\ncode\n", goodRule + "~~~~\ncode\n~~~~\n", []string{CodeFenceUnclosed}},
		{"longer closing fence is fine", goodRule + "```\ncode\n`````\n", "", nil},
		{"shorter closing fence leaves it open", goodRule + "`````\ncode\n```\n", goodRule + "`````\ncode\n```\n`````\n", []string{CodeFenceUnclosed}},
		{"fence in frontmatter text is not a fence", "---\ndescription: \"```\"\n---\n# T\n", "", nil},
		{"missing final newline", goodRule + "last line", goodRule + "last line\n", []string{CodeFinalNewline}},
		{"unclosed fence and no final newline are one fix", goodRule + "```\ncode", goodRule + "```\ncode\n```\n", []string{CodeFenceUnclosed, CodeFinalNewline}},
		{"unclosed fence in a CRLF file", "---\r\ndescription: a rule\r\n---\r\n```\r\ncode\r\n", "---\r\ndescription: a rule\r\n---\r\n```\r\ncode\r\n```\r\n", []string{CodeFenceUnclosed}},
		{"unclosed fence and no newline in a CRLF file", "---\r\ndescription: a rule\r\n---\r\n```\r\ncode", "---\r\ndescription: a rule\r\n---\r\n```\r\ncode\r\n```\r\n", []string{CodeFenceUnclosed, CodeFinalNewline}},
		{"missing newline in a CRLF file", "---\r\ndescription: a rule\r\n---\r\n# T\r\nlast", "---\r\ndescription: a rule\r\n---\r\n# T\r\nlast\r\n", []string{CodeFinalNewline}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, file := markdownProject(t, tt.rule)
			rep := lintReport(t, root)
			var got []string
			for _, f := range rep.Findings {
				if f.Code == CodeFenceUnclosed || f.Code == CodeFinalNewline {
					got = append(got, f.Code)
					assert.True(t, f.Fixable(), f.Code)
					assert.Equal(t, FixSafe, f.Meta.Fix.Confidence)
				}
			}
			assert.ElementsMatch(t, tt.wantCode, got)

			// Act
			res, err := ApplyFixes(rep.Findings, fixOptions(root))
			require.NoError(t, err)

			// Assert
			if tt.wantFix == "" {
				assert.Equal(t, tt.rule, readFile(t, file))
				return
			}
			assert.Equal(t, tt.wantFix, readFile(t, file))
			assert.Empty(t, res.Skipped)
			again := lintReport(t, root)
			for _, f := range again.Findings {
				assert.NotContains(t, []string{CodeFenceUnclosed, CodeFinalNewline}, f.Code, "the fix leaves nothing to report")
			}
			second, err := ApplyFixes(again.Findings, fixOptions(root))
			require.NoError(t, err)
			assert.Empty(t, second.Applied, "running --fix twice changes nothing the second time")
			assert.Equal(t, tt.wantFix, readFile(t, file))
		})
	}
}

func TestMarkdownFixRepeatedOnStaleFindingsIsIdempotent(t *testing.T) {
	root, file := markdownProject(t, goodRule+"```\ncode")
	rep := lintReport(t, root)

	first, err := ApplyFixes(rep.Findings, fixOptions(root))
	require.NoError(t, err)
	second, err := ApplyFixes(rep.Findings, fixOptions(root)) // the same findings again

	require.NoError(t, err)
	assert.NotEmpty(t, first.Applied)
	assert.Empty(t, second.Applied)
	assert.Equal(t, goodRule+"```\ncode\n```\n", readFile(t, file))
}

func TestMarkdownFixKeepsFileModeAndShowsADiffInDryRun(t *testing.T) {
	root, file := markdownProject(t, goodRule+"```\ncode\n")
	require.NoError(t, os.Chmod(file, 0o640))
	rep := lintReport(t, root)
	opts := fixOptions(root)
	opts.DryRun = true

	dry, err := ApplyFixes(rep.Findings, opts)
	require.NoError(t, err)

	assert.Equal(t, goodRule+"```\ncode\n", readFile(t, file), "a dry run writes nothing")
	assert.Contains(t, dry.Diff, "+```\n")
	assert.Contains(t, dry.Diff, "@@ -")
	opts.DryRun = false
	_, err = ApplyFixes(rep.Findings, opts)
	require.NoError(t, err)
	if runtime.GOOS != "windows" { // Windows has no permission bits to keep
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	}
}

func TestMarkdownFixSkipsBaselinedFindings(t *testing.T) {
	root, file := markdownProject(t, goodRule+"last line")
	rep := lintReport(t, root)
	base, err := UpdateBaseline(rep, nil, "accepted")
	require.NoError(t, err)
	ApplyBaseline(rep, base, "baseline.json", "2026-01-01")

	res, err := ApplyFixes(rep.Findings, fixOptions(root))

	require.NoError(t, err)
	assert.Empty(t, res.Applied)
	assert.Equal(t, goodRule+"last line", readFile(t, file))
}

func TestMarkdownShapeIgnoresEmptyAndNewlineOnlyFiles(t *testing.T) {
	for _, body := range []string{"", "\n"} {
		root, _ := markdownProject(t, body)
		for _, f := range lintReport(t, root).Findings {
			assert.NotContains(t, []string{CodeFenceUnclosed, CodeFinalNewline}, f.Code, "%q", body)
		}
	}
}

func TestStringBooleanFrontmatterIsFixed(t *testing.T) {
	tests := []struct {
		name, line, want string
		fixable          bool
	}{
		{"double quoted true", `user-invocable: "true"`, "user-invocable: true", true},
		{"single quoted false with comment", `disable-model-invocation: 'false' # off`, "disable-model-invocation: false # off", true},
		{"upper case value", `user-invocable: "True"`, "user-invocable: true", true},
		{"a number is reported but not fixed", `user-invocable: 1`, "", false},
		{"yes-style string is reported but not fixed", `user-invocable: "yes"`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			skill := "---\nname: helper\ndescription: Use when you need the helper to do things for you.\n" + tt.line + "\n---\n# Helper\n"
			writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/skills/helper/SKILL.md": skill})
			gitAdd(t, root)
			rep := lintReport(t, root)
			bad := fixesOf(rep.Findings, CodeFrontmatterValue)
			require.Len(t, bad, 1)
			assert.Equal(t, tt.fixable, bad[0].Fixable())

			// Act
			_, err := ApplyFixes(rep.Findings, fixOptions(root))
			require.NoError(t, err)

			// Assert
			got := readFile(t, filepath.Join(root, ".ai-rulez/skills/helper/SKILL.md"))
			if tt.fixable {
				assert.Contains(t, got, "\n"+tt.want+"\n")
				assert.Empty(t, fixesOf(lintReport(t, root).Findings, CodeFrontmatterValue))
			} else {
				assert.Equal(t, skill, got)
			}
		})
	}
}

func TestEveryBooleanKeyOfEveryKindIsCoerced(t *testing.T) {
	for kind, keys := range boolKeys {
		for _, key := range keys {
			t.Run(kind+"/"+key, func(t *testing.T) {
				it := &item{abs: "/x/a.md", kind: kind}
				d := parseDoc("---\n" + key + ": \"false\"\n---\n")
				fix := boolCoercion(it, d, fmKey{Name: key, Line: 2})
				require.NotNil(t, fix)
				assert.Equal(t, key+": false", fix.Edits[0].New)
			})
		}
	}
}

func TestRuleDocsMentionBadAndGoodForTheMarkdownRules(t *testing.T) {
	for _, code := range []string{CodeFenceUnclosed, CodeFinalNewline} {
		e, ok := Explain(code)
		require.True(t, ok)
		assert.NotEmpty(t, e.Bad)
		assert.NotEmpty(t, e.Good)
		assert.True(t, strings.Contains(e.Why, "validate --fix"), code)
	}
}

func TestMarkdownFixDryRunDiffShowsTheMissingNewline(t *testing.T) {
	root, file := markdownProject(t, goodRule+"```\ncode")
	rep := lintReport(t, root)
	opts := fixOptions(root)
	opts.DryRun = true

	res, err := ApplyFixes(rep.Findings, opts)

	require.NoError(t, err)
	assert.Equal(t, goodRule+"```\ncode", readFile(t, file))
	want := "@@ -3,4 +3,5 @@\n ---\n # Title\n ```\n-code\n\\ No newline at end of file\n+code\n+```\n"
	assert.Contains(t, res.Diff, want)
	assert.NotContains(t, res.Diff, "+\n", "the final newline is not shown as an empty added line")
}
