package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codeHits(findings []Finding, code string) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestKiroAndKiloTraps(t *testing.T) {
	const cfg = ".ai-rulez/config.toml"
	kiro := trapConfig(`"kiro"`, "")
	kilo := trapConfig(`"kilo"`, "")
	steer := "# style\n"
	tests := []struct {
		name  string
		files map[string]string
		code  string
		want  int
		line  int
	}{
		{name: "AR9C5 bad: agent without resources next to steering", files: map[string]string{cfg: kiro, ".kiro/agents/a.json": `{"name":"a"}`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering, want: 1, line: 1},
		{name: "AR9C5 bad: empty resources", files: map[string]string{cfg: kiro, ".kiro/agents/a.json": `{"name":"a","resources":[]}`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering, want: 1, line: 1},
		{name: "AR9C5 good: resources set", files: map[string]string{cfg: kiro, ".kiro/agents/a.json": `{"resources":["file://.kiro/steering/**/*.md"]}`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering},
		{name: "AR9C5 good: no steering files", files: map[string]string{cfg: kiro, ".kiro/agents/a.json": `{"name":"a"}`}, code: CodeKiroAgentSteering},
		{name: "AR9C5 good: not JSON", files: map[string]string{cfg: kiro, ".kiro/agents/a.json": `{`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering},
		{name: "AR9C5 good: markdown agent is not a JSON agent", files: map[string]string{cfg: kiro, ".kiro/agents/a.md": "---\nname: a\n---\n", ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering},
		{name: "AR9C5 nested package looks beside itself", files: map[string]string{cfg: kiro, "pkg/.kiro/agents/a.json": `{"name":"a"}`, "pkg/.kiro/steering/s.md": steer}, code: CodeKiroAgentSteering, want: 1, line: 1},
		{name: "AR9C5 nested package ignores root steering", files: map[string]string{cfg: kiro, "pkg/.kiro/agents/a.json": `{"name":"a"}`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering},
		{name: "AR9C5 off without the kiro preset", files: map[string]string{cfg: trapConfig(`"claude"`, ""), ".kiro/agents/a.json": `{"name":"a"}`, ".kiro/steering/s.md": steer}, code: CodeKiroAgentSteering},
		{name: "AR9C6 bad: blank line before frontmatter", files: map[string]string{cfg: kiro, ".kiro/steering/s.md": "\n---\ninclusion: manual\n---\n# s\n"}, code: CodeKiroSteeringFirst, want: 1, line: 1},
		{name: "AR9C6 bad: text before frontmatter", files: map[string]string{cfg: kiro, ".kiro/steering/s.md": "# s\n---\ninclusion: auto\nname: x\n---\nbody\n"}, code: CodeKiroSteeringFirst, want: 1, line: 1},
		{name: "AR9C6 good: frontmatter first", files: map[string]string{cfg: kiro, ".kiro/steering/s.md": "---\ninclusion: manual\n---\n# s\n"}, code: CodeKiroSteeringFirst},
		{name: "AR9C6 good: no frontmatter", files: map[string]string{cfg: kiro, ".kiro/steering/s.md": "# s\n\ntext\n"}, code: CodeKiroSteeringFirst},
		{name: "AR9C6 good: horizontal rules without inclusion", files: map[string]string{cfg: kiro, ".kiro/steering/s.md": "# s\n---\nfoo: bar\n---\n"}, code: CodeKiroSteeringFirst},
		{name: "AR9C9 bad: Kilo REVIEW.md over 10000 chars", files: map[string]string{cfg: kilo, "REVIEW.md": strings.Repeat("x", 10001)}, code: CodeHarnessLimitExceeded, want: 1, line: 1},
		{name: "AR9C9 good: Kilo REVIEW.md at the limit", files: map[string]string{cfg: kilo, "REVIEW.md": strings.Repeat("x", 10000)}, code: CodeHarnessLimitExceeded},
		{name: "AR9C9 good: nested REVIEW.md is not read by Kilo", files: map[string]string{cfg: kilo, "docs/REVIEW.md": strings.Repeat("x", 10001)}, code: CodeHarnessLimitExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange / Act
			got := codeHits(trapFindings(t, tc.files), tc.code)

			// Assert
			if len(got) != tc.want {
				t.Fatalf("want %d %s findings, got %d: %+v", tc.want, tc.code, len(got), got)
			}
			if tc.want > 0 {
				if got[0].Line != tc.line || got[0].Trap == nil || got[0].Trap.Harness == "" || got[0].Trap.Evidence == "" || got[0].Trap.VerifiedOn == "" {
					t.Errorf("finding lacks position or provenance: %+v", got[0])
				}
			}
		})
	}
}

func TestProjectTraps(t *testing.T) {
	const cfg = ".ai-rulez/config.toml"
	base := trapConfig(`"claude"`, "")
	row := `[[trap]]
name = "docs-need-title"
harness = "team"
message = "docs pages need a title"
hint = "Add title: to the frontmatter."
[trap.scope]
dir = "docs"
suffix = ".md"
[trap.predicate]
kind = "frontmatter-missing-all"
keys = ["title"]
`
	tests := []struct {
		name  string
		files map[string]string
		want  int
		match string
	}{
		{name: "row fires for a matching file", files: map[string]string{cfg: base, ".ai-rulez/traps/team.toml": row, "docs/a.md": "# a\n"}, want: 1, match: "docs pages need a title"},
		{name: "row quiet when the file complies", files: map[string]string{cfg: base, ".ai-rulez/traps/team.toml": row, "docs/a.md": "---\ntitle: A\n---\n"}},
		{name: "size-over row with its own limit", files: map[string]string{cfg: base, ".ai-rulez/traps/s.toml": "[[trap]]\nname = \"big\"\nharness = \"team\"\nmessage = \"too long\"\n[trap.scope]\ndir = \"notes\"\n[trap.predicate]\nkind = \"size-over\"\nmeasure = \"file-chars\"\nlimit = 5\n", "notes/n.md": "123456"}, want: 1, match: "too long"},
		{name: "unknown predicate kind is reported", files: map[string]string{cfg: base, ".ai-rulez/traps/b.toml": strings.Replace(row, "frontmatter-missing-all", "regex", 1), "docs/a.md": "# a\n"}, want: 1, match: "not one of"},
		{name: "unknown field is reported", files: map[string]string{cfg: base, ".ai-rulez/traps/b.toml": strings.Replace(row, "hint =", "hnt =", 1)}, want: 1, match: "invalid trap file"},
		{name: "scope outside the project is refused", files: map[string]string{cfg: base, ".ai-rulez/traps/b.toml": strings.Replace(row, `dir = "docs"`, `dir = "../docs"`, 1)}, want: 1, match: "inside the project"},
		{name: "size-over without a limit is refused", files: map[string]string{cfg: base, ".ai-rulez/traps/b.toml": "[[trap]]\nname = \"x\"\nharness = \"t\"\nmessage = \"m\"\n[trap.scope]\ndir = \"a\"\n[trap.predicate]\nkind = \"size-over\"\nmeasure = \"file-bytes\"\n"}, want: 1, match: "limit > 0"},
		{name: "no traps directory", files: map[string]string{cfg: base, "docs/a.md": "# a\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange / Act
			got := codeHits(trapFindings(t, tc.files), CodeProjectTrap)

			// Assert
			if len(got) != tc.want {
				t.Fatalf("want %d AR9CA findings, got %d: %+v", tc.want, len(got), got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Message, tc.match) {
				t.Errorf("message %q lacks %q", got[0].Message, tc.match)
			}
		})
	}
}

func TestTrapFixRenamesMisspeltKeyInHandwrittenFile(t *testing.T) {
	// Arrange: a hand-written skill (outside the authored source root) and a generated one.
	banner := "<!-- Generated by ai-rulez from x. -->\n"
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":     trapConfig(`"claude"`, ""),
		".claude/skills/h/SKILL.md": "---\nname: h\nallowed_tools: Read\n---\nbody\n",
		".claude/skills/g/SKILL.md": "---\nname: g\nallowed_tools: Read\n---\n" + banner + "body\n",
	})
	gitAdd(t, root)
	findings := codeHits(lintDir(t, root), "AR9C7")

	// Act
	res, err := ApplyFixes(findings, FixOptions{EditRoot: filepath.Join(root, ".ai-rulez")})

	// Assert: only the hand-written file is rewritten.
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want one applied fix, got %+v skipped %+v", res.Applied, res.Skipped)
	}
	hand, _ := os.ReadFile(filepath.Join(root, ".claude/skills/h/SKILL.md")) //nolint:errcheck // a missing file fails the assertion
	if !strings.Contains(string(hand), "allowed-tools: Read") || strings.Contains(string(hand), "allowed_tools") {
		t.Errorf("key not renamed: %q", hand)
	}
	gen, _ := os.ReadFile(filepath.Join(root, ".claude/skills/g/SKILL.md")) //nolint:errcheck // a missing file fails the assertion
	if !strings.Contains(string(gen), "allowed_tools") {
		t.Errorf("generated file was rewritten: %q", gen)
	}
}

func TestTrapFixIsIdempotent(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":     trapConfig(`"claude"`, ""),
		".claude/skills/h/SKILL.md": "---\nname: h\nuser_invocable: false\n---\nbody\n",
	})
	gitAdd(t, root)
	opts := FixOptions{EditRoot: filepath.Join(root, ".ai-rulez")}
	if _, err := ApplyFixes(codeHits(lintDir(t, root), "AR9C7"), opts); err != nil {
		t.Fatal(err)
	}

	// Act
	again := codeHits(lintDir(t, root), "AR9C7")

	// Assert
	if len(again) != 0 {
		t.Fatalf("finding remains after the fix: %+v", again)
	}
}
