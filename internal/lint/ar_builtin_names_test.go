package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
)

// fakeReach is a HarnessReach that returns the same targets for every item.
type fakeReach []ReachTarget

func (f fakeReach) Reach(string, string) []ReachTarget { return f }

// lintOutputs lints the project at root with the opts and returns the findings.
func lintOutputs(t *testing.T, root string, opts ...Option) []Finding {
	t.Helper()
	cfg, err := loadWithResolvers(context.Background(), root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree, append([]Option{WithCwd(root)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func namesConfig(presets string, extra string) string {
	return "version = \"5.0\"\nname = \"t\"\npresets = [" + presets + "]\n" + extra
}

func commandFile(desc string) string {
	return "---\ndescription: " + desc + "\n---\nbody\n"
}

func agentFile(name string) string {
	return "---\nname: " + name + "\ndescription: Use when testing agents named like built-in agents.\n---\nbody\n"
}

func findingsOf(got []Finding, code string) []Finding {
	var out []Finding
	for _, f := range got {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestHarnessBuiltinNames(t *testing.T) {
	tests := []struct {
		name     string
		presets  string
		extra    string
		files    map[string]string
		code     string
		want     []Severity // severities of the findings of code, in order
		contains []string   // substrings of the first message
	}{
		{
			name: "builtin wins: the harness keeps the built-in", presets: `"gemini"`,
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityError},
			contains: []string{`skill "help" collides with the gemini built-in command /help`, "keeps the built-in and renames or ignores your skill", `"gemini:help"`, "SlashCommandResolver.ts"},
		},
		{
			name: "user wins: the item replaces the built-in", presets: `"qwen"`,
			files: map[string]string{".ai-rulez/commands/init.md": commandFile("Use when testing the init collision.")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityError},
			contains: []string{`command "init" collides with the qwen`, "Your command replaces the built-in"},
		},
		{
			name: "both listed is a warning", presets: `"codex"`,
			files: map[string]string{".ai-rulez/skills/imagegen/SKILL.md": reservedSkill("imagegen")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
			contains: []string{"Both are listed"},
		},
		{
			name: "a quoted outcome in a table set to warning is a warning, not an error", presets: `"vibe"`,
			files: map[string]string{".ai-rulez/skills/skill-creator/SKILL.md": reservedSkill("skill-creator")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
		},
		{
			name: "an undocumented outcome is information, listed once", presets: `"cursor", "kiro"`,
			files: map[string]string{".ai-rulez/skills/plan/SKILL.md": reservedSkill("plan")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityInfo},
			contains: []string{"cursor", "kiro", "does not document"},
		},
		{
			name: "a built-in no user item can collide with is not reported", presets: `"codebuff"`,
			files: map[string]string{".ai-rulez/skills/diagnostics/SKILL.md": reservedSkill("diagnostics")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "NFKC and case folding of the name: frontmatter", presets: `"gemini"`,
			files: map[string]string{".ai-rulez/skills/x/SKILL.md": reservedSkill(" ＨＥＬＰ ")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityError},
		},
		{
			name: "an alias is reserved", presets: `"codewhale"`,
			files: map[string]string{".ai-rulez/commands/quit.md": commandFile("Use when testing an alias collision.")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
			contains: []string{"alias /quit of /exit"},
		},
		{
			name: "a skill does not collide with a command in a separate namespace", presets: `"opencode"`,
			files: map[string]string{".ai-rulez/skills/review/SKILL.md": reservedSkill("review")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "a built-in agent", presets: `"claude"`,
			files: map[string]string{".ai-rulez/agents/Explore.md": agentFile("Explore")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityError},
			contains: []string{`agent "Explore" collides with the claude built-in agent Explore`},
		},
		{
			name: "claude commands are reported by AR808 only", presets: `"claude"`,
			files: map[string]string{".ai-rulez/skills/init/SKILL.md": reservedSkill("init")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "allow by preset and name", presets: `"gemini"`, extra: "\n[lint.builtin_names]\nallow = [\"Gemini:help\"]\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "allow by name for any harness", presets: `"gemini"`, extra: "\n[lint.builtin_names]\nallow = [\"help\"]\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "allow_overrides is honored", presets: `"gemini"`, extra: "\n[lint]\nallow_overrides = [\"gemini:help\"]\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "another harness entry does not allow it", presets: `"gemini"`, extra: "\n[lint.builtin_names]\nallow = [\"qwen:help\"]\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityError},
		},
		{
			name: "disabled", presets: `"gemini"`, extra: "\n[lint.builtin_names]\nenabled = false\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision,
		},
		{
			name: "severity override downgrades the rule", presets: `"gemini"`, extra: "\n[lint.severity]\nAR9U0 = \"warning\"\n",
			files: map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
		},
		{
			name: "extra names are declared built-ins", presets: `"claude"`, extra: "\n[lint.builtin_names.extra]\nclaude = [\"acme-deploy\"]\n",
			files: map[string]string{".ai-rulez/skills/acme-deploy/SKILL.md": reservedSkill("acme-deploy")},
			code:  CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
			contains: []string{"declared by the project"},
		},
		{
			name: "a project table adds names", presets: `"gemini"`,
			files: map[string]string{
				".ai-rulez/skills/acme-ship/SKILL.md": reservedSkill("acme-ship"),
				".ai-rulez/harness-names/gemini.toml": "schema = 1\npreset = \"gemini\"\nseverity = \"error\"\n[[group]]\nkind = \"command\"\nnames = [\"acme-ship\"]\nuser_collision = \"builtin_wins\"\n",
			},
			code: CodeBuiltinNameCollision, want: []Severity{SeverityError},
		},
		{
			name: "an invalid project table is reported and skipped", presets: `"gemini"`,
			files: map[string]string{
				".ai-rulez/skills/acme-ship/SKILL.md": reservedSkill("acme-ship"),
				".ai-rulez/harness-names/gemini.toml": "schema = 1\npreset = \"claude\"\n",
			},
			code: CodeBuiltinNameCollision, want: []Severity{SeverityWarning},
			contains: []string{"invalid harness names file", "not used"},
		},
		{
			name: "a reserved name is an error", presets: `"claude"`,
			files: map[string]string{".ai-rulez/skills/synced/SKILL.md": reservedSkill("synced")},
			code:  CodeReservedHarnessName, want: []Severity{SeverityError},
			contains: []string{`skill "synced"`, "refuses to load", "does not apply"},
		},
		{
			name: "a reserved prefix is an error", presets: `"claude"`,
			files: map[string]string{".ai-rulez/skills/x/SKILL.md": reservedSkill("Anthropic-Skills:pdf")},
			code:  CodeReservedHarnessName, want: []Severity{SeverityError},
		},
		{
			name: "allow does not silence a reserved name", presets: `"claude"`,
			extra: "\n[lint.builtin_names]\nallow = [\"synced\", \"claude:synced\"]\n[lint]\nallow_overrides = [\"synced\"]\n",
			files: map[string]string{".ai-rulez/skills/synced/SKILL.md": reservedSkill("synced")},
			code:  CodeReservedHarnessName, want: []Severity{SeverityError},
		},
		{
			name: "an ordinary name is fine", presets: `"claude", "gemini", "cursor"`,
			files: map[string]string{".ai-rulez/skills/deploy-helper/SKILL.md": reservedSkill("deploy-helper")},
			code:  CodeBuiltinNameCollision,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := map[string]string{".ai-rulez/config.toml": namesConfig(tt.presets, tt.extra)}
			for k, v := range tt.files {
				files[k] = v
			}
			writeFiles(t, root, files)
			gitAdd(t, root)
			// Act
			got := findingsOf(lintOutputs(t, root), tt.code)
			// Assert
			var sev []Severity
			for _, f := range got {
				sev = append(sev, f.Severity)
			}
			if len(sev) != len(tt.want) {
				t.Fatalf("%s severities = %v, want %v:\n%s", tt.code, sev, tt.want, dump(got))
			}
			for i := range sev {
				if sev[i] != tt.want[i] {
					t.Errorf("finding %d severity = %s, want %s", i, sev[i], tt.want[i])
				}
			}
			for _, c := range tt.contains {
				if len(got) == 0 || !strings.Contains(got[0].Message, c) {
					t.Errorf("message lacks %q:\n%s", c, dump(got))
				}
			}
		})
	}
}

func TestHarnessBuiltinNamesFollowTheReachOfTheOutput(t *testing.T) {
	skill := map[string]string{".ai-rulez/skills/help/SKILL.md": reservedSkill("help")}
	tests := []struct {
		name    string
		presets string
		reach   HarnessReach
		want    []string // presets that carry an AR9U0 error: harnesses the project writes for
		warn    []string // presets that carry an AR9U0 warning: harnesses that only read the directory
		note    string   // in every finding message
		notNote string   // in no error message
	}{
		{"a reader of the shared directory is checked, as a warning, although its preset is not enabled", `"cursor"`,
			fakeReach{{Preset: "cursor", Role: "skill", Dir: ".agents/skills", Writer: true}, {Preset: "gemini", Role: "skill", Dir: ".agents/skills"}},
			nil, []string{"gemini"}, "The harness reads .agents/skills.", ""},
		{"several readers each get their own finding", `"cursor"`,
			fakeReach{{Preset: "gemini", Role: "skill", Dir: ".agents/skills"}, {Preset: "qwen", Role: "skill", Dir: ".agents/skills"}},
			nil, []string{"gemini", "qwen"}, "The harness reads .agents/skills.", ""},
		{"a harness the project writes for stays an error", `"gemini"`,
			fakeReach{{Preset: "gemini", Role: "skill", Writer: true}, {Preset: "qwen", Role: "skill", Dir: ".agents/skills"}},
			[]string{"gemini"}, []string{"qwen"}, "", ""},
		{"an output the plan does not write reaches no harness (plugin-only)", `"gemini"`, fakeReach{}, nil, nil, "", ""},
		{"the same harness twice is reported once", `"gemini"`,
			fakeReach{{Preset: "gemini", Role: "skill", Writer: true}, {Preset: "gemini", Role: "skill", Dir: ".agents/skills"}},
			[]string{"gemini"}, nil, "", "The harness reads"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := map[string]string{".ai-rulez/config.toml": namesConfig(tt.presets, "")}
			for k, v := range skill {
				files[k] = v
			}
			writeFiles(t, root, files)
			gitAdd(t, root)
			// Act
			got := findingsOf(lintOutputs(t, root, WithHarnessReach(tt.reach)), CodeBuiltinNameCollision)
			// Assert
			var presets, warned []string
			for _, f := range got {
				for _, p := range []string{"gemini", "qwen", "cursor"} {
					if !strings.Contains(f.Message, "the "+p+" built-in") {
						continue
					}
					switch f.Severity {
					case SeverityError:
						presets = append(presets, p)
					case SeverityWarning:
						warned = append(warned, p)
					}
				}
			}
			if strings.Join(presets, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("error presets = %v, want %v:\n%s", presets, tt.want, dump(got))
			}
			if strings.Join(warned, ",") != strings.Join(tt.warn, ",") {
				t.Fatalf("warning presets = %v, want %v:\n%s", warned, tt.warn, dump(got))
			}
			for _, f := range got {
				if f.Severity == SeverityInfo {
					continue
				}
				if tt.note != "" && !strings.Contains(f.Message, tt.note) {
					t.Errorf("message lacks %q: %s", tt.note, f.Message)
				}
				if tt.notNote != "" && strings.Contains(f.Message, tt.notNote) {
					t.Errorf("message has %q: %s", tt.notNote, f.Message)
				}
			}
		})
	}
}

func TestReservedNameAliasAR808(t *testing.T) {
	// Arrange: /new is an alias of /clear; a skill replaces the built-in command but not its aliases
	files := map[string]string{
		".ai-rulez/config.toml":           baseConfig,
		".ai-rulez/skills/new/SKILL.md":   reservedSkill("new"),
		".ai-rulez/skills/pdf/SKILL.md":   reservedSkill("pdf"),
		".ai-rulez/skills/clear/SKILL.md": reservedSkill("clear"),
	}
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	// Act
	got := findingsOf(lintOutputs(t, root), "AR808")
	// Assert: the alias and the command, and not the synced pdf skill
	if len(got) != 2 {
		t.Fatalf("AR808 count = %d, want 2:\n%s", len(got), dump(got))
	}
	var alias bool
	for _, f := range got {
		isAlias := strings.Contains(f.Message, `skill "new" is an alias of the Claude Code built-in /clear`) && strings.Contains(f.Message, "keeps running the built-in")
		alias = alias || isAlias
		// The docs say the skill loads and only its alias keeps the built-in, so an alias is a
		// warning; shadowing a built-in command stays an error.
		if want := map[bool]Severity{true: SeverityWarning, false: SeverityError}[isAlias]; f.Severity != want {
			t.Errorf("%s has severity %s, want %s", f.Message, f.Severity, want)
		}
	}
	if !alias {
		t.Errorf("no alias message:\n%s", dump(got))
	}
}

func TestValidateBuiltinNamesSettings(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  string
	}{
		{"unknown preset", "[lint.builtin_names.extra]\nnope = [\"x\"]\n", `unknown preset "nope"`},
		{"not a name", "[lint.builtin_names.extra]\nclaude = [\"a/b\"]\n", "is not a name"},
		{"empty allow", "[lint.builtin_names]\nallow = [\"claude:\"]\n", "names nothing"},
		{"valid", "[lint.builtin_names]\nallow = [\"claude:init\", \"help\"]\n[lint.builtin_names.extra]\nclaude = [\"acme\"]\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig + tt.extra})
			cfg, err := loadWithResolvers(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			problems := ValidateSettings(cfg.Lint)
			joined := strings.Join(problems, "\n")
			if tt.want == "" && joined != "" {
				t.Fatalf("problems = %q, want none", joined)
			}
			if tt.want != "" && !strings.Contains(joined, tt.want) {
				t.Fatalf("problems = %q, want %q", joined, tt.want)
			}
		})
	}
}

func TestNameTableRowsInTheAgeCheck(t *testing.T) {
	// Arrange / Act
	var names, others int
	for _, row := range TableRows() {
		if row.Kind == "names" {
			names++
			if row.Harness == "" || row.VerifiedOn == "" {
				t.Errorf("incomplete names row: %+v", row)
			}
			continue
		}
		others++
	}
	// Assert: one row per preset that has names; the trap and limit rows are still there
	if names < 40 || others == 0 {
		t.Fatalf("names rows = %d, other rows = %d", names, others)
	}
}

func TestReservedNameAR808SkipsWhatTheHarnessDoesNotLoad(t *testing.T) {
	tests := []struct {
		name  string
		reach HarnessReach
		want  int
	}{
		{"no reach: every authored item counts", nil, 1},
		{"the plan writes it where Claude Code reads", fakeReach{{Preset: "claude", Role: "skill", Dir: ".claude/skills", Writer: true}}, 1},
		{"a plugin-only skill is invoked under its plugin prefix", fakeReach{}, 0},
		{"only another harness loads it", fakeReach{{Preset: "cursor", Role: "skill", Dir: ".agents/skills", Writer: true}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":          baseConfig,
				".ai-rulez/skills/init/SKILL.md": reservedSkill("init"),
			})
			gitAdd(t, root)
			var opts []Option
			if tt.reach != nil {
				opts = append(opts, WithHarnessReach(tt.reach))
			}
			// Act
			got := findingsOf(lintOutputs(t, root, opts...), CodeReservedName)
			// Assert
			if len(got) != tt.want {
				t.Fatalf("AR808 count = %d, want %d:\n%s", len(got), tt.want, dump(got))
			}
		})
	}
}

// No embedded group reports AR9U0 as an error without a verbatim quote of the outcome.
func TestEmbeddedTablesNeverErrorWithoutAQuote(t *testing.T) {
	kinds := []string{"command", "skill", "agent"}
	for _, preset := range harnessnames.Presets() {
		table, _ := harnessnames.For(preset)
		for _, e := range table.Entries(kinds) {
			for _, kind := range kinds {
				for _, h := range table.Find(kind, e.Name) {
					if h.Kind == harnessnames.KindReserved || h.Collision == harnessnames.Error {
						if !h.Quoted { // AR9U1 is always an error: its outcome needs the quote
							t.Errorf("%s reserved %q has no quote", preset, e.Name)
						}
						continue
					}
					if h.Quoted {
						continue
					}
					if got := nameSeverity(h, ReachTarget{Preset: preset, Role: kind, Writer: true}); got == SeverityError {
						t.Errorf("%s %s %q (%s) is an error without a quote", preset, kind, e.Name, h.Collision)
					}
				}
			}
		}
	}
}
