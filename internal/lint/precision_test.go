package lint

import (
	"testing"
)

// precisionTree is a minimal project whose single rule carries body; extra adds
// files (tracked paths, a skill, more config).
func precisionTree(body, extraConfig string, extra map[string]string) map[string]string {
	files := map[string]string{
		".ai-rulez/config.toml":  baseConfig + extraConfig,
		".ai-rulez/rules/doc.md": "# Doc\n" + body + "\n",
		"src/main.go":            "package main\n",
		"adrs/0065-license.md":   "# adr\n",
		"README.md":              "# r\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func TestPathMissingPrecision(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		// False positives seen in the corpus: each must stay quiet.
		{"negated: there is no", "There is no `src/e2e-generator`; generation is done elsewhere.", 0},
		{"negated: was removed", "(An older `src/mock-server` was removed; only the task text mentions it.)", 0},
		{"negated: decommissioned", "The dead path is decommissioned with `src/llm-gateway`.", 0},
		{"placeholder date", "Spill detail into `src/log/YYYY-MM-DD.md` and keep the summary short.", 0},
		{"alternative: or under", "Place it in `src/queries/lang.rs` (or under `src/extract/queries/`, matching the layout).", 1},
		{"build artifact: target dir", "`task build` builds the binary at `src/target/release/server`.", 0},
		{"build artifact: venv", "The e2e venv at `src/python/.venv` keeps a stale extension.", 0},
		{"build artifact: vendor", "Formatter is `src/php/vendor/bin/php-cs-fixer`, which the job never installs.", 0},
		{"stated as gitignored", "- `src/state/daily.log` is the run log (gitignored, not committed state).", 0},
		{"other repo: @ ref", "| Pro | `src/charts/pro` @ `development` | values |", 0},
		{"other repo: table row with slug", "| `xberg.json` | `acme/xberg` | `src/scripts/publish/x.tmpl` |", 0},
		{"glob example in paragraph", "A glob such as `e2e/**` also prunes\n`src/test/java/io/e2e/` at any depth.", 0},
		{"stated empty", "`src/fixtures/` on disk is empty; do not add a tree.", 0},
		{"ADR number prefix", "Licensed by contract (see `adrs/0065`).", 0},
		// True positives: must keep firing.
		{"plain stale path", "Edit `src/old_module/api.py` after the move.", 1},
		{"stale workflow", "The release lives in `src/release.yml` and is run by CI.", 1},
		{"ADR prefix with no match", "See `adrs/0099` for the decision.", 1},
		{"instruction not negated", "Do not edit `src/gone.go` by hand.", 1},
		{"no far from the path", "There are no tests, so edit `src/gone.go` directly.", 1},
		{"existing file stays quiet", "See `src/main.go`.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", nil))
			gitAdd(t, root)
			// Act
			fs := lintDir(t, root)
			// Assert
			if got := countCode(fs, CodePathMissing); got != tt.want {
				t.Errorf("AR401 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

func TestSkillResourceMissingPrecision(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"table row naming other repos", "| Manifest | Source repo | Template |\n|---|---|---|\n| `a.json` | `acme/alpha` | `scripts/publish/a.json.tmpl` |", 0},
		{"negated", "There is no `scripts/build.sh` in this skill.", 0},
		{"true positive: missing script", "Run `scripts/build.sh` first.", 1},
		{"true positive: missing reference", "Read `references/api.md` for the shapes.", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":           baseConfig,
				".ai-rulez/skills/alpha/SKILL.md": "---\nname: alpha\ndescription: Deploy the billing service. Use when releasing billing changes to staging.\n---\n" + tt.body + "\n",
				"README.md":                       "# r\n",
			})
			gitAdd(t, root)
			fs := lintDir(t, root)
			if got := countCode(fs, CodeSkillResourceMissing); got != tt.want {
				t.Errorf("AR402 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

// AR402 accepts a resource path that names a sibling skill explicitly, so a
// SKILL.md can point at another skill's file (#346). A bare references/ path is
// still resolved against the current skill only.
func TestSkillResourceMissing_SiblingSkillPaths(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"harness path to a sibling's file", "See `.claude/skills/beta/references/service-reference.md`.", 0},
		{"source path to a sibling's file", "See `skills/beta/references/service-reference.md`.", 0},
		{"missing sibling file", "See `.claude/skills/beta/references/absent.md`.", 1},
		{"the current skill's own path still resolves", "See `references/own.md`.", 0},
		{"a bare path that is only in a sibling still reports", "See `references/service-reference.md`.", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":                                 baseConfig,
				".ai-rulez/skills/alpha/SKILL.md":                       "---\nname: alpha\ndescription: Deploy the billing service. Use when releasing billing changes to staging.\n---\n" + tt.body + "\n",
				".ai-rulez/skills/alpha/references/own.md":              "# own\n",
				".ai-rulez/skills/beta/SKILL.md":                        "---\nname: beta\ndescription: Beta skill for billing. Use when releasing billing changes to staging.\n---\nbody\n",
				".ai-rulez/skills/beta/references/service-reference.md": "# ref\n",
			})
			gitAdd(t, root)
			fs := lintDir(t, root)
			if got := countCode(fs, CodeSkillResourceMissing); got != tt.want {
				t.Errorf("AR402 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

func TestUnpinnedExecOwnPackagePrecision(t *testing.T) {
	const cfg = "version = \"5.0\"\nname = \"gitfluff\"\npresets = [\"claude\"]\n"
	tests := []struct {
		name string
		body string
		want int
	}{
		{"prose naming the project's own tool", "Published on PyPI. Also usable via `uvx gitfluff`.", 0},
		{"prose naming another tool stays reported", "Also usable via `uvx other-tool`.", 1},
		{"own tool under @latest is exempt", "Run `npx -y gitfluff@latest`.", 0},
		{"own tool in a fenced shell block is exempt", "```sh\nnpx -y gitfluff@latest generate\n```", 0},
		{"own tool under another moving tag stays reported", "Run `npx -y gitfluff@next`.", 1},
		{"another tool under @latest stays reported", "Run `npx -y other-tool@latest`.", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":  cfg,
				".ai-rulez/rules/doc.md": "# Doc\n" + tt.body + "\n",
			})
			gitAdd(t, root)
			fs := lintDir(t, root)
			if got := countCode(fs, CodeUnpinnedExec); got != tt.want {
				t.Errorf("AR021 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

func TestDescriptionStylePrecision(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want int
	}{
		// Descriptions that do state a trigger, in the wording the corpus uses.
		{"load for", "Change or evaluate OCR backends and caching. Load for OCR behavior work, not ordinary PDF text extraction.", 0},
		{"load before", "The fixtures submodule is not committed. Load before running Rust tests on a fresh clone.", 0},
		{"use any time", "Audit bindings for coverage gaps. Use this skill any time you need to check a type is present in every language.", 0},
		{"use as stage", "Discovers a raw pool of developer profiles. Use as stage 1 of the daily workflow.", 0},
		{"use tool as the gate", "Use poly as the single lint and format gate instead of invoking ruff directly.", 0},
		{"existing use when", "Deploy the billing service. Use when releasing billing changes.", 0},
		// Descriptions that name no trigger stay reported.
		{"noun phrase", "Storage, middleware, and server infrastructure specialist", 1},
		{"capability only", "Reviews SQL compilation correctness and generated code quality across all target languages.", 1},
		{"imperative without trigger", "Run the test suite and report results", 1},
		{"the word load as a noun", "Handles the load balancer configuration for staging clusters", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":           baseConfig + "\n[lint.description]\nrequire_use_when = true\n",
				".ai-rulez/skills/alpha/SKILL.md": "---\nname: alpha\ndescription: " + tt.desc + "\n---\nbody\n",
			})
			gitAdd(t, root)
			fs := lintDir(t, root)
			if got := countCode(fs, CodeDescriptionStyle); got != tt.want {
				t.Errorf("AR803 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

// A slash command is invoked by the user by name, so it has no trigger to state:
// AR803 asks for "when to use" on skills and agents only.
func TestDescriptionStyleAppliesToSkillsAndAgentsOnly(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int
	}{
		{"skill is checked", ".ai-rulez/skills/alpha/SKILL.md", 1},
		{"agent is checked", ".ai-rulez/agents/alpha.md", 1},
		{"command is not", ".ai-rulez/commands/alpha.md", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml": baseConfig + "\n[lint.description]\nrequire_use_when = true\n",
				tt.path:                 "---\nname: alpha\ndescription: Auto-fix linting, formatting, and common issues\n---\nbody\n",
			})
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			if got := countCode(fs, CodeDescriptionStyle); got != tt.want {
				t.Errorf("AR803 count = %d, want %d:\n%s", got, tt.want, dump(fs))
			}
		})
	}
}

// A code span inside link text is the label of the link, not a reference (#456):
// the target is what is checked.
func TestSkillResourceMissing_LinkTextCodeSpan(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantAR402    int
		wantUnlinked int
	}{
		{"label span with a resolving target", "See [`b` `references/s.md`](references/own.md).", 0, 0},
		{"label span with an unresolved target reports the target only", "See [`references/own.md`](references/gone.md).", 0, 1},
		{"span outside the link text still reports", "See [the guide](references/own.md) and `references/absent.md`.", 1, 0},
		{"span before the link text still reports", "Read `references/absent.md` then [guide](references/own.md).", 1, 0},
		{"unlinked label span still reports", "See [`references/absent.md`] for the shapes.", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":                    baseConfig,
				".ai-rulez/skills/alpha/SKILL.md":          "---\nname: alpha\ndescription: Deploy the billing service. Use when releasing billing changes to staging.\n---\n" + tt.body + "\n",
				".ai-rulez/skills/alpha/references/own.md": "# own\n",
				"README.md": "# r\n",
			})
			gitAdd(t, root)
			fs := lintDir(t, root)
			if got := countCode(fs, CodeSkillResourceMissing); got != tt.wantAR402 {
				t.Errorf("AR402 count = %d, want %d:\n%s", got, tt.wantAR402, dump(fs))
			}
			if got := countCode(fs, CodeLinkUnresolved); got != tt.wantUnlinked {
				t.Errorf("link-unresolved count = %d, want %d:\n%s", got, tt.wantUnlinked, dump(fs))
			}
		})
	}
}
