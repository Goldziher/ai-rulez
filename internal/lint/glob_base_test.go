package lint

import (
	"testing"
)

func TestGlobNoMatchNestedClaudeRulesBase(t *testing.T) {
	rule := func(glob string) string { return "---\npaths:\n  - \"" + glob + "\"\n---\nRule body.\n" }
	tests := []struct {
		name  string
		files map[string]string
		rule  string
		want  bool
	}{
		{
			name:  "dir-relative glob matches under the nested dir",
			files: map[string]string{".github/workflows/ci.yml": "on: push\n", ".github/.claude/rules/ci.md": rule("workflows/**")},
			rule:  ".github/.claude/rules/ci.md",
			want:  false,
		},
		{
			name:  "root-relative glob does not match the root tree",
			files: map[string]string{"docs/a.md": "x\n", ".github/.claude/rules/docs.md": rule("docs/**")},
			rule:  ".github/.claude/rules/docs.md",
			want:  true,
		},
		{
			name:  "sub/.claude/rules matches sub/src",
			files: map[string]string{"sub/src/x.py": "x = 1\n", "sub/.claude/rules/r.md": rule("src/**")},
			rule:  "sub/.claude/rules/r.md",
			want:  false,
		},
		{
			name:  "repo-root-relative glob does not match under the nested dir",
			files: map[string]string{"sub/src/x.py": "x = 1\n", "sub/.claude/rules/r.md": rule("sub/src/**")},
			rule:  "sub/.claude/rules/r.md",
			want:  true,
		},
		{
			name:  "root .claude/rules is unchanged",
			files: map[string]string{"docs/a.md": "x\n", ".claude/rules/docs.md": rule("docs/**")},
			rule:  ".claude/rules/docs.md",
			want:  false,
		},
		{
			name:  "root .claude/rules reports a dead glob",
			files: map[string]string{"docs/a.md": "x\n", ".claude/rules/gone.md": rule("nothing/**")},
			rule:  ".claude/rules/gone.md",
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			tc.files[".ai-rulez/config.toml"] = baseConfig
			writeFiles(t, root, tc.files)
			gitAdd(t, root)

			// Act
			got, _ := lintWithTargets(t, root, TargetSpec{Detect: true})

			// Assert
			if has(got, CodeGlobNoMatch, tc.rule, 0) != tc.want {
				t.Fatalf("AR101 on %s: want %v\n%s", tc.rule, tc.want, dump(got))
			}
		})
	}
}
