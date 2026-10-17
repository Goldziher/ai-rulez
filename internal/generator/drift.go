package generator

import (
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/genmark"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
)

// DriftKind says how a generated file differs from what the sources render.
type DriftKind string

// Drift kinds. They appear in `generate --check` and `verify` output, so they
// are part of the CLI contract.
const (
	// DriftMissing: the file is absent (or, for verify, listed in the manifest
	// but gone from disk).
	DriftMissing DriftKind = "missing"
	// DriftStale: the file exists but a generate run would rewrite it (source
	// changed, or the rendering differs).
	DriftStale DriftKind = "stale"
	// DriftEdited: the body no longer matches the Content-Hash in its own
	// header, so somebody edited a generated file by hand.
	DriftEdited DriftKind = "edited"
	// DriftOrphan: the file is in the previous manifest, no longer rendered, and
	// a generate run would delete it.
	DriftOrphan DriftKind = "orphan"
	// DriftBlocked: a machine-local input would change this file and generate
	// refuses to write it (it is tracked or not ignored); `generate` and
	// `generate --dry-run` report the same refusal. --allow-local-drift lifts it.
	DriftBlocked DriftKind = "blocked"
)

// Drift is one generated file that differs from its expected state.
type Drift struct {
	Path string    `json:"path"` // slash-separated, relative to the project root
	Kind DriftKind `json:"kind"`
	// Note says which part of Path drifted when it is not the whole file: the
	// managed block of a .gitignore.
	Note string `json:"note,omitempty"`
}

// outputState classifies one rendered file against the disk without writing
// anything. kind is "" when the file is current. rewrite reports whether a
// generate run would write the file; a hand-edited file in the default "full"
// hash mode is reported as edited but not rewritten, because generate compares
// header hashes. ok is false for an output that is not compared (directories,
// and hand-written rule files that generate deliberately leaves alone).
func (g *Generator) outputState(output config.OutputFile) (kind DriftKind, rewrite, ok bool) {
	if output.IsDir {
		return "", false, false
	}
	abs := g.absOutputPath(output.Path)
	if output.RawContent != nil {
		kind = g.rawState(abs, output)
		return kind, kind != "", true
	}
	final := g.finalContent(output)
	if g.isUnmanagedRuleFile(abs, final) {
		return "", false, false
	}
	existing, err := g.config.ReadExisting(abs)
	if err != nil {
		return DriftMissing, true, true
	}
	rewrite = !g.canSkipWrite(abs, output, final)
	switch {
	case g.config.GetHeaderHashes() != config.HeaderHashesNone && g.bodyEdited(string(existing), abs):
		return DriftEdited, rewrite, true
	case rewrite:
		return DriftStale, true, true
	}
	return "", false, true
}

func (g *Generator) rawState(abs string, output config.OutputFile) DriftKind {
	mode := output.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if output.Sensitive {
		mode &= sensitiveFileMode
		if mode == 0 {
			mode = sensitiveFileMode
		}
	}
	if _, err := g.config.StatExisting(abs); err != nil {
		return DriftMissing
	}
	if !g.rawWriteCanSkip(abs, output.RawContent, mode) {
		return DriftStale
	}
	return ""
}

// maxHashedTrailingNewlines is the longest run of trailing newlines a
// Content-Hash covers. The hash is taken before generate normalizes the file
// to one trailing newline, so bodyEdited cannot see how many the rendering had
// and tries every run up to this bound; bodyHash caps the run at the same bound
// so that every hash it writes is one of those shapes.
const maxHashedTrailingNewlines = genmark.MaxHashedTrailingNewlines

// bodyEdited reports whether content, read from path, no longer matches the
// Content-Hash stored in its header. A file without a stored hash cannot be
// judged and is not reported.
func bodyEdited(content, path string) bool {
	stored, _, _ := scanStoredHashes(path)
	return bodyEditedWith(stored, content, path)
}

// bodyEdited is bodyEdited reading the stored hash from the project's workspace.
func (g *Generator) bodyEdited(content, path string) bool {
	stored, _, _ := g.scanHashes(path)
	return bodyEditedWith(stored, content, path)
}

// bodyHash is the Content-Hash of a rendered output: its body after the
// header, with a run of trailing newlines longer than maxHashedTrailingNewlines
// cut to that bound. A shorter run is hashed as rendered, so the hashes of
// existing files do not move. Without the cap a body ending in a long run of
// blank lines (an empty last section) could not be verified, and generate
// --check reported the file generate had just written as edited.
func bodyHash(output config.OutputFile) string {
	body := stripHeader(output.Content, output.Path)
	trimmed := strings.TrimRight(body, "\n")
	if len(body)-len(trimmed) > maxHashedTrailingNewlines {
		body = trimmed + strings.Repeat("\n", maxHashedTrailingNewlines)
	}
	return templates.HashContent(body)
}

func bodyEditedWith(stored, content, path string) bool {
	return genmark.BodyEdited(stored, content, path)
}

// CheckDrift renders every output in memory and reports the files that differ
// from the disk. It never writes or deletes anything. An empty result means a
// generate run would change nothing and no generated file was edited by hand.
func (g *Generator) CheckDrift(profile string) ([]Drift, error) {
	res, err := g.run(profile, CheckApplier)
	if err != nil {
		return nil, err
	}
	return res.Drift, nil
}

// VerifyGenerated checks the files listed in the committed manifest without
// re-rendering: each must exist and, when it carries a Content-Hash, still
// match it. This catches hand edits and deleted files offline and quickly, but
// not sources that changed since the last generate (use CheckDrift for that).
// checked is the number of files whose hash was compared.
func (g *Generator) VerifyGenerated() (drift []Drift, checked int, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	defer g.resetRunState()

	manifest := g.readManifest(g.manifestPath())
	if len(manifest.Files) == 0 {
		if _, statErr := g.config.StatExisting(g.manifestPath()); statErr != nil {
			return nil, 0, oops.With("manifest", g.manifestPath()).
				Hint("Run ai-rulez generate first; the manifest is written next to the configuration").
				Errorf("no generated manifest found")
		}
	}
	for _, rel := range manifest.Files {
		abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel))
		if !isUnderBaseDir(g.config.BaseDir, abs) {
			continue
		}
		data, readErr := g.config.ReadExisting(abs)
		if readErr != nil {
			drift = append(drift, Drift{Path: rel, Kind: DriftMissing})
			continue
		}
		if stored, _, _ := g.scanHashes(abs); stored != "" {
			checked++
			if g.bodyEdited(string(data), abs) {
				drift = append(drift, Drift{Path: rel, Kind: DriftEdited})
			}
		}
	}
	sortDrift(drift)
	return drift, checked, nil
}

func (g *Generator) resetRunState() {
	g.previousFiles = nil
	g.manifests = nil
	g.pluginOwned = nil
	g.refusedOutputs, g.linkedOutputs = nil, nil
}

// orphanGeneratedFiles lists generated files (those carrying a Content-Hash
// header) that no output in the plan emits. It is the manifest-free half of the
// orphan check (#313, #333): on a fresh checkout or in CI there is no previous
// manifest, so a file left behind by a removed source (an agent whose source was
// deleted, a builtin pack excluded) would otherwise pass `generate --check`. It
// walks the top-level directories the plan writes into and, below them, the rest
// of the project tree (never the whole source tree blindly: the recursive
// discovery's prune list bounds it), and reads only each candidate's header.
// generate removes what it lists (retiredFiles); --check reports it as an orphan.
func (g *Generator) orphanGeneratedFiles(outputs []config.OutputFile) []string {
	planned, roots, rootLevel := g.plannedOutputIndex(outputs)
	seen := make(map[string]bool)
	var orphans []string
	consider := func(abs string) {
		rel := g.relSlash(abs)
		if planned[rel] || seen[rel] {
			return
		}
		seen[rel] = true
		// A plugin bundle committed into the project tree carries the same
		// generated header, but no preset output plans it: its own provenance
		// sidecar owns it (#350 regression).
		if g.pluginOwnedPath(rel) {
			return
		}
		if g.cleanGeneratedLeftover(abs) {
			if len(g.keepSkipped([]string{abs})) == 0 {
				return // excluded by skip_outputs: left in place, never an orphan
			}
			orphans = append(orphans, rel)
		}
	}
	// A root-level output (AGENTS.md, .mcp.json) means the root files are
	// candidates. The root is also scanned when a preset declares a root file but
	// this run emits none: a root whose content tree is empty writes no
	// instruction file (#327), so a previously written one has to be found.
	if rootLevel || g.declaresRootLevelOutput() {
		g.considerRootFiles(consider)
	}
	for root := range roots {
		g.considerRootTree(root, consider)
	}
	// An entire nested root can be removed (#345): its generated files then sit
	// in a directory no current root's presets write into, so the output-root
	// walk above never visits them. Walk the rest of the project tree for them.
	g.considerStrayTree(roots, consider)
	sort.Strings(orphans)
	return orphans
}

// considerStrayTree walks the project tree below the project root for generated
// files no output root of this plan covers (#345): after an entire nested root is
// removed its generated files (a CLAUDE.md, a .claude/rules/*.md) sit in a
// directory no current root emits, so the output-root walk never visits them and
// `--recursive` would call the tree up to date.
//
// The walk is bounded by the recursive discovery's prune list (walkutil.SkipDirs)
// and stops at two places: this root's own config directory, whose sources are
// never generated files, and every nested current root (a directory holding a
// config file), whose own run owns its generated files — so a file a current root
// still emits, such as a shared AGENTS.md, is never reported here. A top-level
// output root (already walked by considerRootTree) is skipped too.
func (g *Generator) considerStrayTree(roots map[string]bool, consider func(string)) {
	base := filepath.Clean(g.config.BaseDir)
	configDir := filepath.Clean(g.config.ConfigDir)
	mayBeOutput := g.strayCandidateFilter(base)
	visited, opened := 0, 0
	clock := ambient.Clock(nil)
	started := clock.Now()
	defer func() {
		g.log().Debug("Scanned the project tree for orphaned generated files",
			"files", visited, "candidates", opened, "elapsed", clock.Now().Sub(started).Round(time.Millisecond))
	}()
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if !d.IsDir() {
			visited++
			// Root-level files are handled by considerRootFiles, under its own gate.
			if filepath.Dir(p) != base && mayBeOutput(p) {
				opened++
				consider(p)
			}
			return nil
		}
		if p == base {
			return nil
		}
		switch {
		case skipOrphanWalk(d.Name()):
			return filepath.SkipDir
		case g.config.ConfigDir != "" && filepath.Clean(p) == configDir:
			return filepath.SkipDir
		case config.ResolveConfigDirName(p) != "":
			// A nested current root: its own run reports and removes its files.
			return filepath.SkipDir
		case roots[g.relSlash(p)]:
			// A top-level output root, already walked by considerRootTree.
			return filepath.SkipDir
		}
		return nil
	})
}

// strayCandidateFilter returns a test for whether a file below base can be a
// generated output, so the stray-tree walk reads the header of a few files instead
// of every file in the work tree (an 89k-file monorepo spent over a minute there).
// A removed nested root left the files its presets wrote, laid out as they are
// under any root: a file named like a preset's root file (CLAUDE.md, AGENTS.md,
// .mcp.json), or one below a directory a preset writes into (.claude, .cursor).
// Without a registry every file stays a candidate.
func (g *Generator) strayCandidateFilter(base string) func(string) bool {
	if g.config.Registry == nil {
		return func(string) bool { return true }
	}
	names, dirs := map[string]bool{}, map[string]bool{}
	g.config.Registry.Each(func(_ string, gen config.PresetGenerator) {
		for _, p := range gen.GetOutputPaths(base) {
			rel, err := filepath.Rel(base, p)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			rel = filepath.ToSlash(rel)
			if idx := strings.IndexByte(rel, '/'); idx > 0 {
				dirs[rel[:idx]] = true
			} else {
				names[rel] = true
			}
		}
	})
	if len(names) == 0 && len(dirs) == 0 {
		return func(string) bool { return true }
	}
	return func(p string) bool {
		if names[filepath.Base(p)] {
			return true
		}
		rel := filepath.ToSlash(g.relSlash(p))
		for _, seg := range strings.Split(rel, "/") {
			if dirs[seg] {
				return true
			}
		}
		return false
	}
}

// plannedOutputIndex maps each output's project path, the top-level directories
// the plan writes into, and whether any output is at the root. It also marks the
// files this run must not treat as orphans: those a local overlay suppresses,
// and those the local manifest owns.
func (g *Generator) plannedOutputIndex(outputs []config.OutputFile) (planned, roots map[string]bool, rootLevel bool) {
	planned = make(map[string]bool, len(outputs))
	roots = map[string]bool{}
	for i := range outputs {
		rel := g.relSlash(g.absOutputPath(outputs[i].Path))
		planned[rel] = true
		if idx := strings.IndexByte(rel, '/'); idx > 0 {
			roots[rel[:idx]] = true
		} else {
			rootLevel = true
		}
	}
	// A file only the shared baseline produces (suppressed by a local overlay) is
	// deliberately left alone: it is not this run's to remove.
	if g.plan != nil {
		for _, rel := range g.plan.suppressed {
			planned[rel] = true
		}
	}
	// A machine-local output is named by the local manifest, not by the orphan
	// walk: with the local inputs skipped (a teammate's shared run) it must be
	// left alone, and otherwise the manifest-driven stale pass owns it.
	for _, rel := range g.readManifest(g.localManifestPath()).Files {
		planned[rel] = true
	}
	return planned, roots, rootLevel
}

// cleanGeneratedLeftover reports whether abs is a generated file left behind by
// a removed source: a regular file whose header carries a Content-Hash its body
// still matches. A symlink is the user's, and a hand-edited body is not a clean
// leftover.
func (g *Generator) cleanGeneratedLeftover(abs string) bool {
	// Read only a regular file: a symlink is the user's (removals refuse one
	// anyway), and reading a device or FIFO planted at a root path would hang.
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	hash, _, _ := g.scanHashes(abs)
	if hash == "" {
		return false
	}
	data, err := g.config.ReadExisting(abs)
	return err == nil && !g.bodyEdited(string(data), abs)
}

// pluginOwnedPath reports whether rel is a file a plugin bundle's provenance
// sidecar (plugin.ProvenanceFileName) declares. `generate --plugin` writes a
// bundle into the project tree and stamps each comment-capable file with the
// same Content-Hash header the generator writes, so a committed bundle is a
// clean leftover by that test alone. The sidecar lists the bundle's own files,
// which no preset output plans, so the orphan walk leaves them alone.
func (g *Generator) pluginOwnedPath(rel string) bool {
	for dir := pathpkg.Dir(rel); ; dir = pathpkg.Dir(dir) {
		if owned := g.pluginSidecarOutputs(dir); owned != nil {
			sub := rel
			if dir != "." {
				sub = strings.TrimPrefix(rel, dir+"/")
			}
			if owned[sub] {
				return true
			}
		}
		if dir == "." || dir == "/" {
			return false
		}
	}
}

// pluginSidecarOutputs returns the set of bundle-relative paths the plugin
// provenance sidecar in dir (project-root relative, "." for the root) declares,
// or nil when dir holds no sidecar. Reads are cached per directory, since the
// walk offers many files under one bundle.
func (g *Generator) pluginSidecarOutputs(dir string) map[string]bool {
	if owned, ok := g.pluginOwned[dir]; ok {
		return owned
	}
	if g.pluginOwned == nil {
		g.pluginOwned = map[string]map[string]bool{}
	}
	abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(dir), plugin.ProvenanceFileName)
	owned := readPluginSidecarOutputs(abs)
	g.pluginOwned[dir] = owned
	return owned
}

// readPluginSidecarOutputs parses a plugin provenance sidecar and returns its
// declared outputs, or nil when the file is absent or is not a provenance
// document (a hand-written .ai-rulez-generated.json then claims nothing).
func readPluginSidecarOutputs(abs string) map[string]bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	hashes, _, err := plugin.ProvenanceOutputs(data)
	if err != nil || len(hashes) == 0 {
		return nil
	}
	owned := make(map[string]bool, len(hashes))
	for rel := range hashes {
		owned[filepath.ToSlash(rel)] = true
	}
	return owned
}

// considerRootFiles offers each root-level file to consider.
func (g *Generator) considerRootFiles(consider func(string)) {
	entries, err := os.ReadDir(g.config.BaseDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			consider(filepath.Join(g.config.BaseDir, e.Name()))
		}
	}
}

// considerRootTree offers each file under a top-level output directory to
// consider, skipping VCS and dependency trees.
func (g *Generator) considerRootTree(root string, consider func(string)) {
	abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(root))
	_ = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // unreadable entries are skipped
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if d.IsDir() {
			if skipOrphanWalk(d.Name()) {
				return filepath.SkipDir
			}
			// A nested current root owns its generated files: its own run
			// reports and removes them, never this one (#368).
			if config.ResolveConfigDirName(p) != "" {
				return filepath.SkipDir
			}
			return nil
		}
		consider(p)
		return nil
	})
}

// skipOrphanWalk names the directories an orphan walk never descends into: the
// same dependency, build and VCS trees the recursive discovery prunes
// (walkutil.SkipDirs). It deliberately does not use walkutil.ShouldSkipDir: that
// also skips hidden directories, and generated outputs live in hidden ones
// (.claude, .cursor, .agents).
func skipOrphanWalk(name string) bool {
	_, skip := walkutil.SkipDirs[name]
	return skip
}

// declaresRootLevelOutput reports whether any configured preset declares an
// output directly under the project root (CLAUDE.md, AGENTS.md, .mcp.json), so
// the orphan walk considers root files even when this run emits none of them.
// A root whose selected content tree is empty writes no instruction file
// (#327); without this the previously written root file could not be found and
// removed as an orphan.
func (g *Generator) declaresRootLevelOutput() bool {
	if g.config.Registry == nil {
		return false
	}
	base := filepath.Clean(g.config.BaseDir)
	found := false
	g.config.Registry.Each(func(_ string, gen config.PresetGenerator) {
		if found {
			return
		}
		for _, p := range gen.GetOutputPaths(base) {
			if filepath.Clean(filepath.Dir(p)) == base {
				found = true
				return
			}
		}
	})
	return found
}

func (g *Generator) relSlash(abs string) string {
	return filepath.ToSlash(g.convertToRelativePath(abs))
}

func sortDrift(d []Drift) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Path != d[j].Path {
			return d[i].Path < d[j].Path
		}
		return d[i].Kind < d[j].Kind
	})
}
