package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const (
	noConfigNotice = "no ai-rulez config found; linting detected files"
	badMarketplace = `{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":"plugins/a"}]}`
	noDescription  = "---\nname: loose\n---\nbody\n"
	e2eV5Config    = "version = \"5.0\"\nname = \"demo\"\ngitignore = false\nagents_md = false\npresets = [\"claude\"]\n"
)

func writeLintFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// Scenario (a): validate, scan and doctor need no configuration, in a git
// repository or outside one; generate, clean and lock still do.
func TestLintWorksWithoutAConfigurationFile(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	tests := []struct {
		name     string
		files    map[string]string
		git      bool
		args     []string
		wantExit int
		wantOut  string
	}{
		{name: "empty directory is clean", args: []string{"validate", "--strict"}, wantExit: 0},
		{name: "empty git repository is clean", git: true, args: []string{"validate", "--strict"}, wantExit: 0},
		{name: "AGENTS.md alone is clean", files: map[string]string{"AGENTS.md": "# Rules\n\nRun the tests.\n"}, args: []string{"validate", "--strict"}, wantExit: 0},
		{
			name: "only a bad SKILL.md", files: map[string]string{"skills/loose/SKILL.md": noDescription},
			args: []string{"validate", "--strict"}, wantExit: 2, wantOut: "AR801",
		},
		{
			name: "only a bad SKILL.md in git", git: true, files: map[string]string{"skills/loose/SKILL.md": noDescription},
			args: []string{"validate", "--strict"}, wantExit: 2, wantOut: "AR801",
		},
		{
			name: "a secret in AGENTS.md is found by scan", files: map[string]string{"AGENTS.md": "key = ghp_abcdefghijklmnopqrstuvwxyz0123456789\n"},
			args: []string{"scan"}, wantExit: 2, wantOut: "AR001",
		},
		{
			name: "an odd-location marketplace needs --glob", files: map[string]string{"catalog/odd/registry.json": badMarketplace},
			args: []string{"validate", "--strict", "--glob", "catalog/**/*.json"}, wantExit: 2, wantOut: "AR963",
		},
		{
			name: "an odd-location marketplace is not found without it", files: map[string]string{"catalog/odd/registry.json": badMarketplace},
			args: []string{"validate", "--strict"}, wantExit: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.CreateTempDir(t)
			if tt.git {
				gitInitAgents(t, dir)
			}
			for rel, body := range tt.files {
				writeLintFile(t, dir, rel, body)
			}
			before := listDir(t, dir)

			res := testutil.RunCLI(t, dir, tt.args...)

			assert.Equal(t, tt.wantExit, res.ExitCode, "stdout:\n%s\nstderr:\n%s", res.Stdout, res.Stderr)
			assert.Contains(t, res.Stderr, noConfigNotice)
			assert.Equal(t, 1, strings.Count(res.Stderr, noConfigNotice), "the notice is printed once")
			if tt.wantOut != "" {
				assert.Contains(t, res.Stdout, tt.wantOut)
			}
			assert.Equal(t, before, listDir(t, dir), "a lint writes nothing")
		})
	}
}

func TestDoctorWorksWithoutAConfigurationFile(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeLintFile(t, dir, "AGENTS.md", "# Rules\n")

	res := testutil.RunCLI(t, dir, "doctor")

	assert.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	assert.Equal(t, 1, strings.Count(res.Stderr, noConfigNotice))
	assert.Equal(t, []string{"AGENTS.md"}, listDir(t, dir))
}

func TestOKFValidateNeedsNoConfiguration(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeLintFile(t, dir, "bundle/index.md", "---\nokf_version: \"0.2\"\n---\n# Bundle\n")

	res := testutil.RunCLI(t, dir, "okf", "validate", "bundle")

	assert.NotContains(t, res.Stdout+res.Stderr, "ai-rulez init", "okf validate never reads a configuration")
}

func TestGenerateCleanAndLockNeedAConfiguration(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	for _, args := range [][]string{{"generate"}, {"clean", "--yes"}, {"lock"}} {
		t.Run(args[0], func(t *testing.T) {
			dir := testutil.CreateTempDir(t)
			writeLintFile(t, dir, "AGENTS.md", "# Rules\n")

			res := testutil.RunCLI(t, dir, args...)

			assert.Equal(t, 1, res.ExitCode, res.Stdout+res.Stderr)
			assert.Contains(t, res.Stderr, "ai-rulez init")
			assert.Equal(t, []string{"AGENTS.md"}, listDir(t, dir), "nothing is written")
		})
	}
}

func TestValidateFromAConfiguredSubdirectoryUsesTheProjectConfig(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeLintFile(t, dir, ".ai-rulez/config.toml", e2eV5Config)
	writeLintFile(t, dir, "pkg/AGENTS.md", "# Pkg\n")

	res := testutil.RunCLI(t, filepath.Join(dir, "pkg"), "validate", "--strict")

	assert.Equal(t, 0, res.ExitCode, "a directory below a configured project validates that project, like git finds its repository: %s%s", res.Stdout, res.Stderr)
	assert.NotContains(t, res.Stderr, noConfigNotice, "the implicit lint of a project without a configuration does not run")
}

// Scenario (c): a marketplace kept in a directory an include names, outside
// .ai-rulez/, is reported once it is named.
func TestPathAndGlobLintAnIncludeRootMarketplace(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	gitInitAgents(t, dir)
	writeLintFile(t, dir, ".ai-rulez/config.toml", e2eV5Config+"\n[[includes]]\nname = \"shared\"\nsource = \"./shared\"\n")
	writeLintFile(t, dir, "shared/rules/shared-rule.md", "# Shared\n\nBe kind.\n")
	writeLintFile(t, dir, "shared/market/registry.json", badMarketplace)

	plain := testutil.RunCLI(t, dir, "validate", "--strict")
	assert.NotContains(t, plain.Stdout, "AR963", "nothing names the marketplace: %s%s", plain.Stdout, plain.Stderr)

	for _, args := range [][]string{
		{"validate", "--strict", "--path", "shared"},
		{"validate", "--strict", "--glob", "shared/**/*.json"},
		{"scan", "--path", "shared/market/registry.json", "--fail-on", "none"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		if args[0] == "scan" {
			assert.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
			continue
		}
		assert.Equal(t, 2, res.ExitCode, "%v: %s%s", args, res.Stdout, res.Stderr)
		assert.Contains(t, res.Stdout, "AR963", "%v", args)
		assert.Contains(t, res.Stdout, "shared/market/registry.json", "%v", args)
	}
}

func TestLintPathsConfigNamesFilesToLint(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeLintFile(t, dir, ".ai-rulez/config.toml", e2eV5Config+"\n[lint]\npaths = [\"catalog\"]\n")
	writeLintFile(t, dir, "catalog/registry.json", badMarketplace)

	res := testutil.RunCLI(t, dir, "validate", "--strict")

	assert.Equal(t, 2, res.ExitCode, res.Stdout+res.Stderr)
	assert.Contains(t, res.Stdout, "AR963")
}

func TestPathOutsideTheProjectIsRefused(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	other := testutil.CreateTempDir(t)
	writeLintFile(t, dir, ".ai-rulez/config.toml", e2eV5Config)
	writeLintFile(t, other, "x.md", "# x\n")

	res := testutil.RunCLI(t, dir, "validate", "--strict", "--path", filepath.Join(other, "x.md"))

	assert.Equal(t, 1, res.ExitCode, res.Stdout+res.Stderr)
	assert.Contains(t, res.Stderr, "outside the project root")
}

func TestPathFlagsAreRefusedWithUpdateBaselineAndConfigOnly(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	writeLintFile(t, dir, ".ai-rulez/config.toml", e2eV5Config)
	writeLintFile(t, dir, "x.md", "# x\n")

	for _, args := range [][]string{
		{"validate", "--strict", "--path", "x.md", "--update-baseline"},
		{"validate", "--config-only", "--path", "x.md"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, 1, res.ExitCode, "%v: %s%s", args, res.Stdout, res.Stderr)
	}
}
