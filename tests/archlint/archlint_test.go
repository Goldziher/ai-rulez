// Package archlint keeps ambient authority out of the library packages (issue #229).
//
// A library package under internal/ must not read the working directory, the
// environment or the clock, or start a process, by itself: those arrive through
// injected values (runner.Runner, generator.Env, generator.Clock) so the engine
// can run inside a service. The check is a ratchet, not a ban: the sites that
// exist today are listed in allowlist.txt, a new one fails the test, and an
// entry whose site is gone must be removed, so the list only ever shrinks.
//
// This is the "warn first, error later" stage of the plan: the same rule can move
// into golangci-lint (forbidigo) once the allowlist is empty.
package archlint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// forbidden maps an import path to the selectors that grant ambient authority.
var forbidden = map[string][]string{
	"os": {
		"Getwd", "Chdir", "Getenv", "LookupEnv", "UserHomeDir", "Environ", "ExpandEnv",
		"UserConfigDir", "UserCacheDir", "Hostname", "Executable",
	},
	"time":    {"Now", "Since", "Until"},
	"os/exec": {"Command", "CommandContext", "LookPath"},
}

// exempt are the packages that are allowed to use ambient authority by design:
// the process runner itself, the CLI-only agent and watch features, and test helpers.
var exempt = []string{
	"internal/ambient/",
	"internal/runner/",
	"internal/agents/",
	"internal/watch/",
	"internal/testutil/",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// scan returns "path symbol" -> count for every forbidden selector in non-test files below internal/.
func scan(t *testing.T, root string) map[string]int {
	t.Helper()
	found := map[string]int{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path) //nolint:errcheck // path is below root
		rel = filepath.ToSlash(rel)
		if d.IsDir() || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		for _, prefix := range exempt {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		names := map[string]string{} // local import name -> import path
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value) //nolint:errcheck // a parsed import path is quoted
			if _, ok := forbidden[p]; !ok {
				continue
			}
			local := filepath.Base(p)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			if local == "." {
				// A dot-import hides every selector from the scan, so the import itself is the site.
				found[rel+" ."+filepath.Base(p)]++
				continue
			}
			names[local] = p
		}
		if len(names) == 0 {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Obj != nil { // a local variable that shadows the package name
				return true
			}
			pkg, ok := names[id.Name]
			if !ok {
				return true
			}
			for _, name := range forbidden[pkg] {
				if sel.Sel.Name == name {
					found[rel+" "+id.Name+"."+name]++
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func readAllowlist(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // repository file
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("allowlist line %q: want \"<file> <symbol> <count>\"", line)
		}
		n, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("allowlist line %q: %v", line, err)
		}
		out[fields[0]+" "+fields[1]] = n
	}
	return out
}

func writeAllowlist(t *testing.T, path string, found map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Ambient-authority sites in library packages (see archlint_test.go).\n")
	b.WriteString("# <file> <symbol> <count>. Only ever remove lines or lower counts; a new line needs a review comment.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, found[k])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil { //nolint:gosec // repository file
		t.Fatal(err)
	}
}

func TestLibraryPackagesStayFreeOfAmbientAuthority(t *testing.T) {
	root := repoRoot(t)
	list := filepath.Join(root, "tests", "archlint", "allowlist.txt")
	found := scan(t, root)
	if os.Getenv("UPDATE_ALLOWLIST") != "" {
		writeAllowlist(t, list, found)
		return
	}
	allowed := readAllowlist(t, list)
	var problems []string
	for key, n := range found {
		if n > allowed[key] {
			problems = append(problems, fmt.Sprintf("NEW  %s: %d site(s), allowlist has %d: inject the dependency (runner.Runner, Env, Clock) instead", key, n, allowed[key]))
		}
	}
	for key, n := range allowed {
		if found[key] < n {
			problems = append(problems, fmt.Sprintf("STALE %s: allowlist has %d, found %d: lower or remove the entry (UPDATE_ALLOWLIST=1)", key, n, found[key]))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d problem(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// TestScannerFlagsASeededViolation proves the scanner sees a forbidden call, so an
// empty allowlist cannot be mistaken for a scanner that finds nothing.
func TestScannerFlagsASeededViolation(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "internal", "seeded")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package seeded\n\nimport (\n\t\"os\"\n\t\"os/exec\"\n\tstdtime \"time\"\n)\n\n" +
		"func F() {\n\t_, _ = os.Getwd()\n\t_ = stdtime.Now()\n\t_ = os.Getenv(\"X\")\n\t_ = os.ExpandEnv(\"$X\")\n" +
		"\t_, _ = os.UserConfigDir()\n\t_, _ = os.UserCacheDir()\n\t_, _ = os.Hostname()\n\t_, _ = os.Executable()\n" +
		"\t_, _ = exec.LookPath(\"x\")\n\t_ = stdtime.Since(stdtime.Time{})\n\t_ = stdtime.Until(stdtime.Time{})\n}\n"
	dot := "package seeded\n\nimport . \"os\"\n\nfunc G() string { return Getenv(\"X\") }\n"
	if err := os.WriteFile(filepath.Join(pkg, "dot.go"), []byte(dot), 0o644); err != nil { //nolint:gosec // scratch file
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "seeded.go"), []byte(src), 0o644); err != nil { //nolint:gosec // scratch file
		t.Fatal(err)
	}
	got := scan(t, dir)
	want := map[string]int{
		"internal/seeded/seeded.go os.Getwd":         1,
		"internal/seeded/seeded.go stdtime.Now":      1,
		"internal/seeded/seeded.go os.Getenv":        1,
		"internal/seeded/seeded.go os.ExpandEnv":     1,
		"internal/seeded/seeded.go os.UserConfigDir": 1,
		"internal/seeded/seeded.go os.UserCacheDir":  1,
		"internal/seeded/seeded.go os.Hostname":      1,
		"internal/seeded/seeded.go os.Executable":    1,
		"internal/seeded/seeded.go exec.LookPath":    1,
		"internal/seeded/seeded.go stdtime.Since":    1,
		"internal/seeded/seeded.go stdtime.Until":    1,
		"internal/seeded/dot.go .os":                 1, // a dot-import hides every selector, so the import itself is the site
	}
	if len(got) != len(want) {
		t.Fatalf("scan found %v, want %v", got, want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("scan found %v, want %v", got, want)
		}
	}
}
