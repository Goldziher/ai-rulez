package lint

import "testing"

func TestDeadCommandsAR403(t *testing.T) {
	build := map[string]string{
		"package.json":   `{"name":"x","scripts":{"build":"tsc","test:unit":"vitest"}}`,
		"Makefile":       ".PHONY: lint test\nlint:\n\techo lint\ntest build: deps\n\techo t\n",
		"Taskfile.yml":   "version: '3'\ntasks:\n  gen:\n    cmds: [echo]\n",
		"justfile":       "default:\n  just --list\nfmt target='x':\n  echo\nalias f := fmt\nversion := \"1\"\n",
		"pyproject.toml": "[tool.pytest.ini_options]\nmarkers = [\"slow: slow tests\", \"integration\"]\n",
	}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range build {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	body := func(s string) string { return skillDoc("", s) }
	runRuleCases(t, []ruleCase{
		{name: "undefined npm script", files: build, skill: body("Run `npm run deploy` then `npm run build`.\n"), want: []string{"AR403:SKILL.md:5"}},
		{name: "defined npm scripts", files: build, skill: body("Run `npm run build` and `pnpm run test:unit`.\n"), absent: []string{"AR403"}},
		{name: "make target", files: build, skill: body("Use `make release` here.\n"), want: []string{"AR403:SKILL.md:5"}},
		{name: "make targets defined", files: build, skill: body("Use `make lint`, `make test` or `make -j4 build`.\n"), absent: []string{"AR403"}},
		{name: "make with -C is out of scope", files: build, skill: body("`make -C sub release`\n"), absent: []string{"AR403"}},
		{name: "task", files: build, skill: body("`task gen` or `task nope`\n"), want: []string{"AR403:SKILL.md:5"}},
		{name: "just", files: build, skill: body("`just fmt`, `just f`, `just --list`, `just missing`\n"), want: []string{"AR403:SKILL.md:5"}},
		{name: "pytest marker", files: build, skill: body("`pytest -m slow` and `pytest -m \"not integration\"` but `pytest -m flaky`\n"), want: []string{"AR403:SKILL.md:5"}},
		{name: "pytest markers defined or builtin", files: build, skill: body("`pytest -m \"slow and not integration\"` `pytest -m skip`\n"), absent: []string{"AR403"}},
		{
			name: "marker used in a test file counts", files: with(map[string]string{"tests/test_a.py": "import pytest\n\n@pytest.mark.flaky\ndef test_a(): pass\n"}),
			skill: body("`pytest -m flaky`\n"), absent: []string{"AR403"},
		},
		{name: "fenced blocks are not read", files: build, skill: body("```sh\nnpm run deploy\nmake release\n```\n"), absent: []string{"AR403"}},
		{name: "placeholders are skipped", files: build, skill: body("`npm run <script>` `make $TARGET` `task {name}`\n"), absent: []string{"AR403"}},
		{name: "workspace scoped", files: build, skill: body("`npm run deploy --workspace web` `npm --prefix web run deploy`\n"), absent: []string{"AR403"}},
		{name: "no manifest at all", skill: body("`npm run deploy` `make release` `task x` `just y` `pytest -m z`\n"), absent: []string{"AR403"}},
		{name: "dynamic Makefile is skipped", files: map[string]string{"Makefile": "include common.mk\nall:\n\techo\n"}, skill: body("`make anything`\n"), absent: []string{"AR403"}},
		{name: "inline ignore", files: build, skill: body("<!-- ai-rulez-lint-ignore: AR403 -->\n`npm run deploy`\n"), absent: []string{"AR403"}},
		{name: "severity off", files: build, config: "\n[lint.severity]\nAR403 = \"off\"\n", skill: body("`npm run deploy`\n"), absent: []string{"AR403"}},
	})
}
