package verifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return root
}

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }

func TestRun_Predicates(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"README.md":             "# Title\n",
		"src/a.go":              "package a\n// TODO(x)\n",
		"src/b.go":              "package b\n",
		"src/vendor/c.go":       "package c\n",
		"node_modules/dep/i.js": "TODO",
		"package.json":          `{"engines":{"node":">=20"},"private":true,"n":3,"list":[{"id":"z"}]}`,
		"cfg.yaml":              "a:\n  b: hello\n",
		"cfg.toml":              "[tool]\nname = \"x\"\n",
	})
	tests := []struct {
		name    string
		v       config.VerifierConfig
		want    Status
		wantMsg string
	}{
		{"file_exists pass", config.VerifierConfig{Type: "file_exists", Path: "README.md"}, StatusPass, ""},
		{"file_exists fail", config.VerifierConfig{Type: "file_exists", Path: "LICENSE"}, StatusFail, "LICENSE"},
		{"file_absent pass", config.VerifierConfig{Type: "file_absent", Path: "LICENSE"}, StatusPass, ""},
		{"file_absent fail", config.VerifierConfig{Type: "file_absent", Path: "README.md"}, StatusFail, "README.md"},
		{"glob_count in range", config.VerifierConfig{Type: "glob_count", Glob: "src/**/*.go", Min: intp(3), Max: intp(3)}, StatusPass, ""},
		{"glob_count exclude", config.VerifierConfig{Type: "glob_count", Glob: "src/**/*.go", Exclude: []string{"src/vendor/**"}, Max: intp(2)}, StatusPass, ""},
		{"glob_count too many", config.VerifierConfig{Type: "glob_count", Glob: "src/**/*.go", Max: intp(2)}, StatusFail, "3"},
		{"glob_count too few", config.VerifierConfig{Type: "glob_count", Glob: "*.txt", Min: intp(1)}, StatusFail, "0"},
		{"glob skips node_modules", config.VerifierConfig{Type: "glob_count", Glob: "**/*.js", Max: intp(0)}, StatusPass, ""},
		{"regex pass", config.VerifierConfig{Type: "regex", Glob: "src/*.go", Pattern: `(?m)^package `}, StatusPass, ""},
		{"regex fail names file", config.VerifierConfig{Type: "regex", Glob: "src/*.go", Pattern: "TODO"}, StatusFail, "src/b.go"},
		{"regex no files", config.VerifierConfig{Type: "regex", Glob: "*.rs", Pattern: "x"}, StatusFail, "no files"},
		{"forbid pass", config.VerifierConfig{Type: "forbid", Glob: "src/b.go", Pattern: "TODO"}, StatusPass, ""},
		{"forbid fail with line", config.VerifierConfig{Type: "forbid", Glob: "src/*.go", Pattern: "TODO"}, StatusFail, "src/a.go:2"},
		{"forbid no files passes", config.VerifierConfig{Type: "forbid", Glob: "*.rs", Pattern: "x"}, StatusPass, ""},
		{"key_equals json string", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "engines.node", Equals: strp(">=20")}, StatusPass, ""},
		{"key_equals json bool", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "private", Equals: strp("true")}, StatusPass, ""},
		{"key_equals json number", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "n", Equals: strp("3")}, StatusPass, ""},
		{"key_equals json index", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "list.0.id", Equals: strp("z")}, StatusPass, ""},
		{"key_equals yaml", config.VerifierConfig{Type: "key_equals", Path: "cfg.yaml", Key: "a.b", Equals: strp("hello")}, StatusPass, ""},
		{"key_equals toml", config.VerifierConfig{Type: "key_equals", Path: "cfg.toml", Key: "tool.name", Equals: strp("x")}, StatusPass, ""},
		{"key_equals mismatch", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "engines.node", Equals: strp(">=18")}, StatusFail, ">=20"},
		{"key_equals missing key", config.VerifierConfig{Type: "key_equals", Path: "package.json", Key: "engines.go", Equals: strp("1")}, StatusFail, "engines.go"},
		{"key_equals missing file", config.VerifierConfig{Type: "key_equals", Path: "nope.json", Key: "a", Equals: strp("1")}, StatusFail, "nope.json"},
		{"key_equals unsupported format", config.VerifierConfig{Type: "key_equals", Path: "README.md", Key: "a", Equals: strp("1")}, StatusFail, "format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.v.Name = "v"
			cfg := &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{tt.v}}

			rep := Run(context.Background(), cfg, Options{})

			require.Len(t, rep.Results, 1)
			res := rep.Results[0]
			assert.Equal(t, tt.want, res.Status, res.Message)
			if tt.wantMsg != "" {
				assert.Contains(t, res.Message, tt.wantMsg)
			}
		})
	}
}

func TestRun_GeneratedInSync(t *testing.T) {
	tests := []struct {
		name  string
		drift []string
		err   error
		want  Status
	}{
		{"in sync", nil, nil, StatusPass},
		{"drifted", []string{"CLAUDE.md"}, nil, StatusFail},
		{"cannot render", nil, assert.AnError, StatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{BaseDir: t.TempDir(), Verifiers: []config.VerifierConfig{{Name: "sync", Type: "generated_in_sync", Profile: "p"}}}
			var gotProfile string

			rep := Run(context.Background(), cfg, Options{Drift: func(_ *config.Config, profile string) ([]string, error) {
				gotProfile = profile
				return tt.drift, tt.err
			}})

			assert.Equal(t, tt.want, rep.Results[0].Status)
			assert.Equal(t, "p", gotProfile)
			if len(tt.drift) > 0 {
				assert.Contains(t, rep.Results[0].Message, "CLAUDE.md")
			}
		})
	}
}

func TestRun_OptionsSeverityAndOrder(t *testing.T) {
	root := writeFiles(t, map[string]string{"a": "x"})
	cfg := &config.Config{BaseDir: root, Verifiers: []config.VerifierConfig{
		{Name: "z-fail-warn", Type: "file_exists", Path: "missing", Severity: "warning"},
		{Name: "a-ok", Type: "file_exists", Path: "a"},
		{Name: "m-fail-err", Type: "file_exists", Path: "missing"},
	}}

	rep := Run(context.Background(), cfg, Options{})

	require.Len(t, rep.Results, 3)
	assert.Equal(t, []string{"z-fail-warn", "a-ok", "m-fail-err"}, []string{rep.Results[0].Name, rep.Results[1].Name, rep.Results[2].Name}, "declaration order is kept")
	assert.True(t, rep.Failed(false))

	only := Run(context.Background(), cfg, Options{Names: []string{"z-fail-warn"}})
	require.Len(t, only.Results, 1)
	assert.False(t, only.Failed(false), "a failing warning does not fail the run")
	assert.True(t, only.Failed(true), "--strict fails on warnings")

	unknown := Run(context.Background(), cfg, Options{Names: []string{"nope"}})
	assert.Error(t, unknown.Err, "an unknown --name cannot run")
}

func TestRun_UnknownTypeIsAnErrorResult(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Verifiers: []config.VerifierConfig{{Name: "c", Type: "command"}}}

	rep := Run(context.Background(), cfg, Options{})

	assert.Equal(t, StatusError, rep.Results[0].Status)
	assert.True(t, rep.CannotRun())
}

func TestEveryConfigTypeHasAPredicate(t *testing.T) {
	for _, typ := range config.VerifierTypes {
		_, ok := predicates[typ]
		assert.True(t, ok, "config type %q has no predicate", typ)
	}
}

func TestRegisterExtendsPredicates(t *testing.T) {
	Register("custom_test_type", func(context.Context, *Env, config.VerifierConfig) (Outcome, error) {
		return Outcome{Pass: true}, nil
	})
	t.Cleanup(func() { delete(predicates, "custom_test_type") })
	cfg := &config.Config{BaseDir: t.TempDir(), Verifiers: []config.VerifierConfig{{Name: "c", Type: "custom_test_type"}}}

	rep := Run(context.Background(), cfg, Options{})

	assert.Equal(t, StatusPass, rep.Results[0].Status)
}

func TestRun_FileTraversalIsContained(t *testing.T) {
	root := writeFiles(t, map[string]string{"in/a.txt": "x"})
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(root), "outside.txt"), []byte("s"), 0o644))
	t.Cleanup(func() { _ = os.Remove(filepath.Join(filepath.Dir(root), "outside.txt")) })
	cfg := &config.Config{BaseDir: filepath.Join(root, "in"), Verifiers: []config.VerifierConfig{
		{Name: "t", Type: "file_exists", Path: "../../outside.txt"},
	}}

	rep := Run(context.Background(), cfg, Options{})

	assert.Equal(t, StatusError, rep.Results[0].Status, "a path escaping the root is refused, not read")
}

func TestWriteText_And_JSON(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Verifiers: []config.VerifierConfig{
		{Name: "ok", Type: "file_absent", Path: "nothing"},
		{Name: "bad", Type: "file_exists", Path: "nothing", Description: "must exist"},
	}}
	rep := Run(context.Background(), cfg, Options{})

	var text bytes.Buffer
	require.NoError(t, WriteText(&text, rep))
	assert.Contains(t, text.String(), "STATUS")
	assert.Contains(t, text.String(), "fail")
	assert.Contains(t, text.String(), "1 passed, 1 failed")

	var js bytes.Buffer
	require.NoError(t, WriteJSON(&js, rep))
	var decoded struct {
		Summary map[string]int `json:"summary"`
		Results []Result       `json:"results"`
	}
	require.NoError(t, json.Unmarshal(js.Bytes(), &decoded))
	assert.Equal(t, 1, decoded.Summary["pass"])
	assert.Equal(t, 1, decoded.Summary["fail"])
	assert.Len(t, decoded.Results, 2)
}
