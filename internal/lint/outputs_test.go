package lint

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func lintWithOutputs(t *testing.T, root string, outputs []OutputFile) []Finding {
	t.Helper()
	cfg, err := loadWithResolvers(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(cfg, tree, WithOutputs(outputs), WithCwd(root))
	if err != nil {
		t.Fatal(err)
	}
	return rep.Findings
}

func sourceOf(f Finding) string {
	if f.Meta == nil {
		return ""
	}
	return f.Meta.Source
}

func TestOutputsAreLintedAndTaggedAndKeepNoFix(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	// Planned only: the file is not on disk yet.
	out := OutputFile{Abs: filepath.Join(root, ".claude", "skills", "gen", "SKILL.md"), Data: []byte("---\nname: gen\n---\nbody\n")}

	fs := lintWithOutputs(t, root, []OutputFile{out})

	var got []Finding
	for _, f := range fs {
		if f.Code == CodeDescriptionMissing {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one AR801 on the output, got %d\n%s", len(got), dump(fs))
	}
	if sourceOf(got[0]) != "output" {
		t.Errorf("source = %q, want output", sourceOf(got[0]))
	}
	if got[0].Meta != nil && got[0].Meta.Fix != nil {
		t.Error("a finding on an output carries no fix")
	}
}

func TestOutputIdenticalToAuthoredSourceIsReportedOnce(t *testing.T) {
	root := t.TempDir()
	skill := "---\nname: gen\n---\nbody\n"
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/skills/gen/SKILL.md": skill})
	gitAdd(t, root)
	out := OutputFile{Abs: filepath.Join(root, ".claude", "skills", "gen", "SKILL.md"), Data: []byte(skill)}

	fs := lintWithOutputs(t, root, []OutputFile{out})

	if n := countCode(fs, CodeDescriptionMissing); n != 1 {
		t.Fatalf("AR801 reported %d times, want once at the source\n%s", n, dump(fs))
	}
	if !has(fs, CodeDescriptionMissing, ".ai-rulez/skills/gen/SKILL.md", 0) {
		t.Errorf("the finding must sit at the authored path\n%s", dump(fs))
	}
}

func TestOutputWhoseFindingRepeatsASourceFindingIsDropped(t *testing.T) {
	root := t.TempDir()
	const key = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/rules/r.md": "# R\n\nkey = " + key + "\n"})
	gitAdd(t, root)
	// An aggregate file that repeats the rule among others.
	out := OutputFile{Abs: filepath.Join(root, "AGENTS.md"), Data: []byte("# Project\n\n# R\n\nkey = " + key + "\n\nmore text\n")}

	fs := lintWithOutputs(t, root, []OutputFile{out})

	if n := countCode(fs, CodeSecretDetected); n != 1 {
		t.Fatalf("AR001 reported %d times, want once\n%s", n, dump(fs))
	}
}

func TestOutputAtAnAuthoredPathIsSkipped(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/skills/gen/SKILL.md": "---\nname: gen\n---\nbody\n"})
	gitAdd(t, root)
	out := OutputFile{Abs: filepath.Join(root, ".ai-rulez", "skills", "gen", "SKILL.md"), Data: []byte("---\nname: other\n---\nchanged\n")}

	fs := lintWithOutputs(t, root, []OutputFile{out})

	for _, f := range fs {
		if sourceOf(f) == "output" {
			t.Errorf("a file the project authors is never an output: %s", dump([]Finding{f}))
		}
	}
}

func TestOutputSourceReachesEveryReportFormat(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	out := OutputFile{Abs: filepath.Join(root, ".claude", "skills", "gen", "SKILL.md"), Data: []byte("---\nname: gen\n---\nbody\n")}
	fs := lintWithOutputs(t, root, []OutputFile{out})
	combined := Combine([]*Report{{Root: ".", Findings: fs}})

	tests := []struct {
		format string
		want   string
	}{
		{FormatJSON, `"source": "output"`},
		{FormatSARIF, `"source": "output"`},
		{FormatText, "[generated output]"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var sb strings.Builder
			if err := Write(&sb, tt.format, combined, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(sb.String(), tt.want) {
				t.Errorf("%s report lacks %q:\n%s", tt.format, tt.want, sb.String())
			}
		})
	}
}
