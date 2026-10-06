package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCheckRoleModes_ReportsDegradedModes(t *testing.T) {
	cfgBody := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\", \"cursor\"]\ngitignore = false\n\n" +
		"[[roles]]\nname = \"r\"\n[roles.skill_mode]\nhidden = \"off\"\n"
	dir := project(t, map[string]string{
		".ai-rulez/config.toml":            cfgBody,
		".ai-rulez/skills/hidden/SKILL.md": "---\nname: hidden\ndescription: Use when hidden.\n---\n# h\n",
	})
	cfg, err := config.LoadConfig(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	got := checkRoleModes(context.Background(), &state{cfg: cfg})

	if len(got) != 1 || got[0].Check != CheckRoleModes || got[0].Severity != SeverityInfo {
		t.Fatalf("findings = %+v, want one info finding for the roles check", got)
	}
	for _, want := range []string{"role r", "hidden", "cursor", "left out"} {
		if !strings.Contains(got[0].Message, want) {
			t.Fatalf("message %q lacks %q", got[0].Message, want)
		}
	}
}

func TestRun_IncludesTheRoleModesCheck(t *testing.T) {
	// Arrange
	cfgBody := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\", \"cursor\"]\ngitignore = false\n\n" +
		"[[roles]]\nname = \"r\"\n[roles.skill_mode]\nhidden = \"off\"\n"
	dir := project(t, map[string]string{
		".ai-rulez/config.toml":            cfgBody,
		".ai-rulez/skills/hidden/SKILL.md": "---\nname: hidden\ndescription: Use when hidden.\n---\n# h\n",
	})

	// Act
	report := run(t, dir)

	// Assert
	if len(byCheck(report, CheckRoleModes)) != 1 {
		t.Fatalf("doctor did not run the role modes check: %+v", report.Findings)
	}
}
