package lint

import (
	"fmt"
	"strings"
	"testing"
)

// ruleCase is one project and the findings it must (and must not) produce.
type ruleCase struct {
	name    string
	config  string            // appended to the base config
	presets string            // replaces the preset list, e.g. `"codex", "cursor"`
	files   map[string]string // extra files, relative to the project
	skill   string            // SKILL.md of skill "bad" (when non-empty)
	want    []string          // "CODE:file-suffix:line" (line 0 = any)
	absent  []string          // codes that must not appear
	sev     map[string]Severity
}

func (c ruleCase) project() map[string]string {
	base := baseConfig
	if c.presets != "" {
		base = strings.Replace(base, `presets = ["claude"]`, "presets = ["+c.presets+"]", 1)
	}
	files := map[string]string{".ai-rulez/config.toml": base + c.config}
	if c.skill != "" {
		files[".ai-rulez/skills/bad/SKILL.md"] = c.skill
	}
	for k, v := range c.files {
		files[k] = v
	}
	return files
}

func runRuleCases(t *testing.T, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, tc.project())
			gitAdd(t, root)
			fs := lintDir(t, root)
			for _, w := range tc.want {
				parts := strings.Split(w, ":")
				line := 0
				if len(parts) > 2 {
					_, _ = fmt.Sscanf(parts[2], "%d", &line)
				}
				if !has(fs, parts[0], parts[1], line) {
					t.Errorf("missing finding %s\n%s", w, dump(fs))
				}
				for _, f := range fs {
					if f.Code == parts[0] && strings.HasSuffix(f.File, parts[1]) && (line == 0 || f.Line == line) {
						if s, ok := tc.sev[parts[0]]; ok && f.Severity != s {
							t.Errorf("%s severity = %s, want %s", w, f.Severity, s)
						}
						break
					}
				}
			}
			for _, code := range tc.absent {
				if n := countCode(fs, code); n > 0 {
					t.Errorf("unexpected %s (%d)\n%s", code, n, dump(fs))
				}
			}
		})
	}
}

// skillDoc builds a SKILL.md with the given extra frontmatter lines and body.
func skillDoc(front, body string) string {
	return "---\nname: bad\ndescription: Use when testing the lint rules of a skill.\n" + front + "---\n" + body
}
