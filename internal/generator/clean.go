package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// CleanOptions controls Clean behavior.
type CleanOptions struct {
	// DryRun computes the plan without deleting anything.
	DryRun bool
	// KeepGitignore leaves the ai-rulez managed block in .gitignore in place.
	KeepGitignore bool
	// KeepManifest leaves the generated manifest (.generated-manifest.json) in place.
	KeepManifest bool
}

// CleanPlan describes what Clean removed (or, in dry-run, would remove).
type CleanPlan struct {
	Profile         string
	Files           []string // absolute paths of generated files to remove
	Dirs            []string // absolute generated dirs, deepest-first (removed only if empty)
	ManifestPath    string   // absolute manifest path; "" when absent or kept
	GitignoreEdited bool     // the managed .gitignore block will be / was stripped
}

// Empty reports whether the plan would remove nothing at all.
func (p *CleanPlan) Empty() bool {
	return len(p.Files) == 0 && len(p.Dirs) == 0 && p.ManifestPath == "" && !p.GitignoreEdited
}

// Clean removes the files and directories that Generate produced for the given
// profile — the inverse of Generate. It never touches the .ai-rulez/ source
// tree, since those paths are not among the generated outputs. Files are removed
// first, then now-empty generated directories deepest-first; the generated
// manifest and the ai-rulez managed .gitignore block are removed too unless the
// corresponding Keep* option is set. With DryRun the plan is computed but nothing
// is deleted.
func (g *Generator) Clean(profile string, opts CleanOptions) (*CleanPlan, error) {
	outputs, activeProfile, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}

	plan := &CleanPlan{Profile: activeProfile}

	var dirs []string
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		if !isUnderBaseDir(g.config.BaseDir, abs) {
			logger.Warn("Skipping generated path outside project", "path", output.Path)
			continue
		}
		if output.IsDir {
			dirs = append(dirs, abs)
		} else {
			plan.Files = append(plan.Files, abs)
		}
	}

	// Include files recorded in the manifest from earlier runs that the current
	// profile no longer emits (e.g. a preset was removed): the exact set generate
	// itself cleans up as stale.
	plan.Files = append(plan.Files, g.staleManifestFiles(outputs)...)
	plan.Files = existingSortedUnique(plan.Files)

	// Deepest-first so children are removed before their parents.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] > dirs[j] })
	plan.Dirs = existingDirs(dirs)

	if !opts.KeepManifest {
		if mp := g.manifestPath(); pathIsFile(mp) {
			plan.ManifestPath = mp
		}
	}
	if !opts.KeepGitignore {
		plan.GitignoreEdited = g.gitignoreHasManagedBlock()
	}

	if opts.DryRun {
		return plan, nil
	}

	for _, f := range plan.Files {
		g.removeStaleFile(f)
	}
	for _, d := range plan.Dirs {
		removeDirIfEmpty(d)
	}
	if plan.ManifestPath != "" {
		g.removeStaleFile(plan.ManifestPath)
	}
	if plan.GitignoreEdited {
		if err := g.stripGitignoreManagedBlock(); err != nil {
			logger.Warn("Failed to strip .gitignore managed block", "error", err)
			plan.GitignoreEdited = false
		}
	}

	return plan, nil
}

// gitignoreHasManagedBlock reports whether <BaseDir>/.gitignore contains the
// ai-rulez fenced block.
func (g *Generator) gitignoreHasManagedBlock() bool {
	data, err := os.ReadFile(filepath.Join(g.config.BaseDir, ".gitignore"))
	if err != nil {
		return false
	}
	return contains(string(data), gitignore.BeginMarker)
}

// stripGitignoreManagedBlock removes the ai-rulez fenced block from .gitignore,
// leaving any user-authored entries intact. If nothing remains, the .gitignore
// file is deleted rather than left empty.
func (g *Generator) stripGitignoreManagedBlock() error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("path", gitignorePath).Wrapf(err, "read .gitignore")
	}

	stripped := strings.TrimRight(gitignore.ReplaceFencedBlock(string(data), ""), "\n")
	if strings.TrimSpace(stripped) == "" {
		if err := os.Remove(gitignorePath); err != nil {
			return oops.With("path", gitignorePath).Wrapf(err, "remove empty .gitignore")
		}
		return nil
	}
	if err := os.WriteFile(gitignorePath, []byte(stripped+"\n"), 0o644); err != nil { //nolint:gosec // path from config, not user input
		return oops.With("path", gitignorePath).Wrapf(err, "write .gitignore")
	}
	return nil
}

// existingSortedUnique filters to on-disk files, deduplicates, and sorts.
func existingSortedUnique(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if seen[p] || !pathIsFile(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// existingDirs filters to directories that exist on disk, preserving order.
func existingDirs(paths []string) []string {
	var out []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// removeDirIfEmpty removes a directory only when it holds no entries, so
// user-authored files inside a generated directory are never destroyed.
func removeDirIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) > 0 {
		return
	}
	if err := os.Remove(dir); err != nil {
		logger.Debug("Kept non-removable directory", "path", dir, "error", err)
	} else {
		logger.Debug("Removed empty directory", "path", dir)
	}
}

// pathIsFile reports whether path exists and is a regular (non-directory) file.
func pathIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
