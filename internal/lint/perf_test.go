package lint

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// syntheticSkillBody is realistic skill prose: headings, lists, links, inline
// code, a URL, fenced shell and a long run of plain words (the shape that
// stresses backtracking regular expressions).
func syntheticSkillBody(lines int) string {
	var sb strings.Builder
	for i := 0; sb.Len() == 0 || i < lines; i++ {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&sb, "## Step %d\n", i)
		case 1:
			sb.WriteString("Use the project conventions when you write the change, keep the diff small and explain the reasoning in the commit message so a reviewer can follow it.\n")
		case 2:
			sb.WriteString("- Read [the guide](https://example.com/docs/guide) and the file `src/main.go` before you start, then run `go test ./...` to check the result.\n")
		case 3:
			sb.WriteString("```sh\n")
		case 4:
			sb.WriteString("git status && git diff --stat | head -20\n")
		case 5:
			sb.WriteString("curl -fsSL https://example.com/install.sh -o install.sh\n")
		case 6:
			sb.WriteString("```\n")
		case 7:
			sb.WriteString("The reviewer, the author and the maintainer each own one part of the change; ask them when a step is unclear or when the tests disagree with the description.\n")
		default:
			sb.WriteString("Remember to update the documentation, the changelog and the examples whenever the behavior of a command or an option changes for the user.\n")
		}
	}
	// close an unterminated fence
	if strings.Count(sb.String(), "```")%2 == 1 {
		sb.WriteString("```\n")
	}
	return sb.String()
}

func syntheticSkillTree(tb testing.TB, skills, lines int) string {
	tb.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		tb.Skip("git not available")
	}
	root := tb.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	write(".ai-rulez/config.toml", baseConfig)
	body := syntheticSkillBody(lines)
	for i := 0; i < skills; i++ {
		write(fmt.Sprintf(".ai-rulez/skills/skill-%03d/SKILL.md", i),
			fmt.Sprintf("---\nname: skill-%03d\ndescription: Use when exercising the lint performance of skill %d.\n---\n# Skill %d\n%s", i, i, i, body))
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		if out, err := gitutil.CommandNoContext(root, args...).CombinedOutput(); err != nil {
			tb.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

// loadTree loads the config and the tree once, so the benchmark and the guard
// time the lint pass itself and not the config loader.
func loadTree(tb testing.TB, root string) (*config.Config, *Tree) {
	tb.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	if err != nil {
		tb.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		tb.Fatal(err)
	}
	return cfg, tree
}

func BenchmarkRunSkillTree(b *testing.B) {
	cfg, tree := loadTree(b, syntheticSkillTree(b, 50, 400))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Run(cfg, tree); err != nil {
			b.Fatal(err)
		}
	}
}

// TestRunSkillTreeStaysLinearAndFast guards against a regular expression that
// backtracks on long prose. The bound is generous (more than 10x the measured
// time, around 80us per line) so only a real regression trips it, not a slow machine.
func TestRunSkillTreeStaysLinearAndFast(t *testing.T) {
	if testing.Short() {
		t.Skip("performance guard")
	}
	const skills, lines = 40, 400
	cfg, tree := loadTree(t, syntheticSkillTree(t, skills, lines))
	start := time.Now()
	if _, err := Run(cfg, tree); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	perLine := elapsed / (skills * lines)
	t.Logf("%d skills x %d lines: %s (%s per line)", skills, lines, elapsed, perLine)
	if perLine > time.Millisecond {
		t.Errorf("lint takes %s per line of skill text (%s total), over the 1ms guard", perLine, elapsed)
	}
}

// TestNegGateKeepsEveryAlternative pins the literal pre-check of negRe to its
// pattern: each alternative must still match through the gate.
func TestNegGateKeepsEveryAlternative(t *testing.T) {
	for _, text := range []string{
		"Never do this", "Don't do this", "dont do this", "Do  not do this", "You MUST NOT do this", "You should not do this", "Shouldn't do this", "shouldnt",
		"Avoid this", "Use X instead of Y", "Use X rather than Y", "Forbidden", "This is prohibited", "prohibits it", "Disallowed", "disallow it", "Denied",
		"Dangerous", "Unsafe", "Insecure", "Malicious", "Attacker", "attacks", "Exploit", "exploitation", "Anti-pattern", "antipatterns", "Bad idea", "Wrong",
		"Incorrect", "Harmful", "Destructive", "Risky", "Refuse it", "refused", "Reject it", "rejection", "Vulnerable", "vulnerability", "Injection", "A threat",
		"Red flag", "red flags", "Whenever you must, do not", "Use the anti pattern? No: anti-patterns", "an antipattern", "❌ no", "⛔ no", "🚫 no", "⚠ careful",
	} {
		if !negRe.re.MatchString(text) {
			t.Errorf("fixture %q does not match the pattern itself", text)
		}
		if !negRe.MatchString(text) {
			t.Errorf("the literal pre-check rejects %q, which the pattern matches", text)
		}
	}
	if negRe.MatchString("Whenever you can, run the tests and read the docs.") {
		t.Error("ordinary prose matched")
	}
}
