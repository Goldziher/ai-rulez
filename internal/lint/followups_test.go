package lint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func fixedNow(t *testing.T, day string) {
	t.Helper()
	fixed, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	old := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = old })
}

func lintFiles(t *testing.T, files map[string]string) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	gitAdd(t, root)
	return lintDir(t, root)
}

func dupFiles(extraConfig string) map[string]string {
	skill := "---\nname: dup\ndescription: Use when testing duplicate handling of skills.\n---\nbody\n"
	return map[string]string{
		".ai-rulez/config.toml":                         baseConfig + extraConfig,
		".ai-rulez/skills/dup/SKILL.md":                 skill,
		".ai-rulez/domains/backend/skills/dup/SKILL.md": skill,
	}
}

func TestDuplicateCollapsed(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{name: "reported by default", want: true},
		{name: "allowed by name", config: "\n[lint]\nallow_overrides = [\"dup\"]\n"},
		{name: "allowed by domain and name", config: "\n[lint]\nallow_overrides = [\"backend/dup\"]\n"},
		{name: "other name does not allow it", config: "\n[lint]\nallow_overrides = [\"other\"]\n", want: true},
		{name: "other domain does not allow it", config: "\n[lint]\nallow_overrides = [\"frontend/dup\"]\n", want: true},
		{name: "severity can be raised", config: "\n[lint.severity]\nduplicate-collapsed = \"error\"\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lintFiles(t, dupFiles(tt.config))
			if gotAny := countCode(got, CodeDuplicateCollapsed) > 0; gotAny != tt.want {
				t.Fatalf("AR703 reported = %v, want %v:\n%s", gotAny, tt.want, dump(got))
			}
		})
	}
	t.Run("message names both paths", func(t *testing.T) {
		got := lintFiles(t, dupFiles(""))
		for _, f := range got {
			if f.Code == CodeDuplicateCollapsed {
				if !strings.Contains(f.Message, "skills/dup/SKILL.md") || !strings.Contains(f.Message, "domains/backend") {
					t.Errorf("message lacks both paths: %s", f.Message)
				}
				return
			}
		}
		t.Fatal("no AR703 finding")
	})
}

func TestUnknownFrontmatterKey(t *testing.T) {
	tests := []struct {
		name   string
		config string
		front  string
		kind   string
		want   bool
		line   int
	}{
		{name: "typo of allowed-tools", front: "allowed_tools: Read\n", kind: "skill", want: true, line: 4},
		{name: "spec and claude keys pass", front: "allowed-tools: Read\nwhen_to_use: x\nuser-invocable: false\nlicense: MIT\nmetadata:\n  owner: a\n", kind: "skill"},
		{name: "ai-rulez keys pass", front: "priority: high\ntargets: [claude]\nplacement: core\n", kind: "skill"},
		{name: "allow-listed custom key", config: "\n[lint]\nallowed_keys = [\"team\"]\n", front: "team: core\n", kind: "skill"},
		{name: "custom key without allow-list", front: "team: core\n", kind: "skill", want: true, line: 4},
		{name: "subagent camelCase key passes", front: "disallowedTools: Bash\n", kind: "agent"},
		{name: "context summary passes", front: "summary: One line.\n", kind: "context"},
		{name: "summary on a skill is flagged", front: "summary: One line.\n", kind: "skill", want: true, line: 4},
		{name: "skill key on a subagent is flagged", front: "user-invocable: false\n", kind: "agent", want: true, line: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := ".ai-rulez/skills/s/SKILL.md"
			if tt.kind == "agent" {
				path = ".ai-rulez/agents/s.md"
			}
			if tt.kind == "context" {
				path = ".ai-rulez/context/s.md"
			}
			md := "---\nname: s\ndescription: Use when testing frontmatter keys here.\n" + tt.front + "---\nbody\n"
			if tt.kind == "agent" {
				md = "---\nname: s\ndescription: Use when testing frontmatter keys here.\n" + tt.front + "---\nbody\n"
			}
			got := lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig + tt.config, path: md})
			if gotAny := countCode(got, CodeFrontmatterKey) > 0; gotAny != tt.want {
				t.Fatalf("AR303 reported = %v, want %v:\n%s", gotAny, tt.want, dump(got))
			}
			if tt.want && !has(got, CodeFrontmatterKey, filepath.Base(path), tt.line) {
				t.Errorf("want line %d:\n%s", tt.line, dump(got))
			}
		})
	}
}

func TestTypedMetadata(t *testing.T) {
	fixedNow(t, "2026-10-04")
	rules := "\n[lint.metadata.last_verified]\ntype = \"date\"\nmax_age_days = 90\nrequired = true\n" +
		"\n[lint.metadata.owner]\ntype = \"string\"\nrequired = true\n" +
		"\n[lint.metadata.tier]\ntype = \"enum\"\nvalues = [\"gold\", \"silver\"]\n"
	tests := []struct {
		name  string
		front string
		want  map[string]bool
	}{
		{name: "fresh nested metadata", front: "metadata:\n  owner: team-a\n  last_verified: \"2026-09-01\"\n", want: map[string]bool{}},
		{name: "unquoted top-level date", front: "owner: team-a\nlast_verified: 2026-09-01\n", want: map[string]bool{}},
		{name: "stale date", front: "metadata:\n  owner: a\n  last_verified: \"2026-01-01\"\n", want: map[string]bool{CodeMetadataStale: true}},
		{name: "invalid date", front: "metadata:\n  owner: a\n  last_verified: soon\n", want: map[string]bool{CodeMetadataInvalid: true}},
		{name: "future date", front: "metadata:\n  owner: a\n  last_verified: \"2027-01-01\"\n", want: map[string]bool{CodeMetadataInvalid: true}},
		{name: "missing required keys", front: "", want: map[string]bool{CodeMetadataMissing: true}},
		{name: "enum violation", front: "metadata:\n  owner: a\n  last_verified: \"2026-09-01\"\n  tier: bronze\n", want: map[string]bool{CodeMetadataInvalid: true}},
		{name: "enum ok", front: "metadata:\n  owner: a\n  last_verified: \"2026-09-01\"\n  tier: Gold\n", want: map[string]bool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := "---\nname: s\ndescription: Use when testing typed metadata rules.\n" + tt.front + "---\nbody\n"
			got := lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig + rules, ".ai-rulez/skills/s/SKILL.md": md})
			for _, code := range []string{CodeMetadataStale, CodeMetadataInvalid, CodeMetadataMissing} {
				if gotAny := countCode(got, code) > 0; gotAny != tt.want[code] {
					t.Errorf("%s reported = %v, want %v:\n%s", code, gotAny, tt.want[code], dump(got))
				}
			}
		})
	}
}

func TestRequireMetadataReadsTheMetadataMap(t *testing.T) {
	cfg := "\n[lint.require_metadata]\nskill = [\"owner\"]\n"
	md := func(front string) string {
		return "---\nname: s\ndescription: Use when testing required metadata here.\n" + front + "---\nbody\n"
	}
	got := lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig + cfg, ".ai-rulez/skills/s/SKILL.md": md("metadata:\n  owner: a\n")})
	if countCode(got, CodeMetadataMissing) != 0 {
		t.Errorf("owner inside metadata must satisfy require_metadata:\n%s", dump(got))
	}
	got = lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig + cfg, ".ai-rulez/skills/s/SKILL.md": md("")})
	if countCode(got, CodeMetadataMissing) != 1 {
		t.Errorf("missing owner must be reported:\n%s", dump(got))
	}
}

func TestSupersededBy(t *testing.T) {
	old := "---\nname: old\ndescription: Use when the legacy flow applies here.\ndeprecated: true\nsuperseded_by: %s\n---\nbody\n"
	newSkill := "---\nname: fresh\ndescription: Use when the current flow applies to you.\n---\nbody\n"
	tests := []struct {
		target string
		want   bool
	}{{"fresh", false}, {"ghost", true}}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := lintFiles(t, map[string]string{
				".ai-rulez/config.toml":           baseConfig,
				".ai-rulez/skills/old/SKILL.md":   strings.Replace(old, "%s", tt.target, 1),
				".ai-rulez/skills/fresh/SKILL.md": newSkill,
			})
			if gotAny := countCode(got, CodeSupersededMissing) > 0; gotAny != tt.want {
				t.Fatalf("AR954 reported = %v, want %v:\n%s", gotAny, tt.want, dump(got))
			}
			if countCode(got, CodeFrontmatterKey) != 0 {
				t.Errorf("deprecated and superseded_by are known keys:\n%s", dump(got))
			}
		})
	}
}

func TestUnpinnedRemote(t *testing.T) {
	remote := "\n[[includes]]\nname = \"shared\"\nsource = \"https://example.com/org/rules.git\"\n"
	sha := strings.Repeat("a", 40)
	tests := []struct {
		name  string
		extra string
		lock  string
		want  bool
	}{
		{name: "moving ref without a lock", extra: remote, want: true},
		{name: "full SHA is pinned", extra: remote + "ref = \"" + sha + "\"\n"},
		{name: "covered by the lock", extra: remote, lock: "version = 1\n[[include]]\nname = \"shared\"\nsource = \"https://example.com/org/rules.git\"\ncommit = \"" + sha + "\"\ndigest = \"sha256:x\"\n"},
		{name: "stale lock", extra: remote + "ref = \"main\"\n", lock: "version = 1\n[[include]]\nname = \"shared\"\nsource = \"https://example.com/org/rules.git\"\ncommit = \"" + sha + "\"\ndigest = \"sha256:x\"\n", want: true},
		{name: "local include needs no pin", extra: "\n[[includes]]\nname = \"loc\"\nsource = \"../shared\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".ai-rulez/config.toml": baseConfig + tt.extra}
			if tt.lock != "" {
				files[".ai-rulez/ai-rulez.lock"] = tt.lock
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)
			cfg := loadNoRemote(t, root)
			tree, err := LoadTree(root)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := Run(cfg, tree)
			if err != nil {
				t.Fatal(err)
			}
			if gotAny := countCode(rep.Findings, CodeUnpinnedRemote) > 0; gotAny != tt.want {
				t.Fatalf("AR010 reported = %v, want %v:\n%s", gotAny, tt.want, dump(rep.Findings))
			}
		})
	}
}

func TestExternalScanner(t *testing.T) {
	sarif := `{"runs":[{"results":[{"ruleId":"X1","level":"error","message":{"text":"bad thing"},` +
		`"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":3}}}]}]}]}`
	tests := []struct {
		name    string
		script  string
		format  string
		run     bool
		wantMsg string
		wantSev Severity
	}{
		{name: "sarif findings are merged", script: "cat <<'EOF'\n" + sarif + "\nEOF\nexit 1\n", run: true, wantMsg: "bad thing", wantSev: SeverityError},
		{name: "json findings are merged", format: "json", script: `echo '[{"file":".ai-rulez/rules/r.md","line":2,"severity":"warning","rule":"J","message":"json thing"}]'` + "\n", run: true, wantMsg: "json thing", wantSev: SeverityWarning},
		{name: "not run without the flag", script: "echo '" + sarif + "'\n", run: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{".ai-rulez/rules/r.md": "# Rule\n\nbody\n"})
			script := filepath.Join(root, "scan.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+tt.script), 0o755); err != nil {
				t.Fatal(err)
			}
			format := ""
			if tt.format != "" {
				format = "format = \"" + tt.format + "\"\n"
			}
			writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig +
				"\n[[lint.external]]\nname = \"fake\"\ncommand = [\"" + filepath.ToSlash(script) + "\"]\n" + format})
			gitAdd(t, root)
			cfg := loadNoRemote(t, root)
			tree, err := LoadTree(root)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := RunWith(cfg, tree, Options{External: tt.run})
			if err != nil {
				t.Fatal(err)
			}
			n := countCode(rep.Findings, CodeExternalFinding)
			if !tt.run {
				if n != 0 {
					t.Fatalf("scanner must not run without the flag:\n%s", dump(rep.Findings))
				}
				return
			}
			found := false
			for _, f := range rep.Findings {
				if f.Code == CodeExternalFinding && strings.Contains(f.Message, tt.wantMsg) && f.Severity == tt.wantSev {
					found = true
					if !strings.Contains(f.Message, "[fake]") {
						t.Errorf("message lacks the scanner name: %s", f.Message)
					}
				}
			}
			if !found {
				t.Fatalf("want %q at %s:\n%s", tt.wantMsg, tt.wantSev, dump(rep.Findings))
			}
		})
	}
}

func TestSecurityOnlyOption(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".ai-rulez/rules/r.md":  "# R\n\nAKIAIOSFODNN7EXAMPLE and [bad](nope.md)\n",
	})
	gitAdd(t, root)
	cfg := loadNoRemote(t, root)
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RunWith(cfg, tree, Options{SecurityOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if countCode(rep.Findings, CodeSecretDetected) != 1 || countCode(rep.Findings, CodeLinkUnresolved) != 0 {
		t.Errorf("security-only keeps AR0xx and drops the rest:\n%s", dump(rep.Findings))
	}
}

func TestValidateSettingsNewFields(t *testing.T) {
	bad := `
[lint.metadata.a]
type = "enum"
[lint.metadata.b]
type = "number"
[lint.metadata.c]
type = "string"
max_age_days = 5
[lint.security]
scan_imports = "sometimes"
allowed_hosts = ["https://x.test"]
secret_patterns = [{ name = "n", regex = "(" }]
[[lint.external]]
name = "x"
`
	cfg := loadNoRemote(t, writeRoot(t, bad))
	problems := ValidateSettings(cfg.Lint)
	for _, want := range []string{"type enum needs values", "unknown type", "max_age_days", "scan_imports", "allowed_hosts", "invalid regex", "lint.external[0]"} {
		found := false
		for _, p := range problems {
			if strings.Contains(p, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no problem mentioning %q in %v", want, problems)
		}
	}
}

func writeRoot(t *testing.T, extraConfig string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig + extraConfig})
	return root
}

// loadNoRemote loads the config without resolving remote includes.
func loadNoRemote(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestGlobNoMatchMessageForAlwaysApply(t *testing.T) {
	tests := []struct {
		name, front, want, not string
	}{
		{name: "plain rule never applies", front: "globs: [\"nothing/**\"]\n", want: "never applies"},
		{name: "alwaysApply rule still applies", front: "alwaysApply: true\nglobs: [\"nothing/**\"]\n", want: "still applies", not: "never applies"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/r.md": "---\n" + tt.front + "---\nbody\n"}

			// Act
			got := lintFiles(t, files)

			// Assert
			for _, f := range got {
				if f.Code == CodeGlobNoMatch {
					if !strings.Contains(f.Message, tt.want) || (tt.not != "" && strings.Contains(f.Message, tt.not)) {
						t.Fatalf("message %q", f.Message)
					}
					return
				}
			}
			t.Fatalf("no AR101:\n%s", dump(got))
		})
	}
}
