package generator

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/samber/oops"
)

// PluginPruneReport lists the obsolete generated plugin files of a run, as
// project-relative paths. Pruned files are deleted by generation; Kept files
// are obsolete but were edited (or cannot be deleted safely) and stay.
type PluginPruneReport struct {
	Pruned []string
	Kept   []string
}

// pluginPrune is one obsolete file with the decision made for it.
type pluginPrune struct {
	plugin.Obsolete
	bundleDir string
	rel       string // project-relative, for reports
	reason    string // why the file is kept; empty when it is pruned
}

// planPluginPrune compares the sidecar each planned bundle will write with the
// one already on disk and returns the files the previous run generated that the
// plan no longer contains. It must run before the outputs are written, which
// replaces the old sidecars.
func (g *Generator) planPluginPrune(outputs []config.OutputFile) ([]pluginPrune, error) {
	var items []pluginPrune
	for _, output := range outputs {
		if output.IsDir || filepath.Base(output.Path) != plugin.ProvenanceFileName {
			continue
		}
		sidecarPath := g.absOutputPath(output.Path)
		previous, err := os.ReadFile(sidecarPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, oops.With("path", sidecarPath).Wrapf(err, "read previous plugin provenance")
		}
		planned := output.RawContent
		if planned == nil {
			planned = []byte(output.Content)
		}
		bundleDir := filepath.Dir(sidecarPath)
		obsolete, err := plugin.ObsoleteFiles(bundleDir, previous, planned)
		if err != nil {
			g.log().Warn("Could not compare a plugin bundle with its previous run; obsolete files are not pruned",
				"bundle", g.convertToRelativePath(bundleDir), "error", err)
			continue
		}
		for _, item := range obsolete {
			entry := pluginPrune{Obsolete: item, bundleDir: bundleDir, rel: filepath.ToSlash(g.convertToRelativePath(item.Path))}
			switch {
			case item.Edited:
				entry.reason = item.Why
				if entry.reason == "" {
					entry.reason = plugin.WhyEdited
				}
			default:
				if _, _, guardErr := g.guardWrite(item.Path); guardErr != nil {
					entry.reason = "its path crosses a symlink that leaves the project"
				}
			}
			items = append(items, entry)
		}
	}
	return items, nil
}

func pruneReport(items []pluginPrune) PluginPruneReport {
	var report PluginPruneReport
	for _, item := range items {
		if item.reason == "" {
			report.Pruned = append(report.Pruned, item.rel)
		} else {
			report.Kept = append(report.Kept, item.rel)
		}
	}
	return report
}

// retainKeptObsolete carries the retained obsolete records of the previous
// sidecars, plus the files this run keeps, into the planned sidecars before they
// are written. Without it, writing a sidecar would forget files regeneration
// preserved, and a later verify --plugin would pass while they still exist. It
// must run after planPluginPrune and before writeOutputs.
func (g *Generator) retainKeptObsolete(outputs []config.OutputFile, items []pluginPrune) error {
	kept := make(map[string][]plugin.Obsolete)
	for _, item := range items {
		if item.reason == "" {
			continue
		}
		bundleDir := filepath.Clean(item.bundleDir)
		kept[bundleDir] = append(kept[bundleDir], item.Obsolete)
	}
	for i := range outputs {
		if outputs[i].IsDir || filepath.Base(outputs[i].Path) != plugin.ProvenanceFileName {
			continue
		}
		sidecarPath := g.absOutputPath(outputs[i].Path)
		bundleDir := filepath.Clean(filepath.Dir(sidecarPath))
		previous, err := os.ReadFile(sidecarPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return oops.With("path", sidecarPath).Wrapf(err, "read previous plugin provenance")
		}
		planned := outputs[i].RawContent
		if planned == nil {
			planned = []byte(outputs[i].Content)
		}
		merged, err := plugin.RetainObsolete(planned, previous, kept[bundleDir])
		if err != nil {
			return oops.With("path", outputs[i].Path).Wrapf(err, "retain kept obsolete plugin files")
		}
		outputs[i].Content = ""
		outputs[i].RawContent = merged
	}
	return nil
}

// resolvePluginOutputs collects the planned plugin outputs and merges the
// retained obsolete records of the previous sidecars into them, so the sidecars
// a run plans agree with the ones already on disk.
func (g *Generator) resolvePluginOutputs(profile string) ([]config.OutputFile, []pluginPrune, error) {
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, nil, err
	}
	items, err := g.planPluginPrune(outputs)
	if err != nil {
		return nil, nil, err
	}
	if err := g.retainKeptObsolete(outputs, items); err != nil {
		return nil, nil, err
	}
	return outputs, items, nil
}

// PluginPrunePlan reports the obsolete generated plugin files a run would
// delete or keep, without touching the disk.
func (g *Generator) PluginPrunePlan(profile string) (PluginPruneReport, error) {
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return PluginPruneReport{}, err
	}
	items, err := g.planPluginPrune(outputs)
	if err != nil {
		return PluginPruneReport{}, err
	}
	return pruneReport(items), nil
}

// applyPluginPrune deletes the unchanged obsolete files, prunes the directories
// they leave empty, and warns about each file it keeps. A failure is reported
// and does not stop the run.
func (g *Generator) applyPluginPrune(items []pluginPrune) {
	for _, item := range items {
		if item.reason != "" {
			g.log().Warn("Kept an obsolete generated plugin file: "+item.reason+"; delete it by hand if it is not needed",
				"file", item.rel)
			continue
		}
		if err := plugin.RemoveObsolete(item.bundleDir, item.Path); err != nil {
			g.log().Warn("Could not remove an obsolete generated plugin file", "file", item.rel, "error", err)
			continue
		}
		g.log().Info("Removed obsolete generated plugin file", "file", item.rel)
	}
}

// obsoleteFilesError is the verify failure for obsolete generated files that
// are still on disk.
func obsoleteFilesError(items []pluginPrune) error {
	files := make([]string, 0, len(items))
	for _, item := range items {
		files = append(files, item.rel)
	}
	return oops.With("files", strings.Join(files, ", ")).
		Hint("Run ai-rulez generate --plugin to remove obsolete generated files (edited ones are kept; delete them by hand)").
		Errorf("obsolete generated plugin file: %s", strings.Join(files, ", "))
}

// unrecordedSkillFiles lists, as absolute paths, the files and symlinks inside
// each generated skill directory that the plan does not write. Such a file ships
// to everyone who installs the bundle, so generate removes it and verify
// reports it. Only directories holding a planned SKILL.md are searched, so a
// hand-made skill directory is never touched, and files the previous run
// recorded as obsolete are left to the obsolete handling. Directories that
// hold the plugin's own sources (the config directory and [plugin]
// content_root) are skipped: a bundle written over its sources must not
// delete authored files such as evals/.
func (g *Generator) unrecordedSkillFiles(expected []config.OutputFile, obsolete []pluginPrune) ([]string, error) {
	planned := make(map[string]bool, len(expected))
	var skillDirs []string
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		abs := filepath.Clean(g.absOutputPath(output.Path))
		planned[abs] = true
		if filepath.Base(abs) == "SKILL.md" && !g.isPluginSourcePath(abs) {
			skillDirs = append(skillDirs, filepath.Dir(abs))
		}
	}
	for _, item := range obsolete {
		planned[filepath.Clean(item.Path)] = true
	}
	var extras []string
	for _, dir := range skillDirs {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return oops.With("path", path).Wrapf(walkErr, "walk generated skill directory")
			}
			if !entry.IsDir() && !planned[filepath.Clean(path)] {
				extras = append(extras, filepath.Clean(path))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(extras)
	return slices.Compact(extras), nil
}

// isPluginSourcePath reports whether path lies in the config directory or the
// [plugin] content_root, where authored content lives.
func (g *Generator) isPluginSourcePath(path string) bool {
	roots := []string{g.config.ConfigDir}
	if g.config.Plugin != nil && g.config.Plugin.ContentRoot != "" {
		roots = append(roots, filepath.Join(g.config.BaseDir, g.config.Plugin.ContentRoot))
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// removeUnrecordedSkillFiles deletes what unrecordedSkillFiles lists. A path
// that crosses a symlink leaving the project is kept with a warning; a failed
// removal is reported and does not stop the run.
func (g *Generator) removeUnrecordedSkillFiles(expected []config.OutputFile, obsolete []pluginPrune) error {
	extras, err := g.unrecordedSkillFiles(expected, obsolete)
	if err != nil {
		return err
	}
	for _, path := range extras {
		rel := filepath.ToSlash(g.convertToRelativePath(path))
		if _, _, guardErr := g.guardWrite(path); guardErr != nil {
			g.log().Warn("Kept an unrecorded file in a generated skill directory: its path crosses a symlink that leaves the project",
				"file", rel)
			continue
		}
		if err := os.Remove(path); err != nil {
			g.log().Warn("Could not remove an unrecorded file in a generated skill directory", "file", rel, "error", err)
			continue
		}
		g.log().Info("Removed unrecorded file in a generated skill directory", "file", rel)
	}
	return nil
}
