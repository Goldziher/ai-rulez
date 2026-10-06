package okf

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func load(t *testing.T, files map[string]string) *Bundle {
	t.Helper()
	m := fstest.MapFS{}
	for p, c := range files {
		m[p] = &fstest.MapFile{Data: []byte(c)}
	}
	b, err := Load(m)
	require.NoError(t, err)
	return b
}

func codes(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Code+" "+f.Path)
	}
	return out
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body := SplitFrontmatter([]byte("---\ntype: Metric\ntitle: X\n---\n\nhello\n---\nrule\n"))
	assert.True(t, fm.Present)
	require.NoError(t, fm.Err)
	assert.Equal(t, "Metric", fm.Scalar("type"))
	assert.Equal(t, "hello\n---\nrule\n", body)

	fm, body = SplitFrontmatter([]byte("\xef\xbb\xbf---\r\ntype: A\r\n---\r\nbody"))
	assert.Equal(t, "A", fm.Scalar("type"))
	assert.Equal(t, "body", body)

	fm, body = SplitFrontmatter([]byte("# just markdown\n"))
	assert.False(t, fm.Present)
	assert.Equal(t, "# just markdown\n", body)

	fm, _ = SplitFrontmatter([]byte("---\ntype: a\n"))
	assert.Error(t, fm.Err, "unclosed block")

	fm, _ = SplitFrontmatter([]byte("---\n: : :\n  - [\n---\nx"))
	assert.Error(t, fm.Err)
	fm, _ = SplitFrontmatter([]byte("---\n- a\n- b\n---\nx"))
	assert.Error(t, fm.Err, "a list is not a mapping")
	fm, _ = SplitFrontmatter([]byte("---\n---\nx"))
	assert.True(t, fm.Present)
	assert.Empty(t, fm.Scalar("type"))
}

func TestConformantMinimalBundle(t *testing.T) {
	b := load(t, map[string]string{
		"index.md": "---\nokf_version: \"0.2\"\n---\n\n# Concepts\n\n* [A](a.md) - the a\n",
		"a.md":     "---\ntype: Anything\n---\nbody\n",
	})
	assert.Empty(t, b.Validate())
}

func TestToleratesUnknownTypeKeysAndMissingIndex(t *testing.T) {
	b := load(t, map[string]string{
		"a.md": "---\ntype: Totally Unknown\nweird: {x: 1}\nx-other: y\n---\nbody [gone](/missing.md)\n",
	})
	fs := b.Validate()
	assert.Equal(t, []string{"AR9B2 a.md"}, codes(fs), "only the broken link, as a warning")
	assert.Equal(t, SeverityWarning, fs[0].Severity)
}

func TestConformanceFailures(t *testing.T) {
	b := load(t, map[string]string{
		"no-fm.md":     "# plain\n",
		"empty.md":     "---\ntype: \"\"\n---\n",
		"bad.md":       "---\ntype: [\n---\n",
		"good.md":      "---\ntype: X\n---\n",
		"sub/index.md": "---\nowner: nope\n---\n",
		"index.md":     "---\nokf_version: \"0.2\"\nowner: no\n---\n* [Good](good.md)\n",
	})
	got := codes(b.Validate())
	assert.Contains(t, got, "AR9B1 no-fm.md")
	assert.Contains(t, got, "AR9B1 empty.md")
	assert.Contains(t, got, "AR9B1 bad.md")
	assert.Contains(t, got, "AR9B6 sub/index.md")
	assert.Contains(t, got, "AR9B6 index.md", "extra root index key")
	assert.NotContains(t, got, "AR9B1 good.md")
}

func TestVersionChecks(t *testing.T) {
	for v, want := range map[string]string{`"0.2"`: "", "0.2": "", `"1.0"`: "info", `"abc"`: "warning", `"0.2.0"`: "warning"} {
		b := load(t, map[string]string{"index.md": "---\nokf_version: " + v + "\n---\n"})
		var sev string
		for _, f := range b.Validate() {
			if f.Code == CodeVersionInvalid {
				sev = string(f.Severity)
			}
		}
		assert.Equal(t, want, sev, v)
	}
}

func TestIndexMismatch(t *testing.T) {
	b := load(t, map[string]string{
		"index.md":   "# C\n\n* [A](a.md)\n* [Ghost](ghost.md) - gone\n* [Sub](sub/)\n",
		"a.md":       "---\ntype: X\n---\n",
		"b.md":       "---\ntype: X\n---\n",
		"sub/c.md":   "---\ntype: X\n---\n",
		"other/d.md": "---\ntype: X\n---\n",
	})
	var msgs []string
	for _, f := range b.Validate() {
		if f.Code == CodeIndexMismatch {
			msgs = append(msgs, f.Message)
		}
	}
	assert.Len(t, msgs, 3)
	assert.Contains(t, strings.Join(msgs, "|"), "ghost.md")
	assert.Contains(t, strings.Join(msgs, "|"), "b.md is not listed")
	assert.Contains(t, strings.Join(msgs, "|"), "other/ is not listed")
}

func TestLinksAndOrphans(t *testing.T) {
	b := load(t, map[string]string{
		"index.md": "* [A](a.md)\n",
		"a.md": "---\ntype: X\n---\n[ok](/b.md) [rel](./b.md#frag) [ext](https://x.y/z) [anchor](#h) [bad](nope.md) [esc](../../etc/passwd)\n" +
			"`[code](nope2.md)`\n```\n[fence](nope3.md)\n```\n",
		"b.md":    "---\ntype: X\n---\n",
		"lone.md": "---\ntype: X\n---\n",
	})
	fs := b.Validate()
	var broken, orphan []string
	for _, f := range fs {
		switch f.Code {
		case CodeLinkBroken:
			broken = append(broken, f.Message)
		case CodeOrphan:
			orphan = append(orphan, f.Path)
		}
	}
	assert.Len(t, broken, 2)
	assert.Equal(t, []string{"lone.md"}, orphan)
}

func TestNoOrphanCheckWithoutIndex(t *testing.T) {
	b := load(t, map[string]string{"a.md": "---\ntype: X\n---\n"})
	assert.Empty(t, b.Validate())
}

func TestLogHeadings(t *testing.T) {
	b := load(t, map[string]string{"log.md": "# Log\n\n## 2026-05-22\n* x\n\n## last week\n"})
	fs := b.Validate()
	require.Len(t, fs, 1)
	assert.Equal(t, CodeReservedStructure, fs[0].Code)
	assert.Equal(t, SeverityWarning, fs[0].Severity)
}

func TestDuplicateTitlesAndCaseCollision(t *testing.T) {
	b := load(t, map[string]string{
		"a.md": "---\ntype: X\ntitle: Same\n---\n",
		"b.md": "---\ntype: X\ntitle: same\n---\n",
		"C.md": "---\ntype: X\n---\n",
		"c.md": "---\ntype: X\n---\n",
	})
	got := codes(b.Validate())
	assert.Contains(t, got, "AR9B7 b.md")
	assert.Contains(t, got, "AR9B8 c.md")
}

func TestOfficialAcmeRetailBundle(t *testing.T) {
	b, err := Load(os.DirFS("testdata/acme_retail"))
	require.NoError(t, err)
	assert.Len(t, b.Concepts, 9)
	for _, f := range b.Validate() {
		assert.NotEqual(t, SeverityError, f.Severity, "%s %s: %s", f.Code, f.Path, f.Message)
		assert.NotEqual(t, CodeTypeInvalid, f.Code)
	}
	assert.Equal(t, "Metric", b.Concepts["metrics/gross-margin.md"].Type())
}

func TestBuildIndexesDeterministicAndValid(t *testing.T) {
	in := []IndexInput{
		{Path: "rules/b.md", Title: "B", Description: "second"},
		{Path: "rules/a.md", Title: "A", Description: "first"},
		{Path: "skills/s/SKILL.md", Title: "S"},
		{Path: "skills/s/references/r.md", Title: "R"},
	}
	one := BuildIndexes(in, DirLabel{"rules": "the rules"}, "")
	two := BuildIndexes([]IndexInput{in[3], in[2], in[1], in[0]}, DirLabel{"rules": "the rules"}, "")
	assert.Equal(t, one, two)
	assert.Equal(t, "---\nokf_version: \"0.2\"\n---\n\n# Subdirectories\n\n* [rules](rules/index.md) - the rules\n* [skills](skills/index.md)\n", string(one["index.md"]))
	assert.Equal(t, "# Concepts\n\n* [A](a.md) - first\n* [B](b.md) - second\n", string(one["rules/index.md"]))

	files := map[string]string{}
	for p, d := range one {
		files[p] = string(d)
	}
	for _, c := range in {
		files[c.Path] = "---\ntype: X\ntitle: '" + c.Title + "'\n---\n"
	}
	assert.Empty(t, load(t, files).Validate())
}

func TestMarshalFrontmatterOrder(t *testing.T) {
	out, err := MarshalFrontmatter([]Field{{"type", "Decision"}, {"title", "T: colon"}, {"x", map[string]any{"b": 1, "a": []string{"z"}}}})
	require.NoError(t, err)
	assert.Equal(t, "---\ntype: Decision\ntitle: 'T: colon'\nx:\n  a:\n    - z\n  b: 1\n---\n", string(out))
}

func TestWriteCompareAndPrune(t *testing.T) {
	dir := t.TempDir() + "/bundle"
	files := []File{{Path: "index.md", Data: []byte("---\nokf_version: \"0.2\"\n---\n")}, {Path: "a/b.md", Data: []byte("x")}, {Path: "s.sh", Data: []byte("#!/bin/sh\n"), Mode: 0o755}}
	require.NoError(t, WriteFiles(dir, files, true))
	d, err := Compare(dir, files)
	require.NoError(t, err)
	assert.True(t, d.Empty())
	info, err := os.Stat(dir + "/s.sh")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100)

	require.NoError(t, os.WriteFile(dir+"/a/b.md", []byte("changed"), 0o644))
	require.NoError(t, os.WriteFile(dir+"/stale.md", []byte("x"), 0o644))
	d, err = Compare(dir, files)
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b.md"}, d.Changed)
	assert.Equal(t, []string{"stale.md"}, d.Extra)

	require.NoError(t, WriteFiles(dir, files[:1], true))
	_, err = os.Stat(dir + "/a")
	assert.True(t, os.IsNotExist(err), "emptied directory is pruned")
}

func TestWriteRefusesForeignDirAndTraversal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/precious.txt", []byte("x"), 0o644))
	assert.Error(t, WriteFiles(dir, []File{{Path: "a.md", Data: nil}}, true))
	assert.NoError(t, WriteFiles(dir, []File{{Path: "a.md", Data: nil}}, false))
	for _, p := range []string{"../x.md", "/abs.md", "a/../../x.md", "a//b.md", `a\b.md`, ""} {
		assert.Error(t, ValidatePath(p), p)
	}
}

func TestWriteDoesNotFollowSymlinkOut(t *testing.T) {
	out := t.TempDir()
	dir := t.TempDir()
	testutil.SymlinkOrSkip(t, out, dir+"/link")
	err := WriteFiles(dir, []File{{Path: "link/x.md", Data: []byte("x")}}, false)
	assert.Error(t, err)
	_, statErr := os.Stat(out + "/x.md")
	assert.True(t, os.IsNotExist(statErr))
}

func TestLoadReportsSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/real.md", []byte("---\ntype: X\n---\n"), 0o644))
	testutil.SymlinkOrSkip(t, dir+"/real.md", dir+"/link.md")
	b, err := Load(os.DirFS(dir))
	require.NoError(t, err)
	assert.Contains(t, codes(b.Validate()), "AR9B8 link.md")
	assert.Len(t, b.Concepts, 1)
}

func TestLoadDoesNotFollowSymlinkedDirectories(t *testing.T) {
	// Arrange
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(outside+"/secret.md", []byte("---\ntype: X\n---\n"), 0o644))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/real.md", []byte("---\ntype: X\n---\n"), 0o644))
	testutil.SymlinkOrSkip(t, outside, dir+"/linked")
	// Act
	b, err := Load(os.DirFS(dir))
	// Assert
	require.NoError(t, err)
	assert.Len(t, b.Concepts, 1)
	assert.NotContains(t, b.Files, "linked/secret.md")
	require.Len(t, b.Problems, 1)
	assert.Equal(t, "linked", b.Problems[0].Path)
}

func FuzzSplitFrontmatter(f *testing.F) {
	for _, s := range []string{"", "---\n", "---\ntype: a\n---\nx", "---\r\n---\r\n", "\xef\xbb\xbf---\na: b\n---"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		fm, _ := SplitFrontmatter(data)
		_ = fm.Keys()
		_ = fm.Scalar("type")
	})
}

// The okf.md guide shows an index.md with title/version/entries frontmatter. It is
// the frontmatter index style, which a reader accepts next to the spec's body one.
func TestReadsGuideStyleBundle(t *testing.T) {
	b := load(t, map[string]string{
		"index.md":            "---\ntitle: My Bundle\nversion: 0.1.0\nentries:\n  - what-is-okf.md\n---\n\n# My Bundle\n\n* [What is OKF](what-is-okf.md) - intro\n",
		"what-is-okf.md":      "---\ntype: concept\ntitle: What is OKF\n---\nBody\n",
		"validation-rules.md": "---\ntype: howto\n---\nSteps\n",
	})
	require.Len(t, b.Concepts, 2)
	require.Len(t, b.Indexes["index.md"].Entries, 2, "one from the frontmatter, one from the body")
	assert.Equal(t, StyleFrontmatter, b.IndexStyle())
	got := codes(b.Validate())
	assert.NotContains(t, got, "AR9B6 index.md", "the frontmatter scheme is accepted")
	assert.Contains(t, got, "AR9B3 index.md", "and reported as info")
	assert.Contains(t, got, "AR9B0 index.md", "validation-rules.md is not listed")
	assert.NotContains(t, got, "AR9B1 what-is-okf.md")
}

func styleInputs() []IndexInput {
	return []IndexInput{
		{Path: "rules/b.md", Title: "B", Description: "second"},
		{Path: "rules/a.md", Title: "A: colon", Description: "first"},
		{Path: "skills/s/SKILL.md", Title: "S"},
	}
}

func TestBuildIndexesFrontmatterGolden(t *testing.T) {
	// Arrange
	labels := DirLabel{"rules": "the rules"}
	// Act
	out := BuildIndexes(styleInputs(), labels, StyleFrontmatter)
	// Assert
	assert.Equal(t, `---
okf_version: "0.2"
title: Index
version: 0.1.0
entries:
  - title: rules
    path: rules/index.md
    description: the rules
  - title: skills
    path: skills/index.md
---
`, string(out["index.md"]))
	assert.Equal(t, `---
title: Rules
version: 0.1.0
entries:
  - title: 'A: colon'
    path: a.md
    description: first
  - title: B
    path: b.md
    description: second
---
`, string(out["rules/index.md"]))
}

func TestBothIndexStylesAreValidAndDetected(t *testing.T) {
	tests := []struct {
		style, want string
		info        bool
	}{
		{"", StyleBody, false},
		{StyleBody, StyleBody, false},
		{StyleFrontmatter, StyleFrontmatter, true},
	}
	for _, tt := range tests {
		t.Run("style "+tt.style, func(t *testing.T) {
			// Arrange
			files := map[string]string{}
			for p, d := range BuildIndexes(styleInputs(), nil, tt.style) {
				files[p] = string(d)
			}
			for _, c := range styleInputs() {
				files[c.Path] = "---\ntype: X\ntitle: '" + c.Title + "'\n---\n"
			}
			// Act
			b := load(t, files)
			findings := b.Validate()
			// Assert
			assert.Equal(t, tt.want, b.IndexStyle())
			if tt.info {
				assert.Equal(t, []string{"AR9B3 index.md"}, codes(findings))
				assert.Equal(t, SeverityInfo, findings[0].Severity)
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestFrontmatterIndexReportsMissingEntryAndUnknownKey(t *testing.T) {
	b := load(t, map[string]string{
		"index.md": "---\ntitle: T\nversion: 0.1.0\nowner: x\nentries:\n  - title: A\n    path: a.md\n  - title: Ghost\n    path: ghost.md\n---\n",
		"a.md":     "---\ntype: X\n---\n",
		"b.md":     "---\ntype: X\n---\n",
	})
	got := b.Validate()
	assert.Contains(t, codes(got), "AR9B6 index.md")
	var ghost Finding
	for _, f := range got {
		if f.Code == CodeIndexMismatch && strings.Contains(f.Message, "Ghost") {
			ghost = f
		}
	}
	assert.Equal(t, 8, ghost.Line, "the line of the ghost entry within the file")
	assert.Contains(t, codes(got), "AR9B0 index.md", "b.md is not listed")
}
