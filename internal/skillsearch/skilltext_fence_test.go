package skillsearch

import "testing"

// A line that only starts with dashes must not close the frontmatter block: the
// shared frontmatter rules require a line that is exactly "---".
func TestSplitSkillIgnoresLongerDashRunInFrontmatter(t *testing.T) {
	front, body := SplitSkill([]byte("---\nname: demo\ndescription: |\n  text\n  ----\n  more\n---\nBody\n"))
	if front["name"] != "demo" {
		t.Fatalf("name = %v, want demo", front["name"])
	}
	if body != "Body" {
		t.Fatalf("body = %q, want Body", body)
	}
}

func TestSplitSkillEmptyFrontmatterBlock(t *testing.T) {
	front, body := SplitSkill([]byte("---\n---\nText\n\n---\n\nrule above\n"))
	if len(front) != 0 {
		t.Fatalf("front = %v, want empty", front)
	}
	if body != "Text\n\n---\n\nrule above" {
		t.Fatalf("body = %q", body)
	}
}
