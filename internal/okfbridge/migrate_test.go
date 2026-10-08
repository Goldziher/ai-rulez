package okfbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func migrateFixture() map[string]string {
	return map[string]string{
		"rules/go.md":                     "---\npriority: high\npaths: [\"**/*.go\"]\n---\n\n# Go\n\nWrap errors.\n",
		"rules/plain.md":                  "# Plain\n\nNo frontmatter.\n",
		"rules/typed.md":                  "---\ntype: Policy\ntitle: Custom Title\ndescription: Kept\nwhen_to_use: x\n---\nBody\n",
		"context/overview.md":             "---\nsummary: One line\n---\nOverview\n",
		"skills/review/SKILL.md":          "---\nname: review\ndescription: Reviews code\n---\n# Review\n",
		"skills/review/references/doc.md": "# Doc\n",
		"domains/web/rules/react.md":      "---\npriority: low\n---\nHooks.\n",
	}
}

func TestMigrateDirConvertsAndIsIdempotent(t *testing.T) {
	dir := writeTree(t, migrateFixture())
	before := readTree(t, dir)

	changes, err := MigrateDir(dir, MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTree(t, dir); len(got) != len(before) {
		t.Fatal("a dry run wrote files")
	}
	pending := 0
	for _, c := range changes {
		if c.Pending() {
			pending++
		}
	}
	if pending == 0 {
		t.Fatal("dry run reported nothing to do")
	}

	if _, err := MigrateDir(dir, MigrateOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	migrated := readTree(t, dir)
	if _, err := MigrateDir(dir, MigrateOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	if again := readTree(t, dir); !equalTrees(migrated, again) {
		t.Fatalf("second run changed the tree:\n%v\n%v", migrated, again)
	}
	again, err := MigrateDir(dir, MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range again {
		if c.Pending() {
			t.Errorf("still pending after migrate: %+v", c)
		}
	}

	// Bodies are kept byte for byte, the loader sees the same metadata.
	for rel, old := range before {
		if rel == "skills/review/references/doc.md" {
			if migrated[rel] != old {
				t.Errorf("resource %s was rewritten", rel)
			}
			continue
		}
		_, oldBody := config.ParseFrontmatterPublic(old)
		_, newBody := config.ParseFrontmatterPublic(migrated[rel])
		if oldBody != newBody {
			t.Errorf("%s: body changed %q -> %q", rel, oldBody, newBody)
		}
		oldMeta, _ := config.ParseFrontmatterPublic(old)
		newMeta, _ := config.ParseFrontmatterPublic(migrated[rel])
		if oldMeta != nil && (newMeta == nil || oldMeta.Priority != newMeta.Priority || len(oldMeta.Extra) != len(newMeta.Extra)) {
			t.Errorf("%s: metadata changed %#v -> %#v", rel, oldMeta, newMeta)
		}
	}
	if !strings.Contains(migrated["rules/typed.md"], "type: Policy") || !strings.Contains(migrated["rules/typed.md"], "title: Custom Title") {
		t.Errorf("type and title were not kept:\n%s", migrated["rules/typed.md"])
	}
	if !strings.HasPrefix(migrated["rules/plain.md"], "---\ntype: Decision\n") {
		t.Errorf("a file without frontmatter was not converted:\n%s", migrated["rules/plain.md"])
	}
	for _, idx := range []string{"index.md", "rules/index.md", "domains/web/rules/index.md"} {
		if migrated[idx] == "" {
			t.Errorf("missing %s", idx)
		}
	}
	if _, ok := migrated["skills/review/references/index.md"]; ok {
		t.Error("a resource directory got an index")
	}
}

func TestMigrateDirMatchesExport(t *testing.T) {
	files := migrateFixture()
	delete(files, "rules/typed.md") // the loader drops a custom type and title, so export cannot restore them
	dir := writeTree(t, files)
	if _, err := MigrateDir(dir, MigrateOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	tree, err := config.ScanContentTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Export(tree, ExportOptions{LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	migrated := readTree(t, dir)
	for _, f := range res.Files {
		// Skill indexes list resources; a migrated tree leaves them out.
		if !strings.HasSuffix(f.Path, ".md") || strings.HasPrefix(f.Path, "skills/") {
			continue
		}
		if got, ok := migrated[f.Path]; !ok || (strings.HasSuffix(f.Path, "index.md") && got != string(f.Data)) {
			t.Errorf("%s: migrated tree does not reproduce the export", f.Path)
		}
	}
}

func TestMigratedTreeValidates(t *testing.T) {
	dir := writeTree(t, migrateFixture())
	if _, err := MigrateDir(dir, MigrateOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append(b.CheckRoot(), b.Validate()...) {
		if !strings.Contains(f.Path, "/references/") && !strings.Contains(f.Message, "subdirectory references/") {
			t.Errorf("finding: %+v", f)
		}
	}
}

func TestMigrateDirSkipsUnclosedFrontmatter(t *testing.T) {
	dir := writeTree(t, map[string]string{"rules/bad.md": "---\npriority: high\nno close\n"})
	changes, err := MigrateDir(dir, MigrateOptions{Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTree(t, dir)["rules/bad.md"]; got != "---\npriority: high\nno close\n" {
		t.Fatalf("file was rewritten: %q", got)
	}
	for _, c := range changes {
		if c.Path == "rules/bad.md" && c.Action != ActionSkipped {
			t.Errorf("action = %s", c.Action)
		}
	}
}

func equalTrees(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
