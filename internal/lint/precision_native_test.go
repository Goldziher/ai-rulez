package lint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const nativeSkillHead = "---\nname: alpha\ndescription: Deploy the billing service. Use when releasing billing changes to staging.\n---\n"

// nativeFindings lints a config-free tree whose native .claude content is detected the way validate finds it.
func nativeFindings(t *testing.T, files map[string]string, implicit bool) []Finding {
	t.Helper()
	root := t.TempDir()
	if _, ok := files[".ai-rulez/config.toml"]; !ok && !implicit {
		files = merge(files, map[string]string{".ai-rulez/config.toml": baseConfig})
	}
	writeFiles(t, root, files)
	gitAdd(t, root)
	if !implicit {
		fs, _ := lintWithTargets(t, root, TargetSpec{Detect: true})
		return fs
	}
	cfg, err := config.LoadImplicit(context.Background(), root)
	require.NoError(t, err)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	targets, err := ResolveTargets(context.Background(), tree, TargetSpec{Detect: true, Cwd: root})
	require.NoError(t, err)
	rep, err := Run(cfg, tree, WithTargets(targets), WithCwd(root))
	require.NoError(t, err)
	return rep.Findings
}

func countSeverity(fs []Finding, code string, sev Severity) int {
	n := 0
	for _, f := range fs {
		if f.Code == code && f.Severity == sev {
			n++
		}
	}
	return n
}

type nativeCase struct {
	name  string
	files map[string]string
	code  string
	sev   Severity // "" counts the code at any severity
	want  int
	// implicit lints the tree without any ai-rulez configuration, the way validate does in a project that has none.
	implicit bool
}

func runNativeCases(t *testing.T, tests []nativeCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			fs := nativeFindings(t, tt.files, tt.implicit)

			// Assert
			got := countCode(fs, tt.code)
			if tt.sev != "" {
				got = countSeverity(fs, tt.code, tt.sev)
			}
			assert.Equal(t, tt.want, got, "%s findings:\n%s", tt.code, dump(fs))
		})
	}
}

func skillFile(dir, body string) map[string]string {
	return map[string]string{dir + "/.claude/skills/alpha/SKILL.md": nativeSkillHead + body + "\n"}
}

func merge(parts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range parts {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}

// A skill's resource paths also resolve from the area root: the directory that holds its .claude directory.
func TestSkillResourceResolvesFromTheAreaRoot(t *testing.T) {
	area := "products/em/backend"
	runNativeCases(t, []nativeCase{
		{"script under the area root", merge(skillFile(area, "Run `scripts/tool.py`."), map[string]string{area + "/scripts/tool.py": "x\n"}), CodeSkillResourceMissing, "", 0, false},
		{"directory under the area root", merge(skillFile(area, "See `scripts/jobs/`."), map[string]string{area + "/scripts/jobs/a.py": "x\n"}), CodeSkillResourceMissing, "", 0, false},
		{"a missing script still reports", merge(skillFile(area, "Run `scripts/other.py`."), map[string]string{area + "/scripts/tool.py": "x\n"}), CodeSkillResourceMissing, "", 1, false},
		{"another area's script does not count", merge(skillFile(area, "Run `scripts/tool.py`."), map[string]string{"products/other/scripts/tool.py": "x\n"}), CodeSkillResourceMissing, "", 1, false},
	})
}

// A resource path next to the name of another skill resolves against that skill.
func TestSkillResourceNamingAnotherSkill(t *testing.T) {
	beta := map[string]string{".claude/skills/beta/SKILL.md": "---\nname: beta\ndescription: Beta. Use when running beta tasks.\n---\nbody\n", ".claude/skills/beta/references/downloads.md": "# d\n"}
	runNativeCases(t, []nativeCase{
		{"the named skill ships it", merge(skillFile(".", "(see skill `beta`, `references/downloads.md`)"), beta), CodeSkillResourceMissing, "", 0, false},
		{"the file follows the skill word", merge(skillFile(".", "See `references/downloads.md` of the `beta` skill."), beta), CodeSkillResourceMissing, "", 0, false},
		{"the named skill lacks it", merge(skillFile(".", "(see skill `beta`, `references/absent.md`)"), beta), CodeSkillResourceMissing, "", 1, false},
		{"a skill from a plugin is not ours to check", skillFile(".", "See `references/queries.md` of the `dt-export-tables` skill."), CodeSkillResourceMissing, "", 0, false},
		{"no skill named, the path is missing", merge(skillFile(".", "Read `references/downloads.md`."), map[string]string{".claude/skills/beta/references/downloads.md": "# d\n"}), CodeSkillResourceMissing, "", 1, false},
	})
}

func TestReferencesToPluginAndBuiltinSkills(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"plugin-qualified subagent_type", skillFile(".", "Use the Task tool with `subagent_type: \"pr-review-toolkit:code-reviewer\"`."), CodeReferenceUnknown, "", 0, false},
		{"plugin-qualified agent in backticks", skillFile(".", "Spawn the `pr-review-toolkit:code-reviewer` agent."), CodeReferenceUnknown, "", 0, false},
		{"unqualified unknown subagent_type", skillFile(".", "Use the Task tool with `subagent_type: \"ghost-agent\"`."), CodeReferenceUnknown, "", 1, false},
		{"a harness built-in skill", skillFile(".", "Run the `simplify` skill on the changes."), CodeReferenceUnknown, "", 0, false},
		{"a bundled plugin skill", skillFile(".", "Validated with the `dataviz` skill."), CodeReferenceUnknown, "", 0, false},
		{"an unknown skill", skillFile(".", "Run the `ghost-skill` skill on the changes."), CodeReferenceUnknown, "", 1, false},
		{"known_names covers a project external", merge(skillFile(".", "Run the `acme-tool` skill."), map[string]string{".ai-rulez/config.toml": baseConfig + "[lint]\nknown_names = [\"acme-tool\"]\n"}), CodeReferenceUnknown, "", 0, false},
	})
}

func TestRuleReferenceNeedsAReferenceContext(t *testing.T) {
	rule := map[string]string{".claude/rules/real.md": "# Real rule\nBody.\n"}
	runNativeCases(t, []nativeCase{
		{"a CSS property next to the word rule", merge(skillFile(".", "`display` rule needs the same guard."), rule), CodeReferenceUnknown, "", 0, false},
		{"a CSS property after an article", merge(skillFile(".", "Apply the `flex` rule to the container."), rule), CodeReferenceUnknown, "", 0, false},
		{"a hyphenated rule name that does not exist", merge(skillFile(".", "Follow the `ghost-rule` rule."), rule), CodeReferenceUnknown, "", 1, false},
		{"a rule file that does not exist", merge(skillFile(".", "Follow the `ghost` rule file."), rule), CodeReferenceUnknown, "", 1, false},
		{"an existing rule", merge(skillFile(".", "Follow the `real` rule file."), rule), CodeReferenceUnknown, "", 0, false},
	})
}

func TestNegatedAndHistoricalMentionsAreNotClaims(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"do not add a tier (path)", skillFile(".", "- **Don't add a `references/patterns/` tier.** See above."), CodeSkillResourceMissing, "", 0, false},
		{"add a tier (path)", skillFile(".", "- **Add a `references/patterns/` tier.** See above."), CodeSkillResourceMissing, "", 1, false},
		{"the former skill", skillFile(".", "Folded in from the former `legacy-writer` skill."), CodeReferenceUnknown, "", 0, false},
		{"a deprecated skill", skillFile(".", "The deprecated `legacy-writer` skill is gone."), CodeReferenceUnknown, "", 0, false},
		{"not a registered command", skillFile(".", "Do NOT run `/qa-review` — that is NOT a registered slash command."), CodeReferenceUnknown, "", 0, false},
		{"a command that is expected to exist", skillFile(".", "Run `/qa-review` before merging."), CodeReferenceUnknown, "", 1, false},
		{"do not edit stays a claim (path)", skillFile(".", "Do not edit `scripts/gone.sh` by hand."), CodeSkillResourceMissing, "", 1, false},
	})
}

func TestPlaceholderResourcePathsAreNotClaims(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"single capital letter segment", skillFile(".", "| What does service X do? | `references/services/X.md` |"), CodeSkillResourceMissing, "", 0, false},
		{"angle placeholder", skillFile(".", "Read `references/<name>.md`."), CodeSkillResourceMissing, "", 0, false},
		{"brace placeholder", skillFile(".", "Read `references/{name}.md`."), CodeSkillResourceMissing, "", 0, false},
		{"glob", skillFile(".", "Run `scripts/*.sh`."), CodeSkillResourceMissing, "", 0, false},
		{"ellipsis segment", skillFile(".", "Read `references/.../index.md`."), CodeSkillResourceMissing, "", 0, false},
		{"a real missing file", skillFile(".", "Read `references/x.md`."), CodeSkillResourceMissing, "", 1, false},
	})
}

func TestSlashSkillWordingIsLookedUpAsASkill(t *testing.T) {
	skill := map[string]string{".claude/skills/sheet-reader/SKILL.md": "---\nname: sheet-reader\ndescription: Sheets. Use when reading sheet-reader.\n---\nbody\n"}
	runNativeCases(t, []nativeCase{
		{"a local skill by slash name", merge(skillFile(".", "Use the `/sheet-reader` skill."), skill), CodeReferenceUnknown, "", 0, false},
		{"unknown, called a skill: info, not error", skillFile(".", "Intent: tracking sheet (use `/gws-sheet-reader-read` skill)"), CodeReferenceUnknown, SeverityError, 0, false},
		{"unknown, called a skill: still reported", skillFile(".", "Intent: tracking sheet (use `/gws-sheet-reader-read` skill)"), CodeReferenceUnknown, SeverityInfo, 1, false},
		{"unknown command stays an error", skillFile(".", "Run `/ghost-cmd` now."), CodeReferenceUnknown, SeverityError, 1, false},
	})
}

func TestIdentifierValuesAreNotCredentials(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"snake_case key name with a digit", skillFile(".", "    \"password\": \"saq_tn3270_scan_password\","), CodeSecretDetected, "", 0, false},
		{"snake_case key name without a digit", skillFile(".", "    \"password\": \"saq_ssh_scan_password\","), CodeSecretDetected, "", 0, false},
		{"a random mixed value", skillFile(".", "    \"password\": \"Xk9fQ2pLm4Zr8TvB1nWc\","), CodeSecretDetected, "", 1, false},
		{"underscored hex-like chunks", skillFile(".", "    \"password\": \"ab12_cd34_ef56_gh78_ij90\","), CodeSecretDetected, "", 1, false},
	})
}

// A path in a nested project's content is written from that project, not from the repository root.
func TestPathMissingResolvesFromTheNearestProject(t *testing.T) {
	area := "products/em/backend"
	top := map[string]string{"scripts/other.sh": "x\n", "src/main.go": "package main\n"}
	rule := func(body string) map[string]string {
		return map[string]string{area + "/.claude/rules/r.md": "# Rule\n" + body + "\n"}
	}
	runNativeCases(t, []nativeCase{
		{"under the project directory", merge(top, rule("See `scripts/tool.py`."), map[string]string{area + "/scripts/tool.py": "x\n"}), CodePathMissing, "", 0, false},
		{"missing in the project too", merge(top, rule("See `scripts/absent.py`."), map[string]string{area + "/scripts/tool.py": "x\n"}), CodePathMissing, "", 1, false},
		{"only in another project", merge(top, rule("See `scripts/tool.py`."), map[string]string{"products/other/scripts/tool.py": "x\n"}), CodePathMissing, "", 1, false},
		{"a nested file reads the project's .claude directory", merge(top, map[string]string{".claude/rules/root.md": "# r\n", area + "/.claude/rules/ctx.md": "# c\n", area + "/commands/CLAUDE.md": "# Cmd\nSee `.claude/rules/ctx.md`.\n"}), CodePathMissing, "", 0, false},
		{"a nested file names a rule its project lacks", merge(top, map[string]string{".claude/rules/root.md": "# r\n", area + "/.claude/rules/ctx.md": "# c\n", area + "/commands/CLAUDE.md": "# Cmd\nSee `.claude/rules/absent.md`.\n"}), CodePathMissing, "", 1, false},
		{"beside the file", merge(top, map[string]string{"deploy/svc/CLAUDE.md": "# Svc\nEdit `src/handler.go`.\n", "deploy/svc/src/handler.go": "package s\n"}), CodePathMissing, "", 0, false},
		{"beside the file, missing", merge(top, map[string]string{"deploy/svc/CLAUDE.md": "# Svc\nEdit `src/handler.go`.\n"}), CodePathMissing, "", 1, false},
	})
}

func TestExplicitHTMLAnchorsResolve(t *testing.T) {
	arch := map[string]string{".claude/skills/alpha/references/arch.md": "# Arch\n## <a name=\"enum-rules\"></a>2. Enum Rules\n<div id='box'></div>\n"}
	runNativeCases(t, []nativeCase{
		{"a named anchor", merge(skillFile(".", "See [enums](references/arch.md#enum-rules)."), arch), CodeAnchorUnresolved, "", 0, false},
		{"an id attribute", merge(skillFile(".", "See [box](references/arch.md#box)."), arch), CodeAnchorUnresolved, "", 0, false},
		{"the heading text itself", merge(skillFile(".", "See [h](references/arch.md#arch)."), arch), CodeAnchorUnresolved, "", 0, false},
		{"an anchor nobody defines", merge(skillFile(".", "See [x](references/arch.md#nope)."), arch), CodeAnchorUnresolved, "", 1, false},
	})
}

func TestReferenceUnknownIsInformationWithoutAConfig(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"no config: an error is too strong", skillFile(".", "Run the `ghost-skill` skill."), CodeReferenceUnknown, SeverityError, 0, true},
		{"no config: reported as information", skillFile(".", "Run the `ghost-skill` skill."), CodeReferenceUnknown, SeverityInfo, 1, true},
		{"a config closes the world: still an error", skillFile(".", "Run the `ghost-skill` skill."), CodeReferenceUnknown, SeverityError, 1, false},
	})
}

func TestConfigTamperNeedsAWholeVerb(t *testing.T) {
	runNativeCases(t, []nativeCase{
		{"update the file", skillFile(".", "Then update CLAUDE.md with what you learned."), CodeConfigTamper, "", 1, false},
		{"appends to the file", skillFile(".", "It appends a line to ~/.claude/CLAUDE.md."), CodeConfigTamper, "", 1, false},
		{"a script named update_x", skillFile(".", "`update_fail_status.sh` (see `relay/base/CLAUDE.md`)"), CodeConfigTamper, "", 0, false},
		{"a key that starts with add", skillFile(".", "The `additionalDirectories` entry in `.claude/settings.json` resolves from the launch folder."), CodeConfigTamper, "", 0, false},
	})
}

func TestMarketplaceSourceMustExist(t *testing.T) {
	market := func(source, meta string) map[string]string {
		return map[string]string{".claude-plugin/marketplace.json": `{"name":"acme","owner":{"name":"A"},` + meta + `"plugins":[{"name":"a","source":"` + source + `"}]}` + "\n"}
	}
	plugin := map[string]string{"plugins/a/.claude-plugin/plugin.json": `{"name":"a"}` + "\n"}
	runNativeCases(t, []nativeCase{
		{"a source directory that is there", merge(market("./plugins/a", ""), plugin), CodePluginManifest, "", 0, false},
		{"a source directory that is missing", market("./plugins/missing", ""), CodePluginManifest, "", 1, false},
		{"the repository itself", market(".", ""), CodePluginManifest, "", 0, false},
		{"under metadata.pluginRoot", merge(market("./a", `"metadata":{"pluginRoot":"./plugins"},`), plugin), CodePluginManifest, "", 0, false},
		{"missing under metadata.pluginRoot", market("./b", `"metadata":{"pluginRoot":"./plugins"},`), CodePluginManifest, "", 1, false},
	})
}

// A tree whose index lists nothing has no files to match a rule's paths against, whatever else a run picks up.
func TestGlobNoMatchNeedsAnIndex(t *testing.T) {
	rule := "---\npaths:\n  - \"src/**/*.ts\"\n---\n# R\nbody\n"
	runNativeCases(t, []nativeCase{
		{"files exist, none match", merge(skillFile(".", "x"), map[string]string{".ai-rulez/rules/r.md": rule, "README.md": "# r\n"}), CodeGlobNoMatch, "", 1, false},
		{"files exist, one matches", merge(skillFile(".", "x"), map[string]string{".ai-rulez/rules/r.md": rule, "src/a.ts": "x\n"}), CodeGlobNoMatch, "", 0, false},
	})
}
