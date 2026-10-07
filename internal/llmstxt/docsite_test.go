package llmstxt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const testToml = `
[project]
site_name = "Demo Docs"
site_description = "Docs for demo."
site_url = "https://example.dev/demo/"
nav = [
  { "Home" = "index.md" },
  { "Install" = "install.md" },
  { "Guides" = [
    { "Rules" = "rules.md" },
    { "Deep Dive" = "guides/deep.md" },
  ] },
  { "Proposals" = [ { "Idea" = "proposals/idea.md" } ] },
  { "Changelog" = "CHANGELOG.md" },
]
`

func testDocs() fstest.MapFS {
	return fstest.MapFS{
		"index.md":          {Data: []byte("# Demo\n\n<p align=\"center\"><img src=\"x.png\"/></p>\n\nDemo is a **tool** that does [things](rules.md) well. It also does more.\n\n## Next\n")},
		"install.md":        {Data: []byte("---\ndescription: How to install it.\n---\n# Install\n\nIgnored when frontmatter has a description.\n")},
		"rules.md":          {Data: []byte("# Rules\n\n!!! note\n    Skip me\n\n```sh\nnot prose\n```\n\nWrite `rules` as\nmarkdown files.\n\n## Sub\n\ntext\n")},
		"guides/deep.md":    {Data: []byte("# Deep\n\n- only a list\n")},
		"proposals/idea.md": {Data: []byte("# Idea\n\nAn idea.\n")},
		"CHANGELOG.md":      {Data: []byte("# Changelog\n\n## 1.0\n\nFirst.\n")},
	}
}

func TestBuildDocs(t *testing.T) {
	site, err := ParseSite([]byte(testToml))
	if err != nil {
		t.Fatal(err)
	}
	index, full, err := BuildDocs(site, testDocs())
	if err != nil {
		t.Fatal(err)
	}
	wantIndex := "# Demo Docs\n\n> Docs for demo.\n\n" +
		"## Docs\n\n" +
		"- [Home](https://example.dev/demo/): Demo is a tool that does things well.\n" +
		"- [Install](https://example.dev/demo/install/): How to install it.\n\n" +
		"## Guides\n\n" +
		"- [Rules](https://example.dev/demo/rules/): Write rules as markdown files.\n" +
		"- [Deep Dive](https://example.dev/demo/guides/deep/)\n\n" +
		"## Optional\n\n" +
		"- [Full documentation](https://example.dev/demo/llms-full.txt): Every page above concatenated into one file.\n" +
		"- [Idea](https://example.dev/demo/proposals/idea/): An idea.\n" +
		"- [Changelog](https://example.dev/demo/CHANGELOG/): First.\n"
	if index != wantIndex {
		t.Fatalf("index:\n%s\nwant:\n%s", index, wantIndex)
	}
	if fs := Validate([]byte(index)); len(fs) != 0 {
		t.Fatalf("index is not valid llms.txt: %v", fs)
	}
	for _, want := range []string{
		"## Home\n\nSource: https://example.dev/demo/\n\n",
		"## Deep Dive\n\nSource: https://example.dev/demo/guides/deep/\n\n- only a list\n",
		"## Idea\n",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full lacks %q:\n%s", want, full)
		}
	}
	if strings.Contains(full, "First.") || strings.Contains(full, "description: How") {
		t.Errorf("full holds the changelog or frontmatter:\n%s", full)
	}
	index2, full2, _ := BuildDocs(site, testDocs())
	if index != index2 || full != full2 {
		t.Fatal("not deterministic")
	}
}

func TestBuildDocsMissingPage(t *testing.T) {
	site, _ := ParseSite([]byte(testToml))
	docs := testDocs()
	delete(docs, "rules.md")
	if _, _, err := BuildDocs(site, docs); err == nil || !strings.Contains(err.Error(), "rules.md") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSiteErrors(t *testing.T) {
	for name, src := range map[string]string{
		"no site url": "[project]\nsite_name = \"x\"\nnav = []\n",
		"no name":     "[project]\nsite_url = \"https://x.dev\"\nnav = []\n",
		"bad toml":    "[project",
	} {
		if _, err := ParseSite([]byte(src)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestClipWords(t *testing.T) {
	if got := clipWords("short", 10); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := clipWords("one two three four five", 15); got != "one two..." {
		t.Fatalf("got %q", got)
	}
}

// TestCheckedInDocsFilesAreFresh fails when docs/llms.txt or docs/llms-full.txt
// differ from the docs sources. Regenerate with `task docs:llms`.
func TestCheckedInDocsFilesAreFresh(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "zensical.toml"))
	if err != nil {
		t.Fatal(err)
	}
	site, err := ParseSite(raw)
	if err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(root, "docs")
	index, full, err := BuildDocs(site, os.DirFS(docs))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"llms.txt": index, "llms-full.txt": full} {
		have, readErr := os.ReadFile(filepath.Join(docs, name))
		if readErr != nil || string(have) != want {
			t.Errorf("docs/%s is stale: run `task docs:llms`", name)
		}
	}
	if fs := Validate([]byte(index)); len(fs) != 0 {
		t.Errorf("docs/llms.txt is not valid: %v", fs)
	}
}
