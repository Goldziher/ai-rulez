package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManifestReviewFixes(t *testing.T) {
	body := skillDoc("", "x\n")
	dir := ".ai-rulez/skills/bad/scripts/"
	with := func(name, content string, extra ...string) map[string]string {
		m := map[string]string{dir + name: content}
		for _, e := range extra {
			m[dir+e] = "{}\n"
		}
		return m
	}
	info := map[string]Severity{"AR054": SeverityInfo}
	const sha = "0123456789abcdef0123456789abcdef01234567"
	runRuleCases(t, []ruleCase{
		{name: "pinned git with an environment marker", skill: body, files: with("requirements.txt", "repo @ git+https://github.com/org/repo.git@"+sha+" ; python_version >= \"3.8\"\n"), absent: []string{"AR054"}},
		{name: "pinned git in pyproject with a marker", skill: body, files: with("pyproject.toml", "[project]\ndependencies = [\"repo @ git+https://github.com/org/repo.git@"+sha+" ; python_version >= '3.8'\"]\n"), absent: []string{"AR054"}},
		{name: "unpinned git with a marker still warns", skill: body, files: with("requirements.txt", "repo @ git+https://github.com/org/repo.git@main ; python_version >= \"3.8\"\n"), want: []string{"AR054:requirements.txt:1"}},
		{name: "scoped npmrc registry is the confusion defense", skill: body, files: with(".npmrc", "@myorg:registry=https://npm.pkg.github.com\n"), absent: []string{"AR055"}},
		{name: "unscoped npmrc registry still warns", skill: body, files: with(".npmrc", "registry=https://npm.example.net/\n"), want: []string{"AR055:.npmrc:1"}},
		{name: "scoped yarnrc registry", skill: body, files: with(".yarnrc", "\"@myorg:registry\" \"https://npm.pkg.github.com\"\n"), absent: []string{"AR055"}},
		{name: "scoped yarnrc.yml registry", skill: body, files: with(".yarnrc.yml", "npmScopes:\n  myorg:\n    npmRegistryServer: \"https://npm.pkg.github.com\"\n"), absent: []string{"AR055"}},
		{name: "unscoped yarnrc.yml registry still warns", skill: body, files: with(".yarnrc.yml", "npmRegistryServer: \"https://npm.example.net\"\n"), want: []string{"AR055:.yarnrc.yml:1"}},
		{name: "scoped registry option of an install command", skill: body, files: with("setup.sh", "npm install --@myorg:registry=https://npm.pkg.github.com left-pad@1.3.0\n"), absent: []string{"AR055"}},
		{name: "malformed package.json is not analyzable", skill: body, files: with("package.json", "{not json"), want: []string{"AR054:package.json:1"}, sev: info},
		{name: "malformed pyproject is not analyzable", skill: body, files: with("pyproject.toml", "[project\n"), want: []string{"AR054:pyproject.toml:1"}, sev: info},
		{name: "package.json beside package-lock.json", skill: body, files: with("package.json", "{\"dependencies\": {\"left-pad\": \"^1.3.0\", \"b\": \"latest\"}}", "package-lock.json"), absent: []string{"AR054"}},
		{name: "package.json beside pnpm-lock.yaml", skill: body, files: with("package.json", "{\"dependencies\": {\"b\": \"*\"}}", "pnpm-lock.yaml"), absent: []string{"AR054"}},
		{name: "pyproject beside uv.lock", skill: body, files: with("pyproject.toml", "[project]\ndependencies = [\"requests\"]\n", "uv.lock"), absent: []string{"AR054"}},
		{name: "pyproject beside poetry.lock", skill: body, files: with("pyproject.toml", "[tool.poetry.dependencies]\nrequests = \"*\"\n", "poetry.lock"), absent: []string{"AR054"}},
		{name: "package.json with no lock still reports", skill: body, files: with("package.json", "{\"dependencies\": {\"b\": \"*\"}}"), want: []string{"AR054:package.json:1"}},
		{name: "build backend requirement reports at info", skill: body, files: with("pyproject.toml", "[build-system]\nrequires = [\"hatchling\"]\n"), want: []string{"AR054:pyproject.toml:2"}, sev: info},
	})
}

func TestPinMessageNamesTheEcosystem(t *testing.T) {
	npm := requirement{name: "left-pad", level: pinNone}
	_, msg, _ := pinFinding(npm, true)
	assert.NotContains(t, msg, "==")
	assert.Contains(t, msg, "exact version")
	_, msg, _ = pinFinding(npm, false)
	assert.Contains(t, msg, "==")
}
