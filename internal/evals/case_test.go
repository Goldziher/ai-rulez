package evals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
