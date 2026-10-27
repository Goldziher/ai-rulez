package okfbridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestStaleIndexesFollowFrontmatterEdits(t *testing.T) {
	skill := func(desc string) string {
		return "---\nname: demo\ndescription: " + desc + "\n---\n\nBody.\n"
	}
	tests := []struct {
		name string
		edit func(t *testing.T, dir string)
		want []string
	}{
		{"untouched bundle is current", func(*testing.T, string) {}, nil},
		{"edited description", func(t *testing.T, dir string) {
			putStaleFile(t, dir, "skills/demo/SKILL.md", skill("New description."))
		}, []string{"skills/demo/index.md"}},
		{"added concept", func(t *testing.T, dir string) {
			putStaleFile(t, dir, "rules/extra.md", "# Extra\n\nText.\n")
		}, []string{"index.md", "rules/index.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			dir := writeTree(t, map[string]string{"skills/demo/SKILL.md": skill("Old description.")})
			if _, err := MigrateDir(ctx, dir, MigrateOptions{Write: true}); err != nil {
				t.Fatal(err)
			}
			tt.edit(t, dir)

			// Act
			got, err := StaleIndexes(ctx, dir)

			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("stale indexes = %v, want %v", got, tt.want)
			}
			changes, err := MigrateDir(ctx, dir, MigrateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var pending []string
			for _, c := range changes {
				if c.Action == ActionIndex {
					pending = append(pending, c.Path)
				}
			}
			if strings.Join(pending, ",") != strings.Join(got, ",") {
				t.Fatalf("migrate okf --check disagrees: %v vs %v", pending, got)
			}
			if err := RefreshIndexes(ctx, dir); err != nil {
				t.Fatal(err)
			}
			if after, err := StaleIndexes(ctx, dir); err != nil || len(after) != 0 {
				t.Fatalf("after a refresh: %v, %v", after, err)
			}
		})
	}
}

// A caller that has loaded the content passes its tree: the index check lists
// the same files as a scan of the directory, and an item merged in from an
// include (a path outside the directory) is not listed.
func TestIndexesFromLoadedTreeMatchTheScan(t *testing.T) {
	// Arrange
	ctx := context.Background()
	dir := writeTree(t, map[string]string{"skills/demo/SKILL.md": "---\nname: demo\ndescription: Old.\n---\n\nBody.\n"})
	if _, err := MigrateDir(ctx, dir, MigrateOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	putStaleFile(t, dir, "skills/demo/SKILL.md", "---\nname: demo\ndescription: New.\n---\n\nBody.\n")
	putStaleFile(t, dir, "rules/extra.md", "# Extra\n\nText.\n")
	tree, err := config.ScanContentTreeContext(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	tree.Rules = append(tree.Rules, config.ContentFile{
		Name: "shared", Path: filepath.Join(t.TempDir(), "rules", "shared.md"), Include: "shared", Content: "# Shared\n",
	})

	// Act
	scanned, err := StaleIndexes(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := StaleIndexesFrom(ctx, dir, tree)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(scanned) == 0 || strings.Join(loaded, ",") != strings.Join(scanned, ",") {
		t.Fatalf("stale from the loaded tree = %v, from the scan = %v", loaded, scanned)
	}
	if err := RefreshIndexesFrom(ctx, dir, tree); err != nil {
		t.Fatal(err)
	}
	after, err := StaleIndexes(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("indexes still stale after a refresh from the loaded tree: %v", after)
	}
}

func TestIsBundleDir(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"no index", map[string]string{"rules/a.md": "# A\n"}, false},
		{"prose index", map[string]string{"index.md": "Welcome to my notes.\n"}, false},
		{"generated listing", map[string]string{"index.md": "---\nokf_version: \"0.2\"\n---\n\n# Index\n\n- [A](rules/a.md) - x\n"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBundleDir(writeTree(t, tt.files)); got != tt.want {
				t.Fatalf("IsBundleDir = %v, want %v", got, tt.want)
			}
		})
	}
}

func putStaleFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
