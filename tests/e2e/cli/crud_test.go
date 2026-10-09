package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const crudConfig = "version = \"5.0\"\nname = \"crud\"\npresets = [\"claude\"]\n"

func crudProject(t *testing.T) string {
	t.Helper()
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(crudConfig), 0o644))
	return dir
}

func decodeDoc(t *testing.T, res *testutil.CLIResult) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &doc), "stdout %q stderr %q", res.Stdout, res.Stderr)
	assert.EqualValues(t, 1, doc["schema_version"])
	return doc
}

// A change reports what it created or removed on stdout, in text as a bare path
// and under --format json as a versioned document; -q does not hide it.
func TestCRUDChangesReportPathsOnStdout(t *testing.T) {
	dir := crudProject(t)

	text := testutil.RunCLI(t, dir, "-q", "add", "rule", "style")
	assert.Equal(t, 0, text.ExitCode, text.Stderr)
	assert.Equal(t, filepath.Join(".ai-rulez", "rules", "style.md")+"\n", text.Stdout)
	assert.Empty(t, text.Stderr, "-q hides the confirmation, not the path")

	for _, tc := range []struct {
		args []string
		want map[string]any
	}{
		{[]string{"add", "context", "arch", "--format", "json"}, map[string]any{"status": "created", "type": "context", "name": "arch", "path": filepath.Join(".ai-rulez", "context", "arch.md")}},
		{[]string{"add", "agent", "helper", "--format", "json"}, map[string]any{"status": "created", "type": "agent", "name": "helper"}},
		{[]string{"add", "command", "deploy", "--format", "json"}, map[string]any{"status": "created", "type": "command", "name": "deploy"}},
		{[]string{"add", "check", "sec", "--format", "json"}, map[string]any{"status": "created", "type": "check", "name": "sec"}},
		{[]string{"add", "skill", "sk", "--format", "json"}, map[string]any{"status": "created", "type": "skill", "name": "sk", "path": filepath.Join(".ai-rulez", "skills", "sk", "SKILL.md")}},
		{[]string{"domain", "add", "backend", "--format", "json"}, map[string]any{"status": "created", "type": "domain", "name": "backend", "path": filepath.Join(".ai-rulez", "domains", "backend")}},
		{[]string{"profile", "add", "api", "backend", "--format", "json"}, map[string]any{"status": "created", "type": "profile", "name": "api", "path": filepath.Join(".ai-rulez", "config.toml")}},
		{[]string{"profile", "set-default", "api", "--format", "json"}, map[string]any{"status": "updated", "type": "profile", "name": "api", "default": true}},
		{[]string{"remove", "rule", "style", "--yes", "--format", "json"}, map[string]any{"status": "removed", "type": "rule", "name": "style", "path": filepath.Join(".ai-rulez", "rules", "style.md")}},
		{[]string{"remove", "skill", "sk", "--yes", "--format", "json"}, map[string]any{"status": "removed", "type": "skill", "name": "sk", "path": filepath.Join(".ai-rulez", "skills", "sk")}},
	} {
		res := testutil.RunCLI(t, dir, tc.args...)
		require.Equal(t, 0, res.ExitCode, "%v: %s", tc.args, res.Stderr)
		doc := decodeDoc(t, res)
		for k, v := range tc.want {
			assert.Equal(t, v, doc[k], "%v: %s", tc.args, k)
		}
	}
}

func TestCRUDIncludeAndInstalledSkillJSON(t *testing.T) {
	dir := crudProject(t)
	src := filepath.Join(dir, "shared")
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".ai-rulez", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".ai-rulez", "config.toml"), []byte(crudConfig), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".ai-rulez", "rules", "x.md"), []byte("# x\n"), 0o644))

	add := decodeDoc(t, testutil.RunCLI(t, dir, "include", "add", "shared", src, "--format", "json"))
	assert.Equal(t, "created", add["status"])
	assert.Equal(t, "include", add["type"])
	assert.Equal(t, filepath.Join(".ai-rulez", "config.toml"), add["path"])

	rm := decodeDoc(t, testutil.RunCLI(t, dir, "include", "remove", "shared", "--yes", "--format", "json"))
	assert.Equal(t, "removed", rm["status"])
}

// A name that does not exist fails before anything is asked.
func TestRemoveChecksExistenceBeforeConfirming(t *testing.T) {
	dir := crudProject(t)

	res := testutil.RunCLI(t, dir, "remove", "rule", "nosuch")

	assert.Equal(t, exitCannot, res.ExitCode)
	assert.Contains(t, res.Stderr, "not found")
	assert.NotContains(t, res.Stderr, "Are you sure")
	assert.NotContains(t, res.Stderr, "--yes", "no confirmation was due")
	assert.Empty(t, res.Stdout)
}

func TestAddValidatesNamesTargetsAndPriority(t *testing.T) {
	dir := crudProject(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"add", "rule", "x.md"}, ".md"},
		{[]string{"add", "rule", "a b"}, "whitespace"},
		{[]string{"add", "rule", ".hidden"}, "must not start"},
		{[]string{"add", "rule", "r", "--targets", "claude,bogus"}, "bogus"},
		{[]string{"add", "rule", "r", "--priority", "urgent"}, "minimal"},
		{[]string{"add", "check", "c", "--severity", "huge"}, "severity"},
	} {
		res := testutil.RunCLI(t, dir, tc.args...)
		assert.Equal(t, exitCannot, res.ExitCode, "%v", tc.args)
		assert.Contains(t, res.Stderr, tc.want, "%v", tc.args)
	}
	ok := testutil.RunCLI(t, dir, "add", "rule", "low-one", "--priority", "minimal")
	assert.Equal(t, 0, ok.ExitCode, ok.Stderr)
}

func TestAddCheckFlagsLandInFrontmatter(t *testing.T) {
	dir := crudProject(t)

	res := testutil.RunCLI(t, dir, "add", "check", "sec", "--severity", "high", "--tools", "a,b", "--targets", "claude")

	require.Equal(t, 0, res.ExitCode, res.Stderr)
	body := testutil.ReadFile(t, filepath.Join(dir, ".ai-rulez", "checks", "sec.md"))
	assert.Contains(t, body, "severity: high")
	assert.Contains(t, body, "- claude")
	assert.Contains(t, body, "- a")
}

func TestAddSkillScaffoldValidatesClean(t *testing.T) {
	dir := crudProject(t)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "add", "skill", "sk1").ExitCode)

	res := testutil.RunCLI(t, dir, "validate", "--fail-on", "warning")

	assert.Equal(t, 0, res.ExitCode, "stdout %s stderr %s", res.Stdout, res.Stderr)
}

func TestDomainRemoveRefusesWhileAProfileUsesIt(t *testing.T) {
	dir := crudProject(t)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "domain", "add", "d1").ExitCode)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "profile", "add", "p1", "d1").ExitCode)

	res := testutil.RunCLI(t, dir, "domain", "remove", "d1", "--yes")

	assert.Equal(t, exitCannot, res.ExitCode)
	assert.Contains(t, res.Stderr, "p1")
	assert.DirExists(t, filepath.Join(dir, ".ai-rulez", "domains", "d1"))
}

// Every group prints its help and exits 0 when no subcommand is given.
func TestGroupsWithoutASubcommandPrintHelp(t *testing.T) {
	dir := crudProject(t)
	for _, group := range []string{"list", "add", "remove", "domain", "profile", "include", "skill", "builtins", "show", "edit", "migrate"} {
		res := testutil.RunCLI(t, dir, group)
		assert.Equal(t, 0, res.ExitCode, "%s: %s", group, res.Stderr)
		assert.Contains(t, res.Stdout, "Usage:", group)
		assert.Contains(t, res.Stdout, "ai-rulez "+group+" [command]", group)
	}
}

func TestShowAndEdit(t *testing.T) {
	dir := crudProject(t)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "add", "rule", "style", "--content", "Use tabs.").ExitCode)

	show := testutil.RunCLI(t, dir, "show", "rule", "style")
	assert.Equal(t, 0, show.ExitCode, show.Stderr)
	assert.Contains(t, show.Stdout, "Use tabs.")

	edit := testutil.RunCLI(t, dir, "edit", "rule", "style", "--content", "Use spaces.", "--priority", "high", "--format", "json")
	require.Equal(t, 0, edit.ExitCode, edit.Stderr)
	assert.Equal(t, "updated", decodeDoc(t, edit)["status"])

	js := decodeDoc(t, testutil.RunCLI(t, dir, "show", "rule", "style", "--format", "json"))
	assert.Contains(t, js["content"], "Use spaces.")
	assert.Equal(t, filepath.Join(".ai-rulez", "rules", "style.md"), js["path"])

	assert.Equal(t, exitCannot, testutil.RunCLI(t, dir, "edit", "rule", "style").ExitCode, "nothing to change")
	missing := testutil.RunCLI(t, dir, "show", "rule", "nosuch")
	assert.Equal(t, exitCannot, missing.ExitCode)
	assert.Contains(t, missing.Stderr, "not found")
}

func TestEditCheckSetsFrontmatterFields(t *testing.T) {
	dir := crudProject(t)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "add", "check", "sec").ExitCode)

	res := testutil.RunCLI(t, dir, "edit", "check", "sec", "--severity", "critical")

	require.Equal(t, 0, res.ExitCode, res.Stderr)
	assert.Contains(t, testutil.ReadFile(t, filepath.Join(dir, ".ai-rulez", "checks", "sec.md")), "severity: critical")
}

// The global --config-dir and -C pick the project the CRUD commands act on.
func TestCRUDHonoursGlobalConfigDirAndC(t *testing.T) {
	dir := crudProject(t)
	alt := filepath.Join(dir, "team-rules")
	require.NoError(t, os.MkdirAll(alt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(alt, "config.toml"), []byte(crudConfig), 0o644))

	byName := testutil.RunCLI(t, dir, "--config-dir", "team-rules", "add", "rule", "viaflag")
	require.Equal(t, 0, byName.ExitCode, byName.Stderr)
	assert.Equal(t, filepath.Join("team-rules", "rules", "viaflag.md")+"\n", byName.Stdout)
	assert.FileExists(t, filepath.Join(alt, "rules", "viaflag.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "rules", "viaflag.md"))

	list := testutil.RunCLI(t, dir, "--config-dir", "team-rules", "list", "rules", "--format", "json")
	assert.Contains(t, list.Stdout, "viaflag")

	elsewhere := testutil.CreateTempDir(t)
	byC := testutil.RunCLI(t, elsewhere, "-C", alt, "add", "rule", "viac")
	require.Equal(t, 0, byC.ExitCode, byC.Stderr)
	assert.FileExists(t, filepath.Join(alt, "rules", "viac.md"))

	prof := testutil.RunCLI(t, dir, "--config-dir", "team-rules", "domain", "add", "d")
	require.Equal(t, 0, prof.ExitCode, prof.Stderr)
	assert.DirExists(t, filepath.Join(alt, "domains", "d"))
	p := testutil.RunCLI(t, dir, "--config-dir", "team-rules", "profile", "add", "pp", "d")
	require.Equal(t, 0, p.ExitCode, p.Stderr)
	assert.Contains(t, testutil.ReadFile(t, filepath.Join(alt, "config.toml")), "pp")
	assert.NotContains(t, testutil.ReadFile(t, filepath.Join(dir, ".ai-rulez", "config.toml")), "pp")
}

func TestBuiltinsShowUnknownNamesTheValidOnes(t *testing.T) {
	dir := crudProject(t)

	res := testutil.RunCLI(t, dir, "builtins", "show", "nosuch")

	assert.Equal(t, exitCannot, res.ExitCode)
	assert.Contains(t, strings.ToLower(res.Stderr), "unknown builtin")
	assert.Contains(t, res.Stderr, "builtins list")
}

func TestChangeDocumentsMatchTheirSchema(t *testing.T) {
	dir := crudProject(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "change-result.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)

	for _, args := range [][]string{
		{"add", "rule", "r", "--format", "json"},
		{"domain", "add", "d", "--format", "json"},
		{"profile", "add", "p", "d", "--format", "json"},
		{"remove", "rule", "r", "--yes", "--format", "json"},
	} {
		res := testutil.RunCLI(t, dir, args...)
		if res.ExitCode != 0 {
			continue
		}
		var v any
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &v))
		result := compiled.Validate(v)
		assert.True(t, result.IsValid(), "%v: %v\n%s", args, result.Errors, res.Stdout)
	}
}
