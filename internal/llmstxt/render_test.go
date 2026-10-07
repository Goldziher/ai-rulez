package llmstxt

import "testing"

func TestRenderGolden(t *testing.T) {
	d := Doc{
		Title:   "Demo",
		Summary: "A demo\nproject.",
		Details: "Read the rules first.",
		Sections: []Section{
			{Name: "Optional", Links: []Link{{Title: "Extra", URL: "extra.md"}}},
			{Name: "Rules", Links: []Link{
				{Title: "Style [x]", URL: "rules/style.md", Note: "Code\nstyle."},
				{Title: "Plain", URL: "rules/a b.md"},
			}},
			{Name: "Empty"},
		},
	}
	want := "# Demo\n\n> A demo project.\n\nRead the rules first.\n\n## Rules\n\n" +
		"- [Style \\[x\\]](rules/style.md): Code style.\n- [Plain](rules/a%20b.md)\n\n" +
		"## Optional\n\n- [Extra](extra.md)\n"
	got := d.Render()
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if fs := Validate([]byte(got)); len(fs) != 0 {
		t.Fatalf("rendered output invalid: %v", fs)
	}
	if again := d.Render(); again != got {
		t.Fatal("render is not deterministic")
	}
}

func TestRenderTitleOnly(t *testing.T) {
	if got := (Doc{Title: " X "}).Render(); got != "# X\n" {
		t.Fatalf("got %q", got)
	}
	if got := (Doc{}).Render(); got != "# Untitled\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderFull(t *testing.T) {
	got := RenderFull("Demo", "Sum", []Page{
		{Title: "One", Source: "a.md", Body: "# Head\n\ntext\n\n```\n# keep\n```\n\n## Sub\n"},
		{Title: "Two", Body: "plain"},
	})
	want := "# Demo\n\n> Sum\n\n## One\n\nSource: a.md\n\n### Head\n\ntext\n\n```\n# keep\n```\n\n#### Sub\n\n## Two\n\nplain\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
