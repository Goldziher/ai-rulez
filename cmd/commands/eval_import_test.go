package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/evals/evalimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetImportFlags(t *testing.T) {
	t.Helper()
	prevDir := configDir
	reset := func() {
		evalImportFlags.from, evalImportFlags.skill, evalImportFlags.out = "tessl", "", ""
		evalImportFlags.dryRun, evalImportFlags.liftAssertions, evalImportFlags.force = false, false, false
		evalImportFlags.rubricMode, evalImportFlags.report, evalImportFlags.format = evalimport.RubricSingle, "", formatText
		configDir = prevDir
	}
	reset()
	t.Cleanup(reset)
}

// tesslScenario writes a scenario directory and returns it.
func tesslScenario(t *testing.T, criteria string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "criteria.json"), []byte(criteria), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task.md"), []byte("Deploy the billing service to staging.\n"), 0o600))
	return dir
}

const tesslCriteria = `{"scenario":"deploy-billing","criteria":[{"name":"deploys","description":"The file \"deploy.log\" exists","weight":2},{"name":"clear","description":"Explains what it did","weight":1}],"pass_threshold":70,"repeats":3}`

func TestEvalImport_WritesIntoTheSkillsEvalsDirectoryAndTheCasesLoad(t *testing.T) {
	// Arrange
	resetImportFlags(t)
	root := evalProject(t)
	scenario := tesslScenario(t, tesslCriteria)
	evalImportFlags.skill, evalImportFlags.liftAssertions = "deploy", true
	var out bytes.Buffer
	evalImportCmd.SetOut(&out)

	// Act
	err := runEvalImport(evalImportCmd, []string{scenario})

	// Assert
	require.NoError(t, err)
	evalsDir := filepath.Join(root, ".ai-rulez", "skills", "deploy", "evals")
	assert.FileExists(t, filepath.Join(evalsDir, "deploy-billing.eval.yaml"))
	assert.FileExists(t, filepath.Join(evalsDir, "deploy-billing.task.md"))
	assert.Contains(t, out.String(), "wrote deploy-billing.eval.yaml")
	assert.Contains(t, out.String(), "AR9A5")
	assert.Contains(t, out.String(), "use `eval run --runs N`")
	assert.Contains(t, out.String(), `lifted:   criterion "deploys"`)
	skills, err := evals.FindSkills(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	var deploy evals.Skill
	for _, s := range skills {
		if s.ID == "deploy" {
			deploy = s
		}
	}
	cases, problems := evals.LoadCases(&deploy)
	require.Empty(t, problems, "the project's own case loader (the AR996 check) accepts the imported file")
	ids := map[string]bool{}
	for _, c := range cases {
		ids[c.ID] = true
	}
	assert.True(t, ids["deploy-billing"])
}

func TestEvalImport_JSONFormatAndReportFile(t *testing.T) {
	resetImportFlags(t)
	evalProject(t)
	scenario := tesslScenario(t, tesslCriteria)
	reportPath := filepath.Join(t.TempDir(), "report.json")
	evalImportFlags.out, evalImportFlags.format, evalImportFlags.report = t.TempDir(), formatJSON, reportPath
	var out bytes.Buffer
	evalImportCmd.SetOut(&out)

	err := runEvalImport(evalImportCmd, []string{scenario})

	require.NoError(t, err)
	var stdout, file evalimport.Result
	require.NoError(t, json.Unmarshal(out.Bytes(), &stdout))
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &file))
	assert.Equal(t, stdout.Reports[0].Scenario, file.Reports[0].Scenario)
	require.Len(t, file.Findings, 1)
	assert.Equal(t, "AR9A5", file.Findings[0].Code)
}

func TestEvalImport_DryRunWritesNothing(t *testing.T) {
	resetImportFlags(t)
	evalProject(t)
	scenario := tesslScenario(t, tesslCriteria)
	dest := t.TempDir()
	evalImportFlags.out, evalImportFlags.dryRun = dest, true
	var out bytes.Buffer
	evalImportCmd.SetOut(&out)

	require.NoError(t, runEvalImport(evalImportCmd, []string{scenario}))

	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Contains(t, out.String(), "would write")
}

func TestEvalImport_RubricItemsMode(t *testing.T) {
	resetImportFlags(t)
	evalProject(t)
	scenario := tesslScenario(t, tesslCriteria)
	dest := t.TempDir()
	evalImportFlags.out, evalImportFlags.rubricMode = dest, evalimport.RubricItems
	evalImportCmd.SetOut(&bytes.Buffer{})

	require.NoError(t, runEvalImport(evalImportCmd, []string{scenario}))

	data, err := os.ReadFile(filepath.Join(dest, "deploy-billing.eval.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "rubric_items:")
	assert.NotContains(t, string(data), "weighted checklist")
}

func TestEvalImport_Errors(t *testing.T) {
	scenario := func(t *testing.T) string { return tesslScenario(t, tesslCriteria) }
	tests := []struct {
		name    string
		set     func(t *testing.T)
		args    func(t *testing.T) []string
		wantErr string
	}{
		{"unknown format", func(*testing.T) { evalImportFlags.from = "notion" }, func(t *testing.T) []string { return []string{scenario(t)} }, "unknown or missing --from"},
		{"bad rubric mode", func(*testing.T) { evalImportFlags.rubricMode = "wild" }, func(t *testing.T) []string { return []string{scenario(t)} }, "unknown --rubric-mode"},
		{"bad output format", func(*testing.T) { evalImportFlags.format = "xml" }, func(t *testing.T) []string { return []string{scenario(t)} }, "unknown --format"},
		{"nowhere to write", func(*testing.T) {}, func(t *testing.T) []string { return []string{scenario(t)} }, "nowhere to write"},
		{"unknown skill", func(*testing.T) { evalImportFlags.skill = "nope" }, func(t *testing.T) []string { return []string{scenario(t)} }, "unknown skill"},
		{"not a scenario", func(t *testing.T) { evalImportFlags.out = t.TempDir() }, func(t *testing.T) []string { return []string{t.TempDir()} }, "does not look like a tessl scenario"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetImportFlags(t)
			evalProject(t)
			tt.set(t)

			err := runEvalImport(evalImportCmd, tt.args(t))

			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEvalImport_RefusesToOverwriteWithoutForce(t *testing.T) {
	resetImportFlags(t)
	evalProject(t)
	scenario := tesslScenario(t, tesslCriteria)
	dest := t.TempDir()
	evalImportFlags.out = dest
	evalImportCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, runEvalImport(evalImportCmd, []string{scenario}))

	err := runEvalImport(evalImportCmd, []string{scenario})
	assert.ErrorContains(t, err, "already exist")

	evalImportFlags.force = true
	assert.NoError(t, runEvalImport(evalImportCmd, []string{scenario}))
}
