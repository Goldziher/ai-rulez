package lint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

type fixedEmbedder struct{ model string }

func (f fixedEmbedder) Embed(_ context.Context, texts []string) (skillsearch.Embedding, error) {
	out := skillsearch.Embedding{}
	for i := range texts {
		out.Vectors = append(out.Vectors, []float32{1, float32(i + 1)})
	}
	return out, nil
}
func (f fixedEmbedder) Fingerprint() string { return "test@local" }
func (f fixedEmbedder) Model() string       { return f.model }

const searchSkill = "---\nname: deploy\ndescription: Deploy a service\n---\nbody\n"

func searchProject(t *testing.T, searchTable string, skill string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":            baseConfig + "\n" + searchTable,
		".ai-rulez/skills/deploy/SKILL.md": skill,
	})
	return root
}

func TestSearchConfigInvalid(t *testing.T) {
	root := searchProject(t, "[search]\nmode = \"semantic\"\nindex_dir = \"../out\"\n", searchSkill)
	gitAdd(t, root)

	fs := lintDir(t, root)

	if n := countCode(fs, CodeSearchConfigInvalid); n != 2 {
		t.Fatalf("want the mode and the index_dir reported (AR9D0), got %d:\n%s", n, dump(fs))
	}
}

func TestSearchIndexStale(t *testing.T) {
	tests := []struct {
		name   string
		table  string
		build  func(t *testing.T, root string)
		edit   string
		want   int
		inText string
	}{
		{"fresh committed index is clean", "[search]\nindex_dir = \"search-index\"\n", buildIndex("m"), "", 0, ""},
		{"edited description", "[search]\nindex_dir = \"search-index\"\n", buildIndex("m"), "---\nname: deploy\ndescription: Deploy a service to prod\n---\nbody\n", 1, "changed: deploy"},
		{"no index in a committed dir", "[search]\nindex_dir = \"search-index\"\n", nil, "", 1, "holds no index"},
		{"model changed", "[search]\nindex_dir = \"search-index\"\n[llm]\nembedding_model = \"other\"\n", buildIndex("m"), "", 1, "model changed"},
		{"machine-local index is never a finding", "[search]\n", nil, "", 0, ""},
		{"no search table", "", nil, "", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := searchProject(t, tt.table, searchSkill)
			if tt.build != nil {
				tt.build(t, root)
			}
			if tt.edit != "" {
				writeFiles(t, root, map[string]string{".ai-rulez/skills/deploy/SKILL.md": tt.edit})
			}
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			if n := countCode(fs, CodeSearchIndexStale); n != tt.want {
				t.Fatalf("want %d AR9D1, got %d:\n%s", tt.want, n, dump(fs))
			}
			for _, f := range fs {
				if f.Code == CodeSearchIndexStale && !strings.Contains(f.Message, tt.inText) {
					t.Errorf("message %q lacks %q", f.Message, tt.inText)
				}
			}
		})
	}
}

// buildIndex writes a committed index of the fixture skill, embedded with model.
func buildIndex(model string) func(t *testing.T, root string) {
	return func(t *testing.T, root string) {
		t.Helper()
		item := skillsearch.ItemFromSkill("deploy", "", []byte(searchSkill))
		res, err := skillsearch.Build(context.Background(), []skillsearch.Item{item}, &skillsearch.BuildOptions{
			Config: skillsearch.Config{IndexDir: "search-index"}, Embedder: fixedEmbedder{model},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := skillsearch.WriteIndex(filepath.Join(root, ".ai-rulez", "search-index"), res.Index); err != nil {
			t.Fatal(err)
		}
	}
}
