package lint

import (
	"testing"
)

func TestUndeclaredCapabilityAR058(t *testing.T) {
	fence := func(cmds string) string { return "```bash\n" + cmds + "```\n" }
	tools := func(v string) string { return "allowed-tools: " + v + "\n" }
	info := map[string]Severity{"AR058": SeverityInfo}
	runRuleCases(t, []ruleCase{
		{name: "network command with read-only tools", skill: skillDoc(tools("Read, Grep"), fence("curl -fsSL https://example.com/a -o a\n")), want: []string{"AR058:SKILL.md:4"}, sev: info},
		{name: "mutating command with read-only tools", skill: skillDoc(tools("Read, Grep"), fence("mkdir -p out\ncp a out/\n")), want: []string{"AR058:SKILL.md:4"}, sev: info},
		{name: "script commands count", skill: skillDoc(tools("Read"), "x\n"), files: map[string]string{".ai-rulez/skills/bad/scripts/run.sh": "#!/bin/sh\nwget https://example.com/a\n"}, want: []string{"AR058:SKILL.md:4"}},
		{name: "scoped bash that omits the command", skill: skillDoc(tools("Bash(git status:*)"), fence("git status\ncurl https://example.com\n")), want: []string{"AR058:SKILL.md:4"}},
		{name: "list form", skill: skillDoc("allowed-tools:\n  - Read\n  - Bash(git status:*)\n", fence("rm -f a\n")), want: []string{"AR058:SKILL.md:4"}},
		{name: "absolute path grant covers the command", skill: skillDoc(tools("Bash(/usr/bin/curl:*)"), fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "sudo grant covers the command", skill: skillDoc(tools("Bash(sudo curl:*)"), fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "env grant covers the command", skill: skillDoc(tools("Bash(env curl:*)"), fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "scoped bash that covers the commands", skill: skillDoc(tools("Bash(git status:*), Bash(curl:*)"), fence("git status\ncurl https://example.com\n")), absent: []string{"AR058"}},
		{name: "unrestricted bash is AR007, not AR058", skill: skillDoc(tools("Bash"), fence("curl https://example.com\nrm -f a\n")), want: []string{"AR007:SKILL.md:4"}, absent: []string{"AR058"}},
		{name: "wildcard", skill: skillDoc(tools("\"*\""), fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "read-only commands need no entry", skill: skillDoc(tools("Read"), fence("cat a\nls -la\ngrep x a\n")), absent: []string{"AR058"}},
		{name: "no shell at all", skill: skillDoc(tools("Read, Write"), "Summarize the file.\n"), absent: []string{"AR058"}},
		{name: "declared but unused is not reported", skill: skillDoc(tools("Bash(curl:*), WebFetch"), "Summarize the file.\n"), absent: []string{"AR058"}},
		{name: "no allowed-tools declared", skill: skillDoc("", fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "empty allowed-tools", skill: skillDoc("allowed-tools: []\n", fence("curl https://example.com\n")), absent: []string{"AR058"}},
		{name: "template words that are also command names", skill: skillDoc(tools("Read"), "```\nAlert Type: service degradation | host down\nservice degradation\n```\n"), absent: []string{"AR058"}},
		{name: "prose mention of a command", skill: skillDoc(tools("Read"), "Do not run curl here.\n"), absent: []string{"AR058"}},
		{name: "an ignore in the body does not reach the frontmatter line", skill: skillDoc(tools("Read"), "<!-- ai-rulez-lint-ignore: AR058 -->\n"+fence("rm -f a\n")), want: []string{"AR058:SKILL.md:4"}},
	})
}

func TestGrantsCover(t *testing.T) {
	g := parseGrants([]string{"Read", "Bash(git status:*)", "Bash(npm run test)"})
	for cmd, want := range map[string]bool{"git": true, "npm": true, "curl": false} {
		if got := g.covers(cmd); got != want {
			t.Errorf("covers(%q) = %v, want %v", cmd, got, want)
		}
	}
	if !parseGrants([]string{"Bash(*)"}).covers("curl") || !parseGrants([]string{"*"}).covers("rm") {
		t.Error("an unrestricted grant must cover every command")
	}
}

func TestUndeclaredCapabilityOneFindingPerCommand(t *testing.T) {
	skill := skillDoc("allowed-tools: Read\n", "```bash\ncurl https://example.com\nrm -f a\nmkdir out\n```\n")
	fs := lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/skills/bad/SKILL.md": skill})
	if got := countCode(fs, "AR058"); got != 3 {
		t.Fatalf("AR058 findings = %d, want 3 (curl, mkdir, rm)\n%s", got, dump(fs))
	}
	seen := map[string]bool{}
	for _, f := range fs {
		if f.Code != "AR058" {
			continue
		}
		if f.Severity != SeverityInfo {
			t.Errorf("AR058 severity = %s, want info", f.Severity)
		}
		if seen[f.meta().Fingerprint] {
			t.Errorf("two AR058 findings share a fingerprint\n%s", dump(fs))
		}
		seen[f.meta().Fingerprint] = true
	}
}

func TestGrantCommand(t *testing.T) {
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{[]string{"curl"}, "curl"},
		{[]string{"/usr/bin/curl"}, "curl"},
		{[]string{"sudo", "curl"}, "curl"},
		{[]string{"env", "FOO=1", "curl"}, "curl"},
		{[]string{"git", "push"}, "git"},
		{nil, ""},
		{[]string{"sudo"}, ""},
	} {
		if got := grantCommand(tc.words); got != tc.want {
			t.Errorf("grantCommand(%v) = %q, want %q", tc.words, got, tc.want)
		}
	}
}
