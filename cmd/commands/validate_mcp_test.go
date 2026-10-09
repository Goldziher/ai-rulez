package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
)

// shortDescriptionProject has a skill whose description is below the minimum
// length: a warning (AR802) that the CLI's `validate` reports.
func shortDescriptionProject(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	skillDir := filepath.Join(cfgDir, "skills", "short")
	require.NoError(t, os.MkdirAll(skillDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: short\ndescription: hi\n---\n# Short\n"), 0o600))
	cfg, err := project.Load(t.Context(), dir)
	require.NoError(t, err)
	return cfg
}

// validate_config must report what `ai-rulez validate` reports: the same
// findings, judged against the same fail-on threshold.
func TestMCPValidator_RunsTheCLILint(t *testing.T) {
	cfg := shortDescriptionProject(t)
	v := &mcpValidator{}

	t.Run("a warning is reported and passes the default threshold", func(t *testing.T) {
		outcome, err := v.validate(t.Context(), cfg, handlers.ValidateParams{})

		require.NoError(t, err)
		var codes []string
		for _, f := range outcome.Report.Findings {
			codes = append(codes, f.Code)
		}
		assert.Contains(t, codes, lint.CodeDescriptionLength)
		assert.False(t, outcome.Failed, "warnings do not fail the default fail-on error")
		assert.Equal(t, "error", outcome.FailOn)
	})
	t.Run("fail_on warning fails on the same finding", func(t *testing.T) {
		outcome, err := v.validate(t.Context(), cfg, handlers.ValidateParams{FailOn: "warning"})

		require.NoError(t, err)
		assert.True(t, outcome.Failed)
		assert.Equal(t, "warning", outcome.FailOn)
	})
	t.Run("strict is fail_on warning", func(t *testing.T) {
		outcome, err := v.validate(t.Context(), cfg, handlers.ValidateParams{Strict: true})

		require.NoError(t, err)
		assert.True(t, outcome.Failed)
	})
	t.Run("fail_on none never fails", func(t *testing.T) {
		outcome, err := v.validate(t.Context(), cfg, handlers.ValidateParams{FailOn: "none"})

		require.NoError(t, err)
		assert.False(t, outcome.Failed)
	})
	t.Run("config_only skips the content checks", func(t *testing.T) {
		outcome, err := v.validate(t.Context(), cfg, handlers.ValidateParams{ConfigOnly: true})

		require.NoError(t, err)
		assert.Empty(t, outcome.Report.Findings)
	})
	t.Run("unknown selectors are rejected like the flags", func(t *testing.T) {
		for name, p := range map[string]handlers.ValidateParams{
			"fail_on":      {FailOn: "loud"},
			"lint_profile": {LintProfile: "nope"},
			"analyzer":     {Analyzers: []string{"nope"}},
			"strict+none":  {Strict: true, FailOn: "none"},
		} {
			_, err := v.validate(t.Context(), cfg, p)
			assert.Error(t, err, name)
		}
	})
	t.Run("the command's flag state is restored", func(t *testing.T) {
		_, err := v.validate(t.Context(), cfg, handlers.ValidateParams{FailOn: "warning", LintProfile: "strict", Analyzers: []string{"security"}})

		require.NoError(t, err)
		assert.Empty(t, validateFailOn)
		assert.Empty(t, validateLintProfile)
		assert.Empty(t, validateAnalyzers)
	})
}
