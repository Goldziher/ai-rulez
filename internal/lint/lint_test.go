package lint

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func gitAdd(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		// gitutil drops GIT_INDEX_FILE and friends, which a hook (or a shell
		// exporting them) would otherwise point at another repository.
		cmd := gitutil.CommandNoContext(root, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// gitSetExec records the executable bit of rel (a path under root, already
// added) in the git index. Lint reads a tracked file's mode from the index, and
// a file system with no mode bits (Windows) can only express it there.
func gitSetExec(t *testing.T, root, rel string) {
	t.Helper()
	cmd := gitutil.CommandNoContext(root, "update-index", "--chmod=+x", "--", rel)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git update-index --chmod=+x %s: %v\n%s", rel, err, out)
	}
}

// gitIndexMode is the mode git records for rel ("100644", "100755", ...).
func gitIndexMode(t *testing.T, root, rel string) string {
	t.Helper()
	out, err := gitutil.CommandNoContext(root, "ls-files", "--stage", "--", rel).Output()
	if err != nil {
		t.Fatalf("git ls-files --stage %s: %v", rel, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		t.Fatalf("git does not track %s", rel)
	}
	return fields[0]
}

func lintDir(t *testing.T, base string) []Finding {
	t.Helper()
	cfg, err := loadWithResolvers(context.Background(), base)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tree, err := LoadTree(base)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func has(fs []Finding, code, fileSuffix string, line int) bool {
	for _, f := range fs {
		if f.Code == code && strings.HasSuffix(f.File, fileSuffix) && (line == 0 || f.Line == line) {
			return true
		}
	}
	return false
}

func countCode(fs []Finding, code string) int {
	n := 0
	for _, f := range fs {
		if f.Code == code {
			n++
		}
	}
	return n
}

const baseConfig = "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"

func fixture(extraConfig string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":     baseConfig + extraConfig,
		".ai-rulez/rules/scoped.md": "---\npaths:\n  - \"src/**/*.py\"\n  - \"nothing/**\"\n---\n# Scoped\n",
		".ai-rulez/rules/linky.md": "# Linky\n" +
			"[ok](../context/c.md) [bad](missing.md) [anchor](../context/c.md#nope) [good](../context/c.md#real-heading)\n" +
			"Path `src/a.py` and `src/gone.py` and `docs/<placeholder>.md`.\n" +
			"Use the `ghost-skill` skill or `/ghost-cmd` or skill `alpha`.\n" +
			"```\n[fenced](nope.md) `src/fenced-missing.py`\n```\n" +
			"<!-- ai-rulez-lint-ignore: AR401 -->\n`src/ignored-missing.py`\n",
		".ai-rulez/context/c.md": "# Real heading\n",
		".ai-rulez/skills/alpha/SKILL.md": "---\nname: alpha\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster.\n---\n" +
			"See `scripts/run.sh` and `references/absent.md`.\n",
		".ai-rulez/skills/alpha/scripts/run.sh": "#!/bin/sh\necho hi\n",
		".ai-rulez/skills/beta/SKILL.md":        "---\nname: Beta_Skill\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster today.\n---\n",
		".ai-rulez/skills/nodesc/SKILL.md":      "---\nname: nodesc\n---\nbody\n",
		".ai-rulez/agents/rev.md":               "---\ndescription: Reviews code changes for the billing team carefully.\nskills:\n  - ghost\n  - alpha\n---\nbody\n",
		".claude/settings.json": `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[` +
			`{"type":"command","command":"\"$CLAUDE_PROJECT_DIR\"/tools/missing.sh"},` +
			`{"type":"command","command":"${CLAUDE_PROJECT_DIR}/tools/hook.sh --flag"}]}]}}`,
		"tools/hook.sh": "#!/bin/sh\n",
		"src/a.py":      "x = 1\n",
	}
}

func TestRunDetectsBrokenContent(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, fixture("\n[[mcp_servers]]\nname = \"nope\"\ncommand = \"definitely-not-a-real-binary-xyz\"\n"))
	gitAdd(t, root)
	fs := lintDir(t, root)

	tests := []struct {
		code, file string
		line       int
	}{
		{CodeGlobNoMatch, "rules/scoped.md", 4},
		{CodeLinkUnresolved, "rules/linky.md", 2},
		{CodeAnchorUnresolved, "rules/linky.md", 2},
		{CodePathMissing, "rules/linky.md", 3},
		{CodeReferenceUnknown, "rules/linky.md", 4},
		{CodeSkillResourceMissing, "skills/alpha/SKILL.md", 5},
		{CodeScriptNotExecutable, "skills/alpha/scripts/run.sh", 1},
		{CodeFrontmatterSkill, "agents/rev.md", 4},
		{CodeDescriptionMissing, "skills/nodesc/SKILL.md", 1},
		{CodeDescriptionNearDup, "skills/beta/SKILL.md", 3},
		{CodeSkillNameInvalid, "skills/beta/SKILL.md", 2},
		{CodeHookMissing, ".claude/settings.json", 0},
		{CodeHookNotExecutable, ".claude/settings.json", 0},
		{CodeMCPCommandNotFound, ".ai-rulez/config.toml", 7},
	}
	for _, tt := range tests {
		if !has(fs, tt.code, tt.file, tt.line) {
			t.Errorf("expected %s in %s (line %d); findings:\n%s", tt.code, tt.file, tt.line, dump(fs))
		}
	}

	// Things that must NOT be reported.
	if n := countCode(fs, CodePathMissing); n != 1 {
		t.Errorf("want exactly 1 path-missing (fenced, placeholder and ignored paths excluded), got %d:\n%s", n, dump(fs))
	}
	if n := countCode(fs, CodeLinkUnresolved); n != 1 {
		t.Errorf("want exactly 1 link-unresolved (fenced link excluded), got %d", n)
	}
	if n := countCode(fs, CodeReferenceUnknown); n != 2 {
		t.Errorf("want ghost-skill and /ghost-cmd only, got %d:\n%s", n, dump(fs))
	}
	if n := countCode(fs, CodeGlobNoMatch); n != 1 {
		t.Errorf("src/**/*.py matches src/a.py and must not be reported, got %d", n)
	}
}

func dump(fs []Finding) string {
	var sb strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&sb, "  %s:%d %s %s\n", f.File, f.Line, f.Code, f.Message)
	}
	return sb.String()
}

func TestRunHonorsLintConfig(t *testing.T) {
	tests := []struct {
		name   string
		cfg    string
		check  func(t *testing.T, fs []Finding)
		expect string
	}{
		{"ignore code", "[lint]\nignore = [\"AR401\", \"link-unresolved\"]\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodePathMissing)+countCode(fs, CodeLinkUnresolved) != 0 {
				t.Error("ignored codes still reported")
			}
		}, ""},
		{"severity override", "[lint.severity]\nAR401 = \"error\"\nglob-no-match = \"off\"\n", func(t *testing.T, fs []Finding) {
			for _, f := range fs {
				if f.Code == CodePathMissing && f.Severity != SeverityError {
					t.Errorf("AR401 severity = %s, want error", f.Severity)
				}
				if f.Code == CodeGlobNoMatch {
					t.Error("off severity must drop the finding")
				}
			}
		}, ""},
		{"ignore paths", "[lint]\nignore_paths = [\"rules/**\"]\n", func(t *testing.T, fs []Finding) {
			for _, f := range fs {
				if strings.Contains(f.File, "/rules/") {
					t.Errorf("finding in ignored path: %+v", f)
				}
			}
		}, ""},
		{"allow paths", "[lint]\nallow_paths = [\"src/gone.py\"]\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodePathMissing) != 0 {
				t.Error("allow-listed path still reported")
			}
		}, ""},
		{"known names", "[lint]\nknown_names = [\"ghost-skill\", \"ghost-cmd\"]\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodeReferenceUnknown) != 0 {
				t.Error("known names still reported")
			}
		}, ""},
		{"budgets", "[lint.budgets.skill]\nmax_lines = 2\nmax_tokens = 5\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodeSizeLines) == 0 || countCode(fs, CodeSizeTokens) == 0 {
				t.Errorf("tight budgets must report size findings:\n%s", dump(fs))
			}
		}, ""},
		{"budget disabled", "[lint.budgets.skill]\nmax_lines = -1\nmax_tokens = -1\n[lint.budgets.rule]\nmax_lines = 1\n", func(t *testing.T, fs []Finding) {
			for _, f := range fs {
				if (f.Code == CodeSizeLines || f.Code == CodeSizeTokens) && strings.Contains(f.File, "/skills/") {
					t.Errorf("negative budget must disable the limit: %+v", f)
				}
			}
			if countCode(fs, CodeSizeLines) == 0 {
				t.Error("rule budget of 1 line must report")
			}
		}, ""},
		{"required metadata", "[lint.require_metadata]\nskill = [\"owner\"]\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodeMetadataMissing) != 3 {
				t.Errorf("want 3 skills missing owner, got %d", countCode(fs, CodeMetadataMissing))
			}
		}, ""},
		{"description bounds", "[lint.description]\nmin_length = 500\n", func(t *testing.T, fs []Finding) {
			if countCode(fs, CodeDescriptionLength) == 0 {
				t.Error("min_length 500 must flag the short descriptions")
			}
		}, ""},
		{"require use when", "[lint.description]\nrequire_use_when = true\n", func(t *testing.T, fs []Finding) {
			if !has(fs, CodeDescriptionStyle, "agents/rev.md", 0) {
				t.Errorf("agent description lacks use-when phrasing:\n%s", dump(fs))
			}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, fixture("\n"+tt.cfg))
			gitAdd(t, root)
			tt.check(t, lintDir(t, root))
		})
	}
}

func TestNestedRootResolvesRelativeToItsDirAndInheritsNames(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":                  baseConfig,
		".ai-rulez/skills/parent-skill/SKILL.md": "---\ndescription: A skill defined by the parent root for everyone.\n---\n",
		"svc/.ai-rulez/config.toml":              baseConfig,
		"svc/.ai-rulez/rules/r.md":               "---\npaths: [\"app/**/*.go\", \"missing/**\"]\n---\nUse the `parent-skill` skill and `app/main.go` and `app/nope.go`.\n",
		"svc/app/main.go":                        "package main\n",
	})
	gitAdd(t, root)
	fs := lintDir(t, filepath.Join(root, "svc"))
	if countCode(fs, CodeReferenceUnknown) != 0 {
		t.Errorf("parent root skill must resolve:\n%s", dump(fs))
	}
	if countCode(fs, CodeGlobNoMatch) != 1 || !has(fs, CodeGlobNoMatch, "r.md", 2) {
		t.Errorf("only missing/** should fail, relative to the nested root:\n%s", dump(fs))
	}
	if countCode(fs, CodePathMissing) != 1 {
		t.Errorf("app/nope.go should be the only missing path:\n%s", dump(fs))
	}
}

func TestFailedThreshold(t *testing.T) {
	warn := []Finding{{Severity: SeverityWarning}}
	errf := []Finding{{Severity: SeverityError}}
	tests := []struct {
		name   string
		fs     []Finding
		failOn string
		want   bool
	}{
		{"error default fails on error", errf, "", true},
		{"warning passes default", warn, "", false},
		{"warning fails on warning", warn, "warning", true},
		{"none never fails", errf, "none", false},
		{"empty passes", nil, "error", false},
	}
	for _, tt := range tests {
		if got := Failed(tt.fs, tt.failOn); got != tt.want {
			t.Errorf("%s: Failed = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestValidateSettings(t *testing.T) {
	tests := []struct {
		name string
		lc   *config.LintConfig
		want int
	}{
		{"nil", nil, 0},
		{"valid", &config.LintConfig{Ignore: []string{"AR401", "path-missing"}, Severity: map[string]string{"AR101": "warning"}}, 0},
		{"unknown ignore", &config.LintConfig{Ignore: []string{"AR999"}}, 1},
		{"unknown severity", &config.LintConfig{Severity: map[string]string{"AR101": "loud"}}, 1},
		{"unknown kind", &config.LintConfig{Budgets: map[string]config.LintBudget{"widget": {}}}, 1},
	}
	for _, tt := range tests {
		if got := len(ValidateSettings(tt.lc)); got != tt.want {
			t.Errorf("%s: %d problems, want %d", tt.name, got, tt.want)
		}
	}
}

func TestRegistryCodesAreUniqueAndNamed(t *testing.T) {
	seenCode, seenName := map[string]bool{}, map[string]bool{}
	for _, r := range Rules() {
		if seenCode[r.Code] || seenName[r.Name] || r.Describe == "" {
			t.Errorf("duplicate or undescribed rule %+v", r)
		}
		seenCode[r.Code], seenName[r.Name] = true, true
	}
}
