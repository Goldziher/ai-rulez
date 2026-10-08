package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchSize names a synthetic project size used by the benchmarks.
type BenchSize struct {
	Name  string
	Files int // content files under .ai-rulez (rules, context, skills, agents, commands)
}

// BenchSizes are the small/medium/large trees every table-driven benchmark runs.
func BenchSizes() []BenchSize {
	return []BenchSize{{"small", 10}, {"medium", 200}, {"large", 2000}}
}

// BenchTreeOptions tunes BuildBenchTree.
type BenchTreeOptions struct {
	// Git initialises a repository and stages the tree.
	Git bool
	// Presets overrides the default preset list.
	Presets []string
	// ExtraConfig is appended to config.toml.
	ExtraConfig string
	// Include adds a local include of a second, smaller tree (rules and context).
	Include bool
}

// BenchBody is realistic prose: headings, lists, links, inline code, a URL, a
// fenced shell block and a long run of plain words.
func BenchBody(lines int) string {
	var sb strings.Builder
	for i := 0; i < lines; i++ {
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
			sb.WriteString("```\n")
		case 6:
			sb.WriteString("The reviewer, the author and the maintainer each own one part of the change; ask them when a step is unclear or when the tests disagree with the description.\n")
		default:
			sb.WriteString("Remember to update the documentation, the changelog and the examples whenever the behavior of a command or an option changes for the user.\n")
		}
	}
	return sb.String()
}

// BuildBenchTree writes a project of roughly files content files under a fresh
// temporary directory and returns its root. The mix is 40% rules, 20% context,
// 30% skills, 5% agents and 5% commands, with frontmatter on every file.
func BuildBenchTree(tb testing.TB, files int, opts BenchTreeOptions) string {
	tb.Helper()
	root := tb.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil { //nolint:gosec // benchmark fixture
			tb.Fatal(err)
		}
	}
	presets := opts.Presets
	if len(presets) == 0 {
		presets = []string{"claude", "cursor"}
	}
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	cfg := fmt.Sprintf("version = \"5.0\"\nname = \"bench\"\npresets = [%s]\n", strings.Join(quoted, ", "))
	if opts.Include {
		cfg += "\n[[includes]]\nname = \"shared\"\nsource = \"./shared/.ai-rulez\"\nmerge_strategy = \"local-override\"\n"
		write("shared/.ai-rulez/config.toml", "version = \"5.0\"\nname = \"shared\"\npresets = []\n")
		for i := 0; i < max(files/10, 2); i++ {
			write(fmt.Sprintf("shared/.ai-rulez/rules/shared-%04d.md", i), fmt.Sprintf("---\npriority: medium\n---\n# Shared %d\n%s", i, BenchBody(8)))
		}
	}
	cfg += opts.ExtraConfig
	write(".ai-rulez/config.toml", cfg)
	write("src/main.go", "package main\n\nfunc main() {}\n")
	write("docs/guide.md", "# Guide\n")
	body := BenchBody(20)
	for i := 0; i < files; i++ {
		switch kind := i % 20; {
		case kind < 8:
			paths := ""
			if i%3 == 0 {
				paths = "paths:\n  - \"src/**/*.go\"\n"
			}
			write(fmt.Sprintf(".ai-rulez/rules/rule-%04d.md", i), fmt.Sprintf("---\npriority: medium\n%s---\n# Rule %d\n%s", paths, i, body))
		case kind < 12:
			write(fmt.Sprintf(".ai-rulez/context/ctx-%04d.md", i), fmt.Sprintf("# Context %d\nSee [guide](../../docs/guide.md).\n%s", i, body))
		case kind < 18:
			write(fmt.Sprintf(".ai-rulez/skills/skill-%04d/SKILL.md", i),
				fmt.Sprintf("---\nname: skill-%04d\ndescription: Use when exercising skill %d of the synthetic benchmark project.\n---\n# Skill %d\n%s", i, i, i, body))
		case kind < 19:
			write(fmt.Sprintf(".ai-rulez/agents/agent-%04d.md", i),
				fmt.Sprintf("---\nname: agent-%04d\ndescription: Use when the benchmark needs agent %d.\n---\n# Agent %d\n%s", i, i, i, body))
		default:
			write(fmt.Sprintf(".ai-rulez/commands/cmd-%04d.md", i),
				fmt.Sprintf("---\nname: cmd-%04d\ndescription: Run benchmark command %d.\n---\n# Command %d\n%s", i, i, i, body))
		}
	}
	if opts.Git {
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
			Git(tb, root, args...)
		}
	}
	return root
}
