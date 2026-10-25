package lint

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const (
	aliasDescription = "Use when extracting text from scanned documents with the demo toolkit."
	// aliasSource is the definition of the skill in the plugin's source tree,
	// aliasRendered what the plugin generator renders from it: plugin front
	// matter, a generated header and rewritten resource links.
	aliasSource   = "---\nname: demo\ndescription: " + aliasDescription + "\n---\n\n# Demo\n\nSee `../../references/cli.md`.\n"
	aliasRendered = "---\ndescription: " + aliasDescription + "\nname: demo\n---\n\n<!--\nAI-RULEZ :: GENERATED FILE — DO NOT EDIT\n" +
		"Content-Hash: blake3:00\nSource-Hash: blake3:11\nSchema-Version: v1\n-->\n\n# Demo\n\nSee `references/cli.md`.\n"

	aliasDefinition = "plugin/.ai-rulez/skills/demo/SKILL.md"
	aliasPluginCopy = "plugin/skills/demo/SKILL.md"
	aliasExport     = "skills/demo/SKILL.md"
)

// aliasLayout is the layout of #485: a plugin configured in plugin/.ai-rulez,
// its generated bundle in plugin/ and a public export of the generated skill at
// the repository root.
func aliasLayout() map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":        baseConfig,
		"plugin/.ai-rulez/config.toml": baseConfig,
		aliasDefinition:                aliasSource,
		aliasPluginCopy:                aliasRendered,
		aliasExport:                    aliasRendered,
	}
}

// renderedCopies stands in for the generator: the rendering of the definition
// at root/aliasDefinition is rendered.
func renderedCopies(root, rendered string) Option {
	return WithGeneratedCopies(func() []GeneratedCopy {
		return []GeneratedCopy{{
			Kind:    kindSkill,
			Source:  config.ContentFile{Name: "demo", Path: filepath.Join(root, filepath.FromSlash(aliasDefinition))},
			Content: []byte(rendered),
		}}
	})
}

func handWritten(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# Hand written\n\nMine.\n"
}

// aliasCase changes the layout of aliasLayout and lists the AR701/AR703
// findings of a run from the repository root and from the plugin's
// configuration, as "CODE path".
type aliasCase struct {
	name string
	edit func(files map[string]string)
	// rendered replaces what the generator renders from the definition.
	rendered     string
	noRendering  bool
	root, plugin []string
}

func aliasCases() []aliasCase {
	return []aliasCase{
		{name: "copies equal to the rendering"},
		{
			name:     "copy with CRLF line endings",
			edit:     func(files map[string]string) { files[aliasExport] = strings.ReplaceAll(aliasRendered, "\n", "\r\n") },
			rendered: aliasRendered,
		},
		{
			name:        "run without the renderings",
			noRendering: true,
			root:        []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport},
			plugin:      []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport, "AR703 " + aliasPluginCopy, "AR703 " + aliasExport},
		},
		{
			name: "independently authored duplicate description",
			edit: func(files map[string]string) {
				files[".claude/skills/helper/SKILL.md"] = handWritten("helper", aliasDescription)
			},
			root:   []string{"AR701 " + aliasDefinition},
			plugin: []string{"AR701 .claude/skills/helper/SKILL.md"},
		},
		{
			name: "hand-written skill with the same name",
			edit: func(files map[string]string) {
				files[".claude/skills/demo/SKILL.md"] = handWritten("demo", "Use when doing something else entirely with the demo.")
			},
			plugin: []string{"AR703 .claude/skills/demo/SKILL.md"},
		},
		{
			name: "hand-written skill with the same name and description",
			edit: func(files map[string]string) {
				files[".claude/skills/demo/SKILL.md"] = handWritten("demo", aliasDescription)
			},
			root:   []string{"AR701 " + aliasDefinition},
			plugin: []string{"AR701 .claude/skills/demo/SKILL.md", "AR703 .claude/skills/demo/SKILL.md"},
		},
		{
			name: "edited export under the generated header",
			edit: func(files map[string]string) {
				files[aliasExport] = strings.Replace(aliasRendered, "# Demo", "# Demo, edited", 1)
			},
			root:   []string{"AR701 " + aliasExport},
			plugin: []string{"AR701 " + aliasExport, "AR703 " + aliasExport},
		},
		{
			// The definition changed after the bundle was generated: the copies
			// on disk are stale, and a hand-written skill shares their old
			// description. The definition no longer stands for them.
			name: "stale copies beside a third-party duplicate",
			edit: func(files map[string]string) {
				files[aliasDefinition] = strings.Replace(aliasSource, aliasDescription, "Use when rolling the toolkit back after a failed release.", 1)
				files[".claude/skills/helper/SKILL.md"] = handWritten("helper", aliasDescription)
			},
			rendered: strings.Replace(aliasRendered, aliasDescription, "Use when rolling the toolkit back after a failed release.", 1),
			root:     []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport},
			plugin:   []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport, "AR703 " + aliasPluginCopy, "AR703 " + aliasExport},
		},
		{
			name: "rendering of another definition with the same id",
			edit: func(files map[string]string) {
				files["other/.ai-rulez/skills/demo/SKILL.md"] = files[aliasDefinition]
				delete(files, aliasDefinition)
				files["plugin/.ai-rulez/skills/keep/SKILL.md"] = handWritten("keep", "Use when keeping the plugin configuration non-empty.")
			},
			root:   []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport},
			plugin: []string{"AR701 " + aliasPluginCopy, "AR701 " + aliasExport},
		},
	}
}

func TestGeneratedSkillAliasesAreNotDuplicates(t *testing.T) {
	for _, tt := range aliasCases() {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			files := aliasLayout()
			if tt.edit != nil {
				tt.edit(files)
			}
			writeFiles(t, root, files)
			gitAdd(t, root)
			rendered := aliasRendered
			if tt.rendered != "" {
				rendered = tt.rendered
			}
			var opts []Option
			if !tt.noRendering {
				opts = append(opts, renderedCopies(root, rendered))
			}
			for _, run := range []struct {
				dir  string
				want []string
			}{{root, tt.root}, {filepath.Join(root, "plugin"), tt.plugin}} {
				got, _ := lintWithTargets(t, run.dir, TargetSpec{Detect: true}, opts...)
				if have := duplicateFindings(root, run.dir, got); !slices.Equal(have, sorted(run.want)) {
					t.Errorf("run in %s: AR701/AR703 = %q, want %q\n%s", run.dir, have, sorted(run.want), dump(got))
				}
			}
		})
	}
}

// TestGeneratedCommandAlias: a command named on the command line that is the
// rendering of a configured command is that command.
func TestGeneratedCommandAlias(t *testing.T) {
	const command = "---\ndescription: Use when extracting text from the current document.\n---\n\nRun the extraction.\n"
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":          baseConfig,
		".ai-rulez/commands/extract.md":  command,
		"bundle/commands/extract.md":     command + "\nGenerated.\n",
		"bundle/commands/extract-old.md": command + "\nOld.\n",
	})
	gitAdd(t, root)
	copies := WithGeneratedCopies(func() []GeneratedCopy {
		return []GeneratedCopy{{
			Kind:    kindCommand,
			Source:  config.ContentFile{Name: "extract", Path: filepath.Join(root, ".ai-rulez", "commands", "extract.md")},
			Content: []byte(command + "\nGenerated.\n"),
		}}
	})
	spec := TargetSpec{Paths: []string{filepath.Join(root, "bundle", "commands", "extract.md"), filepath.Join(root, "bundle", "commands", "extract-old.md")}}
	got, _ := lintWithTargets(t, root, spec, copies)
	if have, want := duplicateFindings(root, root, got), []string{"AR701 bundle/commands/extract-old.md"}; !slices.Equal(have, want) {
		t.Fatalf("AR701/AR703 = %q, want %q\n%s", have, want, dump(got))
	}
}

// TestGeneratedAliasKeepsSecurityFindings: an alias is exempt from the duplicate
// rules only; the rest of the lint, the security rules first, still reads it.
func TestGeneratedAliasKeepsSecurityFindings(t *testing.T) {
	root := t.TempDir()
	rendered := strings.Replace(aliasRendered, "See `references/cli.md`.", "Run `uvx demo-cli extract file.pdf`.", 1)
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		aliasDefinition:         aliasSource,
		aliasPluginCopy:         rendered,
	})
	gitAdd(t, root)
	got, _ := lintWithTargets(t, root, TargetSpec{Detect: true}, renderedCopies(root, rendered))
	if have := duplicateFindings(root, root, got); len(have) > 0 {
		t.Fatalf("duplicate findings on a generated alias: %q", have)
	}
	if !slices.ContainsFunc(got, func(f Finding) bool {
		return f.Code == CodeUnpinnedExec && relTo(root, root, f.File) == aliasPluginCopy
	}) {
		t.Fatalf("want %s on the generated copy:\n%s", CodeUnpinnedExec, dump(got))
	}
}

// duplicateFindings lists the AR701, AR702 and AR703 findings of a run in dir
// as "CODE path", the path relative to root.
func duplicateFindings(root, dir string, fs []Finding) []string {
	var out []string
	for _, f := range fs {
		if f.Code == CodeDescriptionDup || f.Code == CodeDescriptionNearDup || f.Code == CodeDuplicateCollapsed {
			out = append(out, f.Code+" "+relTo(root, dir, f.File))
		}
	}
	return sorted(out)
}

// relTo returns the path of a finding of a run in dir relative to root.
func relTo(root, dir, file string) string {
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	for _, base := range []string{root, resolvedRoot} {
		if rel, err := filepath.Rel(base, file); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return file
}

func sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	if out == nil {
		out = []string{}
	}
	return out
}
