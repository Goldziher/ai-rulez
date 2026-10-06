package evals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const validCases = `
schema_version: 1
cases:
  - id: deploy-basic
    prompt: Deploy the service to staging
    expect_trigger: true
    near_miss:
      - Explain how staging environments differ from production
    files:
      - path: app/config.yaml
        content: "env: staging"
    assertions:
      - type: contains
        value: staging
      - type: regex
        value: 'deployed \d+ services'
      - type: file_exists
        path: deploy.log
      - type: command_exit
        command: test -f deploy.log
        exit_code: 0
    rubric: The answer names the staging cluster.
    rubric_min_score: 0.8
    model: haiku
    tags: [smoke, deploy]
  - id: unrelated
    prompt: What is the capital of France?
    expect_trigger: false
`

func TestParseFile_ValidMultiCase(t *testing.T) {
	cases, problems := ParseFile("x.eval.yaml", []byte(validCases))
	require.Empty(t, problems)
	require.Len(t, cases, 2)
	assert.Equal(t, "deploy-basic", cases[0].ID)
	assert.True(t, cases[0].Expects())
	assert.False(t, cases[1].Expects())
	assert.Equal(t, 4, cases[0].Line)
	assert.Equal(t, 26, cases[1].Line)
	assert.Len(t, cases[0].Assertions, 4)
	assert.InDelta(t, 0.8, *cases[0].RubricMinScore, 1e-9)
}

func TestParseFile_SingleTopLevelCaseDefaultsID(t *testing.T) {
	cases, problems := ParseFile("/tmp/Trigger-Basic.eval.yaml", []byte("prompt: hi\nexpect_trigger: true\n"))
	require.Empty(t, problems)
	require.Len(t, cases, 1)
	assert.Equal(t, "trigger-basic", cases[0].ID)
}

func TestParseFile_JSON(t *testing.T) {
	cases, problems := ParseFile("c.eval.json", []byte(`{"cases":[{"id":"a","prompt":"p","expect_trigger":false}]}`))
	require.Empty(t, problems)
	require.Len(t, cases, 1)

	_, problems = ParseFile("c.eval.json", []byte(`{"cases":[{"id":"a","prompt":"p","expect_trigger":false,"bogus":1}]}`))
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "invalid JSON")

	_, problems = ParseFile("c.eval.json", []byte("{\n\"cases\": [\n"))
	require.Len(t, problems, 1)
	assert.Equal(t, 3, problems[0].Line)
}

func TestParseFile_Problems(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing expect_trigger", "cases:\n  - id: a\n    prompt: p\n", "expect_trigger is required"},
		{"missing prompt", "cases:\n  - id: a\n    expect_trigger: true\n", "one of prompt and prompt_file"},
		{"both prompts", "cases:\n  - id: a\n    prompt: p\n    prompt_file: p.md\n    expect_trigger: true\n", "mutually exclusive"},
		{"bad id", "cases:\n  - id: Bad Id\n    prompt: p\n    expect_trigger: true\n", "must be lowercase"},
		{"unknown field", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    nope: 1\n", "field nope not found"},
		{"bad regex", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    assertions:\n      - type: regex\n        value: '('\n", "not a valid regular expression"},
		{"unknown assertion", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    assertions:\n      - type: equals\n        value: x\n", "unknown type"},
		{"contains needs value", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    assertions:\n      - type: contains\n", "value is required"},
		{"command needs command", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    assertions:\n      - type: command_exit\n", "command is required"},
		{"absolute fixture", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    files:\n      - path: /etc/passwd\n", "must be relative"},
		{"escaping fixture", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    files:\n      - path: ../x\n", "must stay inside"},
		{"near miss on negative", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: false\n    near_miss: [q]\n", "near_miss only makes sense"},
		{"rubric score without rubric", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    rubric_min_score: 0.5\n", "needs a rubric"},
		{"bad tag", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n    tags: [Bad Tag]\n", "tags[0]"},
		{"duplicate id", "cases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n  - id: a\n    prompt: q\n    expect_trigger: true\n", "duplicate case id"},
		{"empty", "", "file is empty"},
		{"no cases", "schema_version: 1\n", "holds no cases"},
		{"newer version", "schema_version: 9\ncases: []\n", "unsupported schema_version"},
		{"both forms", "prompt: x\nexpect_trigger: true\ncases:\n  - id: a\n    prompt: p\n    expect_trigger: true\n", "not both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, problems := ParseFile("c.eval.yaml", []byte(tt.body))
			require.NotEmpty(t, problems)
			var all []string
			for _, p := range problems {
				all = append(all, p.Message)
			}
			assert.Contains(t, strings.Join(all, "\n"), tt.want)
		})
	}
}

func TestExpand_NearMissBecomesNegativeCase(t *testing.T) {
	cases, problems := ParseFile("x.eval.yaml", []byte(validCases))
	require.Empty(t, problems)
	expanded := Expand(cases)
	require.Len(t, expanded, 3)
	assert.Equal(t, "deploy-basic", expanded[0].ID)
	assert.Nil(t, expanded[0].NearMiss)
	nm := expanded[1]
	assert.Equal(t, "deploy-basic.near-miss-1", nm.ID)
	assert.False(t, nm.Expects())
	assert.Equal(t, "deploy-basic", nm.NearMissOf)
	assert.Contains(t, nm.Tags, NearMissTag)
	assert.Equal(t, "unrelated", expanded[2].ID)
}

func TestLoadCases_ResolvesFilesAndRejectsEscapes(t *testing.T) {
	cfg := t.TempDir()
	skillDir := filepath.Join(cfg, "skills", "deploy")
	evalDir := filepath.Join(skillDir, "evals")
	require.NoError(t, os.MkdirAll(filepath.Join(evalDir, "prompts"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: deploy\n---\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "prompts", "p.md"), []byte("deploy it\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "fixture.txt"), []byte("fixture body"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "native.yaml"), []byte("not: an eval case"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "a.eval.yaml"), []byte(
		"cases:\n  - id: a\n    prompt_file: prompts/p.md\n    expect_trigger: true\n    files:\n      - path: f.txt\n        source: fixture.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "b.eval.yaml"), []byte(
		"cases:\n  - id: b\n    prompt_file: ../../SKILL.md\n    expect_trigger: true\n"), 0o600))

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	cases, problems := LoadCases(&skills[0])
	require.Len(t, cases, 2)
	assert.Equal(t, "deploy it", cases[0].Prompt)
	assert.Empty(t, cases[0].PromptFile)
	assert.Equal(t, "fixture body", cases[0].Files[0].Content)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "must stay inside")
}

func TestLoadCases_ProjectLevelDirAndDuplicateIDs(t *testing.T) {
	cfg := t.TempDir()
	skillDir := filepath.Join(cfg, "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "evals"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, "evals", "deploy"), 0o750))
	one := "prompt: hi\nexpect_trigger: true\nid: same\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "evals", "one.eval.yaml"), []byte(one), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cfg, "evals", "deploy", "two.eval.yaml"), []byte(one), 0o600))

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	cases, problems := LoadCases(&skills[0])
	assert.Len(t, cases, 2)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "also used in")
}

func TestDigests_SkillDigestIgnoresEvals(t *testing.T) {
	cfg := t.TempDir()
	skillDir := filepath.Join(cfg, "skills", "s")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "evals"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("v1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "evals", "a.eval.yaml"), []byte("c1"), 0o600))

	d1, err := SkillDigest(skillDir)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(d1, "sha256:"))

	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "evals", "a.eval.yaml"), []byte("c2"), 0o600))
	d2, err := SkillDigest(skillDir)
	require.NoError(t, err)
	assert.Equal(t, d1, d2, "editing a case must not change the skill digest")

	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("v2"), 0o600))
	d3, err := SkillDigest(skillDir)
	require.NoError(t, err)
	assert.NotEqual(t, d1, d3)

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	c1, err := CasesDigest(&skills[0])
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "evals", "a.eval.yaml"), []byte("c3"), 0o600))
	c2, err := CasesDigest(&skills[0])
	require.NoError(t, err)
	assert.NotEqual(t, c1, c2)
}

func TestLoadCases_RejectsSymlinkedEscapes(t *testing.T) {
	cfg := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOP SECRET"), 0o600))
	skillDir := filepath.Join(cfg, "skills", "deploy")
	evalDir := filepath.Join(skillDir, "evals")
	require.NoError(t, os.MkdirAll(evalDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("x"), 0o600))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(evalDir, "link"))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "ok.txt"), []byte("fine"), 0o600))
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.txt"), filepath.Join(evalDir, "file-link.txt"))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "a.eval.yaml"), []byte(
		"cases:\n  - id: a\n    prompt_file: link/secret.txt\n    expect_trigger: true\n    files:\n      - path: f.txt\n        source: link/secret.txt\n"+
			"  - id: b\n    prompt_file: file-link.txt\n    expect_trigger: true\n"+
			"  - id: c\n    prompt_file: /etc/passwd\n    expect_trigger: true\n"), 0o600))

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	cases, problems := LoadCases(&skills[0])
	require.NotEmpty(t, problems)
	for i := range cases {
		assert.NotContains(t, cases[i].Prompt, "TOP SECRET")
		for _, f := range cases[i].Files {
			assert.NotContains(t, f.Content, "TOP SECRET")
		}
	}
	var text []string
	for _, p := range problems {
		text = append(text, p.Message)
	}
	assert.Contains(t, strings.Join(text, "\n"), "outside")
}

func TestFindSkills_RejectsEvalDirThatEscapesTheProject(t *testing.T) {
	cfg := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOP SECRET"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "a.eval.yaml"), []byte(
		"cases:\n  - id: a\n    prompt_file: secret.txt\n    expect_trigger: true\n"), 0o600))
	skillDir := filepath.Join(cfg, "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("x"), 0o600))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(skillDir, "evals"))

	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	cases, problems := LoadCases(&skills[0])
	assert.Empty(t, cases)
	require.NotEmpty(t, problems)
	assert.Contains(t, problems[0].Message, "outside")
}

func TestFileAssertions_DoNotFollowSymlinksOutOfTheWorkDir(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("hunter2"), 0o600))
	testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.txt"), filepath.Join(work, "out.txt"))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(work, "dir"))
	require.NoError(t, os.WriteFile(filepath.Join(work, "real.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(work, "big.txt"), make([]byte, maxCaseFileBytes+1), 0o600))

	r := &Result{WorkDir: work}
	check := func(a Assertion) string { return checkAssertion(&a, r, GradeOptions{}) }

	assert.Empty(t, check(Assertion{Type: AssertContains, Path: "real.txt", Value: "hello"}))
	assert.NotEmpty(t, check(Assertion{Type: AssertContains, Path: "out.txt", Value: "hunter2"}), "a symlink to a host file is not read")
	assert.NotEmpty(t, check(Assertion{Type: AssertContains, Path: "dir/secret.txt", Value: "hunter2"}))
	assert.NotEmpty(t, check(Assertion{Type: AssertFileExists, Path: "out.txt"}), "existence of host files is not leaked")
	assert.NotEmpty(t, check(Assertion{Type: AssertFileExists, Path: "dir/secret.txt"}))
	assert.NotEmpty(t, check(Assertion{Type: AssertContains, Path: "big.txt", Value: "x"}), "oversized files are refused")
	exists := false
	assert.Empty(t, check(Assertion{Type: AssertFileExists, Path: "missing.txt", Exists: &exists}))
}

func TestDigests_IgnoreOSJunkAndResultsButNotSymlinkTargets(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "deploy", "body", twoCases)
	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	skill := &skills[0]
	skillBefore, err := SkillDigest(skill.Dir)
	require.NoError(t, err)
	casesBefore, err := CasesDigest(skill)
	require.NoError(t, err)

	evalDir := filepath.Join(skill.Dir, "evals")
	for _, junk := range []string{filepath.Join(skill.Dir, ".DS_Store"), filepath.Join(evalDir, ".DS_Store"), filepath.Join(evalDir, "._main.eval.yaml")} {
		require.NoError(t, os.WriteFile(junk, []byte("junk"), 0o600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(evalDir, "results"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(evalDir, "results", "run.json"), []byte("{}"), 0o600))
	skillAfter, err := SkillDigest(skill.Dir)
	require.NoError(t, err)
	casesAfter, err := CasesDigest(skill)
	require.NoError(t, err)
	assert.Equal(t, skillBefore, skillAfter)
	assert.Equal(t, casesBefore, casesAfter)

	// a symlink counts by its target, so retargeting it is a change
	testutil.SymlinkOrSkip(t, "SKILL.md", filepath.Join(skill.Dir, "alias.md"))
	linked, err := SkillDigest(skill.Dir)
	require.NoError(t, err)
	assert.NotEqual(t, skillBefore, linked)
	require.NoError(t, os.Remove(filepath.Join(skill.Dir, "alias.md")))
	testutil.SymlinkOrSkip(t, "other.md", filepath.Join(skill.Dir, "alias.md"))
	retargeted, err := SkillDigest(skill.Dir)
	require.NoError(t, err)
	assert.NotEqual(t, linked, retargeted)
}

func TestLoadCases_NearMissIDCollidingWithAnAuthoredCaseIsAProblem(t *testing.T) {
	cfg := t.TempDir()
	writeSkill(t, cfg, "deploy", "x", "cases:\n  - id: deploy\n    prompt: p\n    expect_trigger: true\n    near_miss: [something close]\n"+
		"  - id: deploy.near-miss-1\n    prompt: q\n    expect_trigger: false\n")
	skills, err := FindSkills(cfg)
	require.NoError(t, err)
	_, problems := LoadCases(&skills[0])
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "deploy.near-miss-1")
}
