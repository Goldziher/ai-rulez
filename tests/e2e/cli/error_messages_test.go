package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const errorsBaseConfig = "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\ngitignore = false\n"

func errorsProject(t *testing.T, extra string) string {
	t.Helper()
	dir := testutil.CreateTempDir(t)
	writeIn(t, dir+"/.ai-rulez", "config.toml", errorsBaseConfig+extra)
	writeIn(t, dir+"/.ai-rulez/rules", "style.md", "# Style\n\nUse tabs.\n")
	writeIn(t, dir+"/.ai-rulez/skills/deploy", "SKILL.md", "---\nname: deploy\ndescription: Deploy the service. Use when releasing it.\n---\nDeploy.\n")
	return dir
}

func runErr(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	res := testutil.RunCLIWithEnv(t, dir, isolatedEnv(t), args...)
	return res.ExitCode, res.Stdout + res.Stderr
}

func TestCLI_MissingArgumentErrorsShowTheUsage(t *testing.T) {
	dir := errorsProject(t, "")
	for _, tc := range []struct {
		args  []string
		usage string
	}{
		{[]string{"add", "rule"}, "ai-rulez add rule"},
		{[]string{"domain", "add"}, "ai-rulez domain add"},
		{[]string{"migrate"}, "ai-rulez migrate"},
	} {
		code, out := runErr(t, dir, tc.args...)
		assert.Equal(t, 1, code, tc.args)
		assert.NotContains(t, out, "arg(s)", tc.args)
		assert.Contains(t, out, "missing", tc.args)
		assert.Contains(t, out, "Usage:", tc.args)
		assert.Contains(t, out, tc.usage, tc.args)
	}
}

func TestCLI_UnknownCommandSuggestsAndPointsAtHelp(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "genrate")

	assert.Equal(t, 1, code)
	assert.Contains(t, out, `unknown command "genrate"`)
	assert.Contains(t, out, "generate", "a did-you-mean")
	assert.Contains(t, out, "ai-rulez --help")

	code, out = runErr(t, dir, "add", "rulez")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "rule", "a did-you-mean for a subcommand")
}

func TestCLI_MissingConfigPathIsClear(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "-C", "/nope/x.toml", "validate")

	assert.Equal(t, 1, code)
	assert.NotContains(t, out, "stat config path")
	assert.Contains(t, out, "/nope/x.toml")
	assert.Contains(t, out, "does not exist")
}

func TestCLI_EvalRunUnknownSkillListsTheAvailableOnes(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "eval", "run", "nope")

	assert.NotEqual(t, 0, code)
	assert.Contains(t, out, `unknown skill "nope"`)
	assert.Contains(t, out, "deploy", "the skills that exist")
}

func TestCLI_RemoveMissingRuleNamesThePath(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "remove", "rule", "nope", "--yes")

	assert.Equal(t, 1, code)
	assert.Contains(t, out, "nope")
	assert.Contains(t, out, ".ai-rulez/rules", "the path that was searched")
	assert.Contains(t, out, "style", "the rules that exist")
}

func TestCLI_UnknownProfileWithNoneDeclaredSaysSo(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "generate", "--profile", "nope")

	assert.Equal(t, 1, code)
	assert.Contains(t, out, "profile not found: nope")
	assert.Contains(t, out, "No profiles are declared")
	assert.NotContains(t, out, "[]")
}

func TestCLI_RolesExtendsTypeErrorDoesNotLeakGoTypes(t *testing.T) {
	dir := errorsProject(t, "\n[[roles]]\nname = \"a\"\nextends = [\"b\"]\n")

	code, out := runErr(t, dir, "validate")

	assert.Equal(t, 1, code)
	assert.NotContains(t, out, "RoleConfig")
	assert.NotContains(t, out, "struct field")
	assert.Contains(t, out, "roles.extends")
	assert.Contains(t, out, "must be a string")
	assert.Contains(t, out, "line")
	assert.NotContains(t, out, "missing quotes around strings", "the generic syntax hint does not fit a type error")
}

func TestCLI_ListWithoutAnArgumentIsAUsageError(t *testing.T) {
	dir := errorsProject(t, "")

	code, out := runErr(t, dir, "list")

	assert.Equal(t, 1, code)
	assert.Contains(t, out, "what to list")
	assert.Contains(t, out, "ai-rulez list")
	require.True(t, strings.Contains(out, "rules"))
}
