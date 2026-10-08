package generator

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// DryRunBlocked reports whether the plan from the last DryRun contains local
// drift that Generate would refuse to write (nil when it is allowed or absent).
// DryRun itself still returns the full plan, including the blocked lines.
func (g *Generator) DryRunBlocked() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.refusedOutputs) > 0 {
		return refusalError(g.refusedOutputs)
	}
	if g.plan == nil {
		return nil
	}
	return g.plan.check(g.allowLocalDrift)
}

// DryRun returns an inspectable generation plan without writing or deleting files.
func (g *Generator) DryRun(profile string) ([]string, error) {
	res, err := g.run(profile, DryRunApplier)
	if err != nil {
		return nil, err
	}
	return res.Lines, nil
}

// planLines lists the directories and files a run would create. A file whose
// rendering already matches the disk is listed as unchanged, one that was
// edited by hand as edited (generate keeps it until its sources change), and a
// file the overwrite guard would leave alone is not listed, because it is not written.
func (g *Generator) planLines(outputs []config.OutputFile) []string {
	g.previousFiles = nil
	defer func() { g.previousFiles = nil }()
	var lines []string
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		relPath := g.convertToRelativePath(abs)
		if reason, ok := g.refusalReason(relPath); ok {
			lines = append(lines, "blocked: "+filepath.ToSlash(relPath)+" ("+reason+")")
			continue
		}
		if output.IsDir {
			lines = append(lines, "create-dir: "+relPath)
			continue
		}
		if g.linkedOutputs[filepath.ToSlash(relPath)] {
			lines = append(lines, "skipped: "+filepath.ToSlash(relPath)+" (a symlink to a path generated in this run)")
			continue
		}
		kind, rewrite, compared := g.outputState(output)
		switch {
		case !compared:
		case rewrite:
			lines = append(lines, "write-file: "+relPath)
		case kind == DriftEdited:
			lines = append(lines, "edited: "+relPath)
		default:
			lines = append(lines, "unchanged: "+relPath)
		}
	}
	return lines
}

// disambiguatedRuleName inserts ".ai-rulez" before the extension of a rule
// file path: "x.md" becomes "x.ai-rulez.md", "x.instructions.md" becomes
// "x.ai-rulez.instructions.md".
func disambiguatedRuleName(p string) string {
	ext := filepath.Ext(p)
	if strings.HasSuffix(p, ".instructions.md") {
		ext = ".instructions.md"
	}
	return strings.TrimSuffix(p, ext) + ".ai-rulez" + ext
}

// disambiguateRuleCollisions renames a generated rule file that would collide
// with a hand-written file of the same name to "<id>.ai-rulez<ext>", so the
// rule still reaches the tool instead of being skipped. It runs while the
// outputs are collected rather than in writeOutput because the new name must be
// what the manifest, the managed .gitignore block, the dry run and clean all
// see, and all of them start from the collected outputs. The name depends only
// on the hand-written file existing, so it is stable across runs, and once the
// file is gone the rule goes back to its plain name and the manifest entry of
// the renamed file makes the next run delete it. Machine-local rule files keep
// the skip-and-warn behavior: their ".local." names are reserved.
func (g *Generator) disambiguateRuleCollisions(outputs []config.OutputFile) {
	g.previousFiles = nil
	defer func() { g.previousFiles = nil }()
	for i, output := range outputs {
		if output.IsDir || output.LocalOnly || output.RawContent != nil {
			continue
		}
		abs := g.absOutputPath(output.Path)
		if !g.config.InRulesDir(filepath.ToSlash(g.convertToRelativePath(abs))) ||
			!g.isUnmanagedRuleFile(abs, g.finalContent(output)) {
			continue
		}
		renamed := output
		renamed.Path = disambiguatedRuleName(output.Path)
		if g.isUnmanagedRuleFile(g.absOutputPath(renamed.Path), g.finalContent(renamed)) {
			continue // the new name is taken by a hand-written file too: the guard skips it
		}
		g.log().Warn("A hand-written rule file has the same name as a generated rule; "+
			"the generated rule was written under another name, rename one of them to silence this",
			"hand_written", output.Path, "generated", renamed.Path)
		outputs[i] = renamed
	}
}
