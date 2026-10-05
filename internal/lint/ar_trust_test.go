package lint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPublisherParsing(t *testing.T) {
	tests := []struct {
		text, source string
		mismatch     bool
	}{
		{"Official helper by Acme Corp, source github.com/randomuser/skills", "github.com/randomuser/skills", true},
		{"Convert data from CSV", "github.com/randomuser/skills", false},
		{"by Acme", "github.com/acme-inc/skills", false},
		{"Made by @acme", "https://github.com/acme/skills.git", false},
		{"Maintained by Design Team", "git@github.com:other-org/skills", true},
		{"no publisher at all", "github.com/x/y", false},
		{"by design, this is fast", "github.com/x/y", false},
	}
	for _, tc := range tests {
		owner := sourceOwner(tc.source)
		claimed := claimedPublisher(tc.text)
		got := claimed != "" && owner != "" && !publisherMatches(claimed, owner)
		if got != tc.mismatch {
			t.Errorf("%q from %q: mismatch=%v (claimed %q owner %q), want %v", tc.text, tc.source, got, claimed, owner, tc.mismatch)
		}
	}
	if sourceOwner("./local/skills") != "" || sourceOwner("/abs/path") != "" {
		t.Error("local sources have no owner")
	}
}

// trustRun builds a runner around hand-made installed skills, since fetching them is out of scope here.
func trustRun(t *testing.T, name, source, desc string) []Finding {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".ai-rulez/config.toml": baseConfig + "\n[[installed_skills]]\nname = \"" + name + "\"\nsource = \"" + source + "\"\n"})
	cfg, err := config.LoadConfig(context.Background(), dir)
	if err != nil {
		t.Skipf("config with an installed skill does not load offline: %v", err)
	}
	cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: name, Source: source}}
	tree, err := LoadTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}}
	r.cwd = dir
	r.resolveSettings()
	r.items = []item{{kind: kindSkill, abs: filepath.Join(dir, "cache", name, "SKILL.md"), cf: config.ContentFile{Name: name, Path: filepath.Join(dir, "cache", name, "SKILL.md"), Metadata: &config.Metadata{Extra: map[string]string{"description": desc, "name": name}}}}}
	checkInstalledTrust(r)
	return r.findings
}

func TestPublisherMismatchAR032(t *testing.T) {
	fs := trustRun(t, "helper", "github.com/randomuser/skills", "Official helper by Acme Corp for deploys")
	if countCode(fs, CodePublisherMismatch) != 1 || countCode(fs, CodeAuthorityClaim) != 1 {
		t.Errorf("want one AR032 and one AR033\n%s", dump(fs))
	}
	fs = trustRun(t, "helper", "github.com/anthropics/skills", "Official helper for deploys")
	if len(fs) != 0 {
		t.Errorf("a trusted org may say official\n%s", dump(fs))
	}
	fs = trustRun(t, "helper", "github.com/acme-inc/skills", "Deploy helper by Acme")
	if len(fs) != 0 {
		t.Errorf("matching publisher\n%s", dump(fs))
	}
	fs = trustRun(t, "helper", "./local/skills", "Official helper by Acme Corp")
	if len(fs) != 0 {
		t.Errorf("local sources are skipped\n%s", dump(fs))
	}
}

func TestLowAnalyzabilityAR034(t *testing.T) {
	body := skillDoc("", "x\n")
	runRuleCases(t, []ruleCase{
		{name: "wasm dominates", skill: body, files: map[string]string{".ai-rulez/skills/bad/assets/tool.wasm": strings.Repeat("\x00asm", 2000)}, want: []string{"AR034:SKILL.md:2"}},
		{name: "zip archive", skill: body, files: map[string]string{".ai-rulez/skills/bad/assets/data.zip": strings.Repeat("PK", 3000)}, want: []string{"AR034:SKILL.md:2"}},
		{name: "small png next to text", skill: body + strings.Repeat("text line\n", 500), files: map[string]string{".ai-rulez/skills/bad/assets/i.png": strings.Repeat("\x89PNG", 50)}, absent: []string{"AR034"}},
		{name: "only text", skill: body, files: map[string]string{".ai-rulez/skills/bad/references/a.md": "# A\n"}, absent: []string{"AR034"}},
	})
}

func TestBodyEmptyAR805(t *testing.T) {
	runRuleCases(t, []ruleCase{
		{name: "frontmatter only", skill: "---\nname: bad\ndescription: Use when testing the lint rules of a skill.\n---\n", want: []string{"AR805:SKILL.md:4"}},
		{name: "whitespace body", skill: "---\nname: bad\ndescription: Use when testing the lint rules of a skill.\n---\n\n  \n", want: []string{"AR805:SKILL.md:4"}},
		{name: "one paragraph", skill: skillDoc("", "Do the thing.\n"), absent: []string{"AR805"}},
		{name: "empty agent", files: map[string]string{".ai-rulez/agents/a.md": "---\ndescription: Reviews code changes for the billing team carefully.\n---\n"}, want: []string{"AR805:a.md:3"}},
		{name: "empty rule", files: map[string]string{".ai-rulez/rules/r.md": "---\npriority: high\n---\n"}, want: []string{"AR805:r.md:3"}},
	})
}

func TestDescriptionNearLimitAR802(t *testing.T) {
	near := "Use when " + strings.Repeat("a", 940)
	long := "Use when " + strings.Repeat("a", 1100)
	runRuleCases(t, []ruleCase{
		{name: "near the limit", skill: "---\nname: bad\ndescription: " + near + "\n---\nx\n", want: []string{"AR802:SKILL.md:3"}},
		{name: "well under", skill: "---\nname: bad\ndescription: Use when " + strings.Repeat("a", 290) + "\n---\nx\n", absent: []string{"AR802"}},
		{name: "over is the existing finding", skill: "---\nname: bad\ndescription: " + long + "\n---\nx\n", want: []string{"AR802:SKILL.md:3"}},
		{name: "scales with max_length", config: "\n[lint.description]\nmax_length = 200\n", skill: "---\nname: bad\ndescription: Use when " + strings.Repeat("a", 175) + "\n---\nx\n", want: []string{"AR802:SKILL.md:3"}},
	})
}
