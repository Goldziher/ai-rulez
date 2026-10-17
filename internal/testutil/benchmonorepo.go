package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// MonorepoSizes are the file counts the monorepo benchmarks run: 100, 1000 and
// 5000, plus 20000 with BenchLargeEnv=1. The 20000-file tree takes seconds to
// write and index, which the default benchmark run (CI) does not spend.
func MonorepoSizes() []int {
	sizes := []int{100, 1000, 5000}
	if os.Getenv(BenchLargeEnv) == "1" {
		sizes = append(sizes, 20000)
	}
	return sizes
}

// MonorepoLargest is the largest of MonorepoSizes.
func MonorepoLargest() int {
	sizes := MonorepoSizes()
	return sizes[len(sizes)-1]
}

// MonorepoSpec describes a synthetic monorepo for the repository-index
// benchmarks.
type MonorepoSpec struct {
	// Files is the total number of files written, configuration files included.
	Files int
	// Scopes is the number of nested directories (the root among them) that
	// hold an AGENTS.md, a skill and a rule.
	Scopes int
	// Git initializes a repository and stages the tree, so the file listing
	// comes from git rather than a directory walk.
	Git bool
	// Extra are additional files (slash path to content) written before the
	// tree is staged; they count against Files.
	Extra map[string]string
}

// MonorepoScopeDir is the directory of scope i: i = 0 is the root (".") and the
// others sit three levels deep, spread over a fan-out of groups and services.
func MonorepoScopeDir(i int) string {
	if i == 0 {
		return "."
	}
	return fmt.Sprintf("apps/group%02d/svc%04d", i%20, i)
}

// BuildMonorepo writes the tree of spec under a fresh temporary directory and
// returns its root. Every scope holds AGENTS.md, .claude/skills/<name>/SKILL.md
// and .claude/rules/<name>.md; the remaining files are small sources spread
// round-robin over the scopes. Files below the three per scope is raised to it.
func BuildMonorepo(tb testing.TB, spec MonorepoSpec) string {
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
	for rel, content := range spec.Extra {
		write(rel, content)
	}
	scopes := max(spec.Scopes, 1)
	body := BenchBody(20)
	written := len(spec.Extra)
	for i := range scopes {
		dir := MonorepoScopeDir(i)
		at := func(rel string) string {
			if dir == "." {
				return rel
			}
			return dir + "/" + rel
		}
		write(at("AGENTS.md"), fmt.Sprintf("# Scope %d\n\nConventions of the %s service.\n%s", i, dir, body))
		write(at(fmt.Sprintf(".claude/skills/skill-%04d/SKILL.md", i)),
			fmt.Sprintf("---\nname: skill-%04d\ndescription: Use when working in scope %d of the synthetic monorepo.\n---\n# Skill %d\n%s", i, i, i, body))
		write(at(fmt.Sprintf(".claude/rules/rule-%04d.md", i)),
			fmt.Sprintf("---\ndescription: Rule %d of the synthetic monorepo.\n---\n# Rule %d\n%s", i, i, body))
		written += 3
	}
	for n := 0; written < spec.Files; n++ {
		dir := MonorepoScopeDir(n % scopes)
		if dir == "." {
			dir = "src"
		} else {
			dir += "/src"
		}
		write(fmt.Sprintf("%s/pkg%03d/file%05d.go", dir, n/scopes%50, n), "package pkg\n\nfunc F() {}\n")
		written++
	}
	if spec.Git {
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
			Git(tb, root, args...)
		}
	}
	return root
}
