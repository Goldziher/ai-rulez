package lint

import "testing"

func evalsFixture(extraConfig string) map[string]string {
	skill := func(name string) string {
		return "---\nname: " + name + "\ndescription: Deploy the billing service to staging. Use when releasing billing changes to the staging cluster.\n---\nbody\n"
	}
	return map[string]string{
		".ai-rulez/config.toml":                     baseConfig + extraConfig,
		".ai-rulez/skills/with-own/SKILL.md":        skill("with-own"),
		".ai-rulez/skills/with-own/evals/case.yaml": "prompt: hi\n",
		".ai-rulez/skills/with-shared/SKILL.md":     skill("with-shared"),
		".ai-rulez/evals/with-shared/case.yaml":     "prompt: hi\n",
		".ai-rulez/skills/bare/SKILL.md":            skill("bare"),
		".ai-rulez/skills/empty/SKILL.md":           skill("empty"),
		".ai-rulez/skills/empty/evals/.gitkeep":     "",
		".ai-rulez/skills/exempt-one/SKILL.md":      skill("exempt-one"),
	}
}

func TestEvalsMissing(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		missing []string
		absent  []string
	}{
		{name: "off by default", config: "", absent: []string{"bare", "empty"}},
		{
			name:    "require",
			config:  "[lint.evals]\nrequire = true\n",
			missing: []string{"bare", "empty", "exempt-one"},
			absent:  []string{"with-own", "with-shared"},
		},
		{
			name:    "allow list",
			config:  "[lint.evals]\nrequire = true\nallow = [\"exempt-*\", \"empty\"]\n",
			missing: []string{"bare"},
			absent:  []string{"exempt-one", "empty", "with-own"},
		},
		{
			name:    "severity by name",
			config:  "[lint.severity]\nevals-missing = \"error\"\n",
			missing: []string{"bare", "empty", "exempt-one"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, evalsFixture(tt.config))
			gitAdd(t, root)
			findings := lintDir(t, root)
			for _, name := range tt.missing {
				if !has(findings, CodeEvalsMissing, "skills/"+name+"/SKILL.md", 0) {
					t.Errorf("expected %s for %s, got %v", CodeEvalsMissing, name, findings)
				}
			}
			for _, name := range tt.absent {
				if has(findings, CodeEvalsMissing, "skills/"+name+"/SKILL.md", 0) {
					t.Errorf("unexpected %s for %s", CodeEvalsMissing, name)
				}
			}
			if len(tt.missing) == 0 && countCode(findings, CodeEvalsMissing) != 0 {
				t.Errorf("rule should be off, got %v", findings)
			}
		})
	}
}
