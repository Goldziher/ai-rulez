package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const pickedLeak = "# Notes\n\nkey = ghp_abcdefghijklmnopqrstuvwxyz0123456789\n"

func recursiveWithPicks(t *testing.T, paths, globs []string) (code int, doc lint.Combined) {
	t.Helper()
	root := twoRoots(t, validRootConfig)
	writeFile(t, filepath.Join(root, "docs", "AGENTS.md"), pickedLeak)
	writeFile(t, filepath.Join(root, "a", "inside.md"), "# fine\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	strictTreeCache = &lint.Loader{}
	t.Cleanup(func() { strictTreeCache = &lint.Loader{} })
	resetValidateFlags(t)
	t.Setenv(repoRootEnv, "")
	viper.Set("quiet", false)
	t.Cleanup(func() { viper.Set("quiet", false) })
	setTargetFlags(t, paths, globs)
	validateRecursive, validateFormat, validateStrict = true, "json", true

	stdout, _ := capture(t, func() { code = runRecursiveValidate() })

	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), "stdout: %q", stdout)
	return code, doc
}

func TestRecursiveLintsAPickOutsideEveryRoot(t *testing.T) {
	tests := []struct {
		name         string
		paths, globs []string
	}{
		{name: "--path file", paths: []string{"docs/AGENTS.md"}},
		{name: "--glob", globs: []string{"docs/*.md"}},
		{name: "--path directory", paths: []string{"docs"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, doc := recursiveWithPicks(t, tc.paths, tc.globs)

			assert.Equal(t, exitStrictFindings, code, "a secret in a picked file outside every root must fail the run")
			var files []string
			for _, f := range doc.Findings {
				if f.Code == lint.CodeSecretDetected {
					files = append(files, f.File)
				}
			}
			assert.Equal(t, []string{"docs/AGENTS.md"}, files)
			assert.Contains(t, doc.Notices, "docs/AGENTS.md: outside every --recursive root, linted at the repository root")
		})
	}
}

func TestRecursiveStaysQuietWhenARootCoversThePick(t *testing.T) {
	code, doc := recursiveWithPicks(t, []string{"a/inside.md"}, nil)

	assert.Zero(t, code)
	assert.Empty(t, doc.Notices)
	assert.Empty(t, doc.Findings)
}
