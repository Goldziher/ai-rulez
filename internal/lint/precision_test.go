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
