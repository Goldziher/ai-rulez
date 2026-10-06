package importer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resourceOf(t *testing.T, p *Plan, skill, rel string) string {
	t.Helper()
	for i := range p.Items {
		if p.Items[i].Kind == KindSkill && p.Items[i].Name == skill {
			for _, r := range p.Items[i].Resources {
				if r.Path == rel {
					return string(r.Data)
				}
			}
		}
	}
	t.Fatalf("skill %s has no resource %s", skill, rel)
	return ""
}

const tesslSkill = "---\nname: %s\ndescription: A skill\n---\nBody.\n"

func tesslSkillFile(name string) string { return strings.Replace(tesslSkill, "%s", name, 1) }

func TestTesslPlan_EvalScenariosBecomeValidEvalFiles(t *testing.T) {
	// Arrange
	root := ".tessl/plugins/acme/tools/"
	fsys := mapFS(map[string]string{
		"tessl.json":                       `{"mode":"vendored","dependencies":{"acme/tools":{"version":"1.0.0"}}}`,
		root + "skills/deploy/SKILL.md":    tesslSkillFile("deploy"),
		root + "evals/basic/task.md":       "Deploy the app: it has a `key: value` and a trailing colon:\n\n- step one\n",
		root + "evals/basic/criteria.json": `{"context":"Deploys correctly.","checklist":[{"name":"runs","description":"Runs the deploy","max_score":10},"Reports success"]}`,
	})
	// Act
	p := planOf(t, tesslImporter{}, fsys, Options{})
	// Assert
	data := resourceOf(t, p, "deploy", "evals/basic.eval.yaml")
	cases, problems := evals.ParseFile("basic.eval.yaml", []byte(data))
	require.Empty(t, problems, "%v", problems)
	require.Len(t, cases, 1)
	c := cases[0]
	assert.Equal(t, "basic", c.ID)
	require.NotNil(t, c.ExpectTrigger)
	assert.True(t, *c.ExpectTrigger)
	assert.Contains(t, c.Prompt, "Deploy the app")
	assert.Contains(t, c.Rubric, "Deploys correctly.")
	assert.Contains(t, c.Rubric, "- runs: Runs the deploy")
	assert.Contains(t, c.Rubric, "- Reports success")
	assert.NotNil(t, findingFor(p, StatusApproximated, root+"evals/basic/criteria.json", "checklist"), "dropped weights are reported")
}

func TestTesslPlan_EvalsAttachToTheRightSkill(t *testing.T) {
	tests := []struct {
		name      string
		skills    []string
		wantSkill string
		wantNote  bool
	}{
		{name: "the only skill", skills: []string{"alpha"}, wantSkill: "alpha"},
		{name: "the skill named like the plugin", skills: []string{"alpha", "tools", "zed"}, wantSkill: "tools"},
		{name: "the first skill otherwise", skills: []string{"zed", "alpha"}, wantSkill: "alpha", wantNote: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := ".tessl/plugins/acme/tools/"
			files := map[string]string{root + "evals/e/task.md": "Do it.\n"}
			for _, s := range tt.skills {
				files[root+"skills/"+s+"/SKILL.md"] = tesslSkillFile(s)
			}
			// Act
			p := planOf(t, tesslImporter{}, mapFS(files), Options{})
			// Assert
			assert.NotEmpty(t, resourceOf(t, p, tt.wantSkill, "evals/e.eval.yaml"))
			var noted bool
			for _, f := range p.Findings {
				noted = noted || (f.Status == StatusApproximated && strings.Contains(f.Reason, "attached to the first"))
			}
			assert.Equal(t, tt.wantNote, noted)
		})
	}
}

func TestTesslPlan_EvalsWithoutASkillNeedAction(t *testing.T) {
	root := ".tessl/plugins/acme/rules-only/"
	p := planOf(t, tesslImporter{}, mapFS(map[string]string{
		root + "rules/r.md":        "Rule.\n",
		root + "evals/e/task.md":   "Do it.\n",
		root + "evals/e/other.txt": "x",
	}), Options{})

	assert.NotNil(t, findingFor(p, StatusNeedsAction, root+"evals", ""))
	assert.Equal(t, []string{"rules/r.md"}, itemRels(p))
}

func TestTesslPlan_ManifestHandling(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		check func(t *testing.T, p *Plan)
	}{
		{
			name: "managed plugins that are not on disk need action",
			files: map[string]string{
				"tessl.json": `{"mode":"managed","dependencies":{"acme/one":"1.0.0","acme/two":{"version":"2.0.0"}},"verify":true,"extra":1}`,
			},
			check: func(t *testing.T, p *Plan) {
				assert.NotNil(t, findingFor(p, StatusNeedsAction, "tessl.json", "dependencies.acme/one"))
				assert.NotNil(t, findingFor(p, StatusNeedsAction, "tessl.json", "dependencies.acme/two"))
				assert.NotNil(t, findingFor(p, StatusUnsupported, "tessl.json", "verify"))
				assert.NotNil(t, findingFor(p, StatusDropped, "tessl.json", "extra"))
				assert.Empty(t, p.Items)
			},
		},
		{
			name: "an unknown mode is reported",
			files: map[string]string{
				"tessl.json": `{"mode":"cloud"}`,
			},
			check: func(t *testing.T, p *Plan) { assert.NotNil(t, findingFor(p, StatusUnsupported, "tessl.json", "mode")) },
		},
		{
			name: "a versioned directory is found by the version tessl.json names",
			files: map[string]string{
				"tessl.json": `{"dependencies":{"acme/v":{"version":"1.2.0"}}}`,
				".tessl/plugins/acme/v/1.1.0/skills/old/SKILL.md": tesslSkillFile("old"),
				".tessl/plugins/acme/v/1.2.0/skills/new/SKILL.md": tesslSkillFile("new"),
			},
			check: func(t *testing.T, p *Plan) { assert.Equal(t, []string{"skills/new/SKILL.md"}, itemRels(p)) },
		},
		{
			name: "a plugin that is not listed is still imported",
			files: map[string]string{
				".tessl/plugins/acme/unlisted/skills/s/SKILL.md": tesslSkillFile("s"),
			},
			check: func(t *testing.T, p *Plan) { assert.Equal(t, []string{"skills/s/SKILL.md"}, itemRels(p)) },
		},
		{
			name: "a directory without plugin content is unsupported",
			files: map[string]string{
				".tessl/plugins/acme/empty/notes.txt": "x",
			},
			check: func(t *testing.T, p *Plan) {
				assert.NotNil(t, findingFor(p, StatusUnsupported, ".tessl/plugins/acme/empty", ""))
			},
		},
		{
			name: "the project can itself be a plugin",
			files: map[string]string{
				".tessl-plugin/plugin.json": `{"name":"me"}`,
				"skills/mine/SKILL.md":      tesslSkillFile("mine"),
				"rules/r.md":                "Rule.\n",
				"docs/guide.md":             "Docs.\n",
				"main.go":                   "package main\n",
			},
			check: func(t *testing.T, p *Plan) {
				assert.Equal(t, []string{"rules/r.md", "skills/mine/SKILL.md"}, itemRels(p))
				assert.NotNil(t, findingFor(p, StatusDropped, "docs", ""))
				assert.Nil(t, findingFor(p, StatusDropped, "main.go", ""), "the project's own files are not reported")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planOf(t, tesslImporter{}, mapFS(tt.files), Options{})
			tt.check(t, p)
		})
	}
}

func TestTesslPlan_InvalidManifestIsAnError(t *testing.T) {
	_, err := tesslImporter{}.Plan(mapFS(map[string]string{"tessl.json": "{not json"}), Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeInvalid)
}

func TestTesslPlan_CriteriaForms(t *testing.T) {
	tests := []struct {
		name       string
		criteria   string
		wantRubric string
		wantFinds  []Status
	}{
		{name: "bare list", criteria: `["First","Second"]`, wantRubric: "The answer should satisfy these criteria:\n- First\n- Second"},
		{name: "another criteria type is read as a checklist", criteria: `{"type":"binary","checklist":["A"]}`, wantRubric: "The answer should satisfy these criteria:\n- A", wantFinds: []Status{StatusApproximated}},
		{name: "not criteria", criteria: `42`, wantFinds: []Status{StatusUnsupported}},
		{name: "context only", criteria: `{"context":"Just context."}`, wantRubric: "Just context."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := ".tessl/plugins/acme/p/"
			p := planOf(t, tesslImporter{}, mapFS(map[string]string{
				root + "skills/s/SKILL.md":     tesslSkillFile("s"),
				root + "evals/e/task.md":       "Task.\n",
				root + "evals/e/criteria.json": tt.criteria,
			}), Options{})
			// Act
			data := resourceOf(t, p, "s", "evals/e.eval.yaml")
			cases, problems := evals.ParseFile("e.eval.yaml", []byte(data))
			// Assert
			require.Empty(t, problems)
			require.Len(t, cases, 1)
			assert.Equal(t, tt.wantRubric, strings.TrimSpace(cases[0].Rubric))
			for _, s := range tt.wantFinds {
				assert.NotNil(t, findingFor(p, s, root+"evals/e/criteria.json", ""), "want a %s finding", s)
			}
		})
	}
}

func TestTesslPlan_ScenarioProblems(t *testing.T) {
	root := ".tessl/plugins/acme/p/"
	p := planOf(t, tesslImporter{}, mapFS(map[string]string{
		root + "skills/s/SKILL.md":       tesslSkillFile("s"),
		root + "evals/no-task/notes.txt": "x",
		root + "evals/loose.md":          "not a scenario directory",
	}), Options{})

	assert.NotNil(t, findingFor(p, StatusDropped, root+"evals/no-task", ""))
	assert.NotNil(t, findingFor(p, StatusDropped, root+"evals/loose.md", ""))
	for i := range p.Items {
		assert.Empty(t, p.Items[i].Resources, "no case is written for a scenario without a task")
	}
}

func TestConvert_TesslEndToEnd(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	root := ".tessl/plugins/acme/tools/"
	writeTree(t, dir, map[string]string{
		"tessl.json":                       `{"mode":"vendored","dependencies":{"acme/tools":{"version":"1.0.0"}}}`,
		root + "skills/deploy/SKILL.md":    tesslSkillFile("deploy"),
		root + "evals/basic/task.md":       "Deploy.\n",
		root + "evals/basic/criteria.json": `["Deploys"]`,
		"AGENTS.md":                        "<!-- tessl-managed-start -->\n@.tessl/RULES.md\n<!-- tessl-managed-end -->\n",
	})
	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})
	// Assert
	require.NoError(t, err)
	require.True(t, report.Written, "%+v", report.Security)
	assert.Equal(t, "tessl", report.Importer, "native is skipped: AGENTS.md is Tessl's output")
	got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
	assert.Contains(t, got, "skills/deploy/SKILL.md")
	assert.Contains(t, got, "skills/deploy/evals/basic.eval.yaml")
}

func TestRenderEvalFile_RefusesAnEmptyTask(t *testing.T) {
	_, err := renderEvalFile(evalCase{id: "x", description: "d"})

	require.Error(t, err)
}

func TestRenderEvalFile_KeepsHostileTextAsData(t *testing.T) {
	// Arrange: YAML syntax inside the prompt must stay text.
	prompt := "key: value\n- item\n---\n&anchor *alias !!tag\n  indented\n"
	// Act
	data, err := renderEvalFile(evalCase{id: "Hostile Name!", description: "d: e", prompt: prompt, rubric: "# not a comment"})
	require.NoError(t, err)
	cases, problems := evals.ParseFile("h.eval.yaml", data)
	// Assert
	require.Empty(t, problems)
	require.Len(t, cases, 1)
	assert.Equal(t, "hostile-name-", cases[0].ID)
	assert.Equal(t, strings.TrimSpace(prompt), strings.TrimSpace(cases[0].Prompt))
	assert.Equal(t, "# not a comment", cases[0].Rubric)
}
