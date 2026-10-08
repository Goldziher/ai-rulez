package golden

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The OKF layout of .ai-rulez/ (issue #281) must not move a single output byte:
// a tree migrated with "migrate okf" generates exactly what the native tree did.

func okfProject(t *testing.T, files map[string]string) (dir, home string) {
	t.Helper()
	dir, home = t.TempDir(), t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if scriptExec()[name] || strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil { //nolint:gosec // a fixture
			t.Fatal(err)
		}
	}
	return dir, home
}

func okfRun(t *testing.T, dir, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary(t), args...) //nolint:gosec // the binary was just built
	cmd.Dir = dir
	cmd.Env = baseEnv(home, goldenEnv)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ai-rulez %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// treeSnapshot reads every file below dir, skipping the given top-level names.
func treeSnapshot(t *testing.T, dir string, skip ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p) //nolint:errcheck // p is below dir
		rel = filepath.ToSlash(rel)
		for _, s := range skip {
			if rel == s || strings.HasPrefix(rel, s+"/") {
				return fs.SkipDir
			}
		}
		if d.IsDir() || rel == "." {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func diffSnapshots(t *testing.T, label string, want, got map[string]string) {
	t.Helper()
	for name, w := range want {
		g, ok := got[name]
		switch {
		case !ok:
			t.Errorf("%s: %s is missing", label, name)
		case g != w:
			t.Errorf("%s: %s differs\n--- native\n%s\n--- okf\n%s", label, name, w, g)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s: unexpected file %s", label, name)
		}
	}
}

func TestOKFNativeTreeGeneratesTheSameOutput(t *testing.T) {
	projects := map[string]map[string]string{
		"rich":    richFiles([]string{"claude", "cursor", "codex", "gemini"}, ""),
		"domains": domainFiles([]string{"claude", "cursor", "codex"}),
	}
	for name, files := range projects {
		t.Run(name, func(t *testing.T) {
			native, nativeHome := okfProject(t, files)
			okfDir, okfHome := okfProject(t, files)

			okfRun(t, okfDir, okfHome, "migrate", "okf")
			migrated := treeSnapshot(t, okfDir)
			if migrated[".ai-rulez/index.md"] == "" || !strings.Contains(migrated[".ai-rulez/rules/always.md"], "x-ai-rulez:") {
				t.Fatalf("the tree was not converted:\n%v", migrated[".ai-rulez/rules/always.md"])
			}

			okfRun(t, okfDir, okfHome, "migrate", "okf") // idempotent
			diffSnapshots(t, "second migrate", migrated, treeSnapshot(t, okfDir))

			okfRun(t, native, nativeHome, "generate", "--yes")
			okfRun(t, okfDir, okfHome, "generate", "--yes")
			diffSnapshots(t, "generate", treeSnapshot(t, native, ".ai-rulez"), treeSnapshot(t, okfDir, ".ai-rulez"))
			okfRun(t, okfDir, okfHome, "generate", "--check")

			okfRun(t, okfDir, okfHome, "clean", "--yes")
			after := treeSnapshot(t, okfDir, ".ai-rulez/lock")
			for rel, body := range migrated {
				if !strings.HasPrefix(rel, ".ai-rulez/") {
					continue
				}
				if after[rel] != body {
					t.Errorf("clean changed %s", rel)
				}
			}
			// The fixtures trip unrelated lint rules; only the OKF findings count.
			out := okfRun(t, okfDir, okfHome, "validate", "--fail-on", "none")
			if !strings.Contains(out, " 0 errors, 0 warnings, 0 info (OKF spec") {
				t.Errorf("okf validate is not clean:\n%s", out)
			}
		})
	}
}
