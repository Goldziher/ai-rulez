package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strictProject writes a minimal project (files are relative to the project
// root) and loads its configuration. The first return is the project root.
func strictProject(t *testing.T, configExtra string, files map[string]string) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), validRootConfig+configExtra)
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root, loadStrictProject(t, root)
}

func loadStrictProject(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

func lintProject(t *testing.T, cfg *config.Config) *lint.Report {
	t.Helper()
	strictTreeCache = lint.Loader{}
	report, err := strictLint(cfg)
	require.NoError(t, err)
	return report
}

const brokenLinkRule = "---\ndescription: a rule\n---\n# Title\n\nSee [the guide](docs/missing.md) now.\n"

func TestCheckStrictFlagsFormatsAndOutput(t *testing.T) {
	oldS, oldF, oldO := validateStrict, validateFormat, validateOutput
	t.Cleanup(func() { validateStrict, validateFormat, validateOutput = oldS, oldF, oldO })
	for _, f := range []string{"", "text", "json", "sarif", "github", "junit", "markdown"} {
		validateStrict, validateFormat, validateOutput = true, f, ""
		assert.NoError(t, checkStrictFlags(), f)
	}
	validateFormat = "xml"
	assert.Error(t, checkStrictFlags())
	validateStrict, validateFormat, validateOutput = false, "", "out.sarif"
	assert.Error(t, checkStrictFlags(), "--output needs --strict")
}

func TestWriteReportToOutputFile(t *testing.T) {
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	oldF, oldO := validateFormat, validateOutput
	t.Cleanup(func() { validateFormat, validateOutput = oldF, oldO })
	validateFormat, validateOutput = "sarif", filepath.Join(root, "out", "r.sarif")

	report := lintProject(t, cfg)
	require.NoError(t, writeReport(lint.Combine([]*lint.Report{report}), "error"))

	data, err := os.ReadFile(validateOutput)
	require.NoError(t, err)
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "2.1.0", doc.Version)
	require.NotEmpty(t, doc.Runs[0].Results)
	assert.Equal(t, "AR201", doc.Runs[0].Results[0].RuleID)
}

func TestFingerprintSurvivesLineMoves(t *testing.T) {
	root, cfg := strictProject(t, "", map[string]string{".ai-rulez/rules/a.md": brokenLinkRule})
	before := lintProject(t, cfg).Findings
	require.Len(t, before, 1)

	moved := strings.Replace(brokenLinkRule, "# Title\n", "# Title\n\nAn inserted paragraph.\n\nAnd another.\n", 1)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "a.md"), moved)
	after := lintProject(t, loadStrictProject(t, root)).Findings
	require.Len(t, after, 1)

	assert.NotEqual(t, before[0].Line, after[0].Line, "the line must have moved")
	assert.Equal(t, before[0].Fingerprint(), after[0].Fingerprint())
	assert.Equal(t, ".ai-rulez/rules/a.md", after[0].RepoPath(), "paths are relative to the repository root")
}
