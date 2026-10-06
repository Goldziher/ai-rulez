package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const minimalRubric = `schema_version = 1
id = "mine"
version = 2
applies_to = ["skill"]

[[dimension]]
id = "trigger-quality"
code = "AR9G1"
group = "intrinsic"
weight = 0.6
severity = "warning"
twins = ["AR801"]
question = "q"
pass = "p"
warn = "w"
fail = "f"

[[dimension]]
id = "body-structure"
code = "AR9G7"
group = "intrinsic"
weight = 0.4
severity = "info"
twins = ["AR805"]
question = "q"
pass = "p"
warn = "w"
fail = "f"
`

func writeRubric(t *testing.T, root, id, body string) string {
	t.Helper()
	dir := filepath.Join(root, RubricsDir, id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, RubricFile), []byte(body), 0o644))
	return dir
}

func messages(ps []Problem) string {
	var parts []string
	for _, p := range ps {
		parts = append(parts, p.Message)
	}
	return strings.Join(parts, "\n")
}

func TestBuiltinRubricIsValid(t *testing.T) {
	// Arrange
	ids := BuiltinIDs()

	// Act / Assert
	require.Contains(t, ids, "skill-quality")
	for _, id := range ids {
		problems, err := LintBuiltin(id)
		require.NoError(t, err)
		assert.Empty(t, problems, "built-in rubric %s: %s", id, messages(problems))
		rb, err := LoadBuiltin(id)
		require.NoError(t, err)
		assert.Equal(t, "builtin:"+id, rb.Ref)
		assert.True(t, strings.HasPrefix(rb.Digest, "sha256:"))
	}
}

func TestLintDir(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"valid", func(s string) string { return s }, ""},
		{"weights must sum to one", func(s string) string { return strings.Replace(s, "weight = 0.6", "weight = 0.5", 1) }, "weights sum to 0.900"},
		{"id must match directory", func(s string) string { return strings.Replace(s, `id = "mine"`, `id = "other"`, 1) }, `must equal the directory name "mine"`},
		{"unknown twin", func(s string) string { return strings.Replace(s, `["AR801"]`, `["AR999"]`, 1) }, `twin "AR999" is not a registered lint rule code`},
		{"unknown dimension code", func(s string) string { return strings.Replace(s, `code = "AR9G1"`, `code = "AR9G9"`, 1) }, "must be one of AR9G1-AR9G7"},
		{"duplicate code", func(s string) string { return strings.Replace(s, `code = "AR9G7"`, `code = "AR9G1"`, 1) }, "reuses code AR9G1"},
		{"error severity refused", func(s string) string { return strings.Replace(s, `severity = "warning"`, `severity = "error"`, 1) }, "never reports an error"},
		{"bad group", func(s string) string { return strings.Replace(s, `group = "intrinsic"`, `group = "other"`, 1) }, "group must be"},
		{"unknown key", func(s string) string { return s + "\nsurprise = 1\n" }, "surprise"},
		{"wrong schema version", func(s string) string { return strings.Replace(s, "schema_version = 1", "schema_version = 2", 1) }, "schema_version must be 1"},
		{"unknown kind", func(s string) string { return strings.Replace(s, `["skill"]`, `["skill", "widget"]`, 1) }, `unknown kind "widget"`},
		{"syntax error", func(s string) string { return "id = = 1\n" }, "parse rubric"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := writeRubric(t, t.TempDir(), "mine", tt.mutate(minimalRubric))

			// Act
			problems, err := LintDir(dir)

			// Assert
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, problems, messages(problems))
				return
			}
			assert.Contains(t, messages(problems), tt.want)
		})
	}
}

func TestLintDirGoldenAndCalibration(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"valid golden", map[string]string{"golden/a.golden.yaml": goldenOK}, ""},
		{"one labeler", map[string]string{"golden/a.golden.yaml": strings.Replace(goldenOK, "  - {id: b, labels: {trigger-quality: warn}}\n", "", 1)}, "at least two labelers"},
		{"unknown dimension", map[string]string{"golden/a.golden.yaml": strings.ReplaceAll(goldenOK, "trigger-quality", "nope")}, `unknown dimension "nope"`},
		{"escaping path", map[string]string{"golden/a.golden.yaml": strings.Replace(goldenOK, "fixtures/a/SKILL.md", "../a/SKILL.md", 1)}, "relative path without .."},
		{"unknown probe", map[string]string{"golden/a.golden.yaml": strings.Replace(goldenOK, "[pad]", "[bogus]", 1)}, `unknown probe "bogus"`},
		{"unknown yaml key", map[string]string{"golden/a.golden.yaml": goldenOK + "extra: 1\n"}, "does not parse"},
		{"calibration wrong rubric", map[string]string{"calibration.json": `{"schema_version":1,"rubric":{"id":"x"},"status":"pass"}`}, `calibration is for rubric "x"`},
		{"calibration bad json", map[string]string{"calibration.json": `{`}, "does not parse"},
		{"calibration ok", map[string]string{"calibration.json": `{"schema_version":1,"rubric":{"id":"mine"},"status":"pass"}`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := writeRubric(t, t.TempDir(), "mine", minimalRubric)
			for name, body := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
			}

			// Act
			problems, err := LintDir(dir)

			// Assert
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, problems, messages(problems))
				return
			}
			assert.Contains(t, messages(problems), tt.want)
		})
	}
}

const goldenOK = `schema_version: 1
id: vague-01
kind: skill
item: {path: fixtures/a/SKILL.md}
labelers:
  - {id: a, labels: {trigger-quality: fail}}
  - {id: b, labels: {trigger-quality: warn}}
adjudicated: {trigger-quality: fail}
probes: [pad]
`

func TestLintDirRefusesSymlinks(t *testing.T) {
	// Arrange
	root := t.TempDir()
	real := writeRubric(t, root, "real", minimalRubric)
	link := filepath.Join(root, RubricsDir, "mine")
	testutil.SymlinkOrSkip(t, real, link)

	// Act
	problems, err := LintDir(link)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, messages(problems), "not a symlink")

	// A symlinked rubric.toml is refused as well.
	dir := writeRubric(t, root, "linked", minimalRubric)
	require.NoError(t, os.Remove(filepath.Join(dir, RubricFile)))
	testutil.SymlinkOrSkip(t, filepath.Join(real, RubricFile), filepath.Join(dir, RubricFile))
	problems, err = LintDir(dir)
	require.NoError(t, err)
	assert.Contains(t, messages(problems), "symlink")
}

func TestLoad(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeRubric(t, root, "mine", minimalRubric)
	writeRubric(t, root, "broken", strings.Replace(strings.Replace(minimalRubric, "weight = 0.6", "weight = 0.1", 1), `id = "mine"`, `id = "broken"`, 1))
	tests := []struct {
		name    string
		ref     string
		wantRef string
		wantErr string
	}{
		{"default is builtin", "", "builtin:skill-quality", ""},
		{"project rubric", "mine", "mine", ""},
		{"invalid refused", "broken", "", `rubric "broken" is invalid: dimension weights sum to 0.500`},
		{"unknown builtin", "builtin:nope", "", "unknown built-in rubric"},
		{"missing project rubric", "absent", "", "absent"},
		{"path traversal refused", "../x", "", "lowercase letters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			rb, err := Load(filepath.Join(root), tt.ref)

			// Assert
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRef, rb.Ref)
		})
	}
}

func TestDigestChangesWithGoldenFiles(t *testing.T) {
	// Arrange
	dir := writeRubric(t, t.TempDir(), "mine", minimalRubric)
	before, _, err := loadDir(dir)
	require.NoError(t, err)

	// Act
	require.NoError(t, os.MkdirAll(filepath.Join(dir, GoldenDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, GoldenDir, "a.golden.yaml"), []byte(goldenOK), 0o644))
	after, _, err := loadDir(dir)
	require.NoError(t, err)

	// Assert
	assert.NotEqual(t, before.Digest, after.Digest)
}
