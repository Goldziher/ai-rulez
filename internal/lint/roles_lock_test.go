package lint

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func rolesFixture() map[string]string {
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Do the " + name + " thing for the team. Use when the " + name + " work comes up.\n---\nbody\n"
	}
	return map[string]string{
		".ai-rulez/config.toml": baseConfig + `
[profiles]
backend = ["backend"]

[[roles]]
name = "dev"
domains = ["backend", "ghost"]
[roles.skills]
exclude = ["migrate"]

[[roles]]
name = "loop-a"
extends = "loop-b"

[[roles]]
name = "loop-b"
extends = "loop-a"

[[roles]]
name = "fine"
domains = ["backend"]
`,
		".ai-rulez/domains/backend/skills/migrate/SKILL.md": skill("migrate"),
		".ai-rulez/domains/backend/skills/deploy/SKILL.md":  skill("deploy"),
		".ai-rulez/domains/backend/rules/db.md":             "---\nskills:\n  - migrate\n---\n# DB\n",
	}
}

func TestRoleCodes(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, rolesFixture())
	gitAdd(t, root)
	findings := lintDir(t, root)
	for _, code := range []string{CodeRoleReferenceUnknown, CodeRoleExtendsInvalid, CodeRoleUnreachable} {
		if countCode(findings, code) == 0 {
			t.Errorf("expected %s in %v", code, findings)
		}
	}
	if countCode(findings, CodeRoleExtendsInvalid) != 2 {
		t.Errorf("both loop roles should be reported: %v", findings)
	}
	for _, f := range findings {
		if f.Code == CodeRoleUnreachable && f.Line < 2 {
			t.Errorf("finding is not anchored to the role entry: %+v", f)
		}
	}
}

func TestLockDriftCodes(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	cfg, err := config.LoadConfig(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree, WithLockDrift([]LockDrift{
		{Path: ".ai-rulez/rules/a.md", Message: "rule a changed"},
		{Output: true, Path: "CLAUDE.md", Message: "CLAUDE.md changed"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if countCode(rep.Findings, CodeLockSourceDrift) != 1 || countCode(rep.Findings, CodeLockOutputDrift) != 1 {
		t.Fatalf("want one AR981 and one AR982: %v", rep.Findings)
	}
}
