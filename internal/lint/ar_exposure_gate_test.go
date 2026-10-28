package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenFront is a frontmatter block whose description holds an unquoted ": ",
// which YAML cannot parse, followed by the given lines.
func brokenFront(name, lines string) string {
	return "---\nname: " + name + "\ndescription: Use when: testing the gate\n" + lines + "---\nBody.\n"
}

func TestExposureGateUnreadableAR9W4(t *testing.T) {
	tests := []struct {
		name     string
		presets  string
		files    map[string]string
		wantFile string
		wantLine int
		inText   []string
		// typed marks a block that parses but holds a gate value of the wrong type, which AR306 does not report.
		typed bool
	}{
		{
			name:     "an authored skill naming paths",
			files:    map[string]string{".ai-rulez/skills/bad/SKILL.md": brokenFront("bad", "paths: [src/**]\n")},
			wantFile: "skills/bad/SKILL.md", wantLine: 4,
			inText: []string{`skill "bad"`, `"paths"`, "cannot be known"},
		},
		{
			name:     "an authored skill naming disable-model-invocation",
			files:    map[string]string{".ai-rulez/skills/bad/SKILL.md": brokenFront("bad", "disable-model-invocation: true\n")},
			wantFile: "skills/bad/SKILL.md", wantLine: 4,
			inText: []string{`"disable-model-invocation"`},
		},
		{
			name:     "every gate key is named, the line is the first",
			files:    map[string]string{".ai-rulez/skills/bad/SKILL.md": brokenFront("bad", "user-invocable: false\npaths: [src/**]\n")},
			wantFile: "skills/bad/SKILL.md", wantLine: 4,
			inText: []string{`"user-invocable", "paths"`},
		},
		{
			name:     "a hand-written Claude Code skill",
			files:    map[string]string{".claude/skills/native/SKILL.md": brokenFront("native", "paths:\n  - \"src/**\"\n")},
			wantFile: ".claude/skills/native/SKILL.md", wantLine: 4,
		},
		{
			name:     "an authored rule naming paths",
			files:    map[string]string{".ai-rulez/rules/scoped.md": "---\ndescription: Use when: x\npaths: [\"**/*.go\"]\n---\n# Scoped\n"},
			wantFile: "rules/scoped.md", wantLine: 3,
		},
		{
			name:     "an authored command naming disable-model-invocation",
			files:    map[string]string{".ai-rulez/commands/ship.md": "---\ndescription: Use when: shipping\ndisable-model-invocation: true\n---\nShip it.\n"},
			wantFile: "commands/ship.md", wantLine: 3,
		},
		{
			name:     "a block that never closes",
			files:    map[string]string{".ai-rulez/rules/open.md": "---\npaths: [\"**/*.go\"]\n\n# Open\nbody\n"},
			wantFile: "rules/open.md", wantLine: 2,
		},
		{
			name:     "paths that is not a string or a list of strings",
			files:    map[string]string{".claude/skills/bad/SKILL.md": "---\nname: bad\ndescription: Use when testing the gate.\npaths: 5\n---\nBody.\n"},
			wantFile: ".claude/skills/bad/SKILL.md", wantLine: 4, typed: true,
			inText: []string{`skill "bad"`, `sets "paths" to a value of the wrong type`, "the gate cannot be read"},
		},
		{
			name:     "a paths list holding a number",
			files:    map[string]string{".claude/skills/native/SKILL.md": "---\nname: native\ndescription: Use when testing the gate.\npaths: [\"src/**\", 5]\n---\nBody.\n"},
			wantFile: ".claude/skills/native/SKILL.md", wantLine: 4, typed: true,
		},
		{
			name:     "disable-model-invocation yes, a string to YAML 1.2",
			files:    map[string]string{".claude/skills/native/SKILL.md": "---\nname: native\ndescription: Use when testing the gate.\ndisable-model-invocation: yes\n---\nBody.\n"},
			wantFile: ".claude/skills/native/SKILL.md", wantLine: 4, typed: true,
			inText: []string{`sets "disable-model-invocation" to a value of the wrong type`},
		},
		{
			name:     "two wrong-typed gates, on the first",
			files:    map[string]string{".claude/skills/two/SKILL.md": "---\nname: two\ndescription: Two gates.\ndisable-model-invocation: \"no\"\npaths: 5\n---\nBody.\n"},
			wantFile: ".claude/skills/two/SKILL.md", wantLine: 4, typed: true,
			inText: []string{`"disable-model-invocation", "paths"`},
		},
		{
			name:  "well-typed gates are clean",
			files: map[string]string{".claude/skills/ok/SKILL.md": "---\nname: ok\ndescription: Use when testing the gate.\ndisable-model-invocation: true\npaths: src/**\n---\nBody.\n"},
		},
		{
			name:  "a broken block that names no gate",
			files: map[string]string{".ai-rulez/skills/bad/SKILL.md": brokenFront("bad", "")},
		},
		{
			name:  "a gate key only in a value",
			files: map[string]string{".ai-rulez/skills/bad/SKILL.md": "---\nname: bad\ndescription: Use when: paths: are set\n---\nBody.\n"},
		},
		{
			name:  "a valid gated skill",
			files: map[string]string{".ai-rulez/skills/ok/SKILL.md": "---\nname: ok\ndescription: Use when testing the gate.\npaths: [src/**]\n---\nBody.\n"},
		},
		{
			name:  "an agent has no gate",
			files: map[string]string{".ai-rulez/agents/helper.md": "---\nname: helper\ndescription: Use when: x\npaths: [src/**]\n---\nHelp.\n"},
		},
		{
			name:    "a project that does not write for Claude Code",
			presets: `"cursor"`,
			files:   map[string]string{".ai-rulez/skills/bad/SKILL.md": brokenFront("bad", "paths: [src/**]\n")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := baseConfig
			if tt.presets != "" {
				base = strings.Replace(base, `presets = ["claude"]`, "presets = ["+tt.presets+"]", 1)
			}
			files := map[string]string{".ai-rulez/config.toml": base}
			for k, v := range tt.files {
				files[k] = v
			}

			// Act
			got := lintDetected(t, files)

			// Assert
			var findings []Finding
			for _, f := range got {
				if f.Code == CodeExposureGateUnreadable {
					findings = append(findings, f)
				}
			}
			if tt.wantFile == "" {
				assert.Empty(t, findings, dump(got))
				return
			}
			require.Len(t, findings, 1, dump(got))
			f := findings[0]
			assert.True(t, strings.HasSuffix(strings.ReplaceAll(f.File, "\\", "/"), tt.wantFile), "file %s, want suffix %s", f.File, tt.wantFile)
			assert.Equal(t, tt.wantLine, f.Line)
			assert.Equal(t, SeverityWarning, f.Severity)
			for _, s := range tt.inText {
				assert.Contains(t, f.Message, s)
			}
			if tt.typed {
				assert.False(t, has(got, "AR306", tt.wantFile, 0), "a block that parses is not AR306:\n%s", dump(got))
				return
			}
			assert.True(t, has(got, "AR306", tt.wantFile, 1), "AR9W4 comes with AR306:\n%s", dump(got))
		})
	}
}

// Without a configuration the run targets Claude Code, so a hand-written skill
// with a lost gate is reported.
func TestExposureGateUnreadableConfigFree(t *testing.T) {
	// Arrange
	files := map[string]string{
		".claude/skills/native/SKILL.md": brokenFront("native", "paths: [src/**]\n"),
		".claude/skills/plain/SKILL.md":  brokenFront("plain", ""),
	}

	// Act
	got := lintConfigFree(t, files)

	// Assert
	assert.Equal(t, 1, countCode(got, CodeExposureGateUnreadable), dump(got))
	assert.True(t, has(got, CodeExposureGateUnreadable, ".claude/skills/native/SKILL.md", 4), dump(got))
}
