package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func (g *Generator) manifestDir() string {
	if g.config.ConfigDir == "" {
		return filepath.Join(g.config.BaseDir, ".ai-rulez")
	}
	return g.config.ConfigDir
}

func (g *Generator) manifestPath() string {
	return filepath.Join(g.manifestDir(), generatedManifestName)
}

func (g *Generator) localManifestPath() string {
	return filepath.Join(g.manifestDir(), generatedLocalManifestName)
}

// beginRun forgets the manifests the previous run of this Generator read. The
// warnings it issued are kept: clean plans (and so warns) once to show the plan
// and again to apply it, and the user should see each only once.
func (g *Generator) beginRun() {
	g.manifests = nil
	g.localManifestPending = false
	g.diagnostics() // the run's warnings share one collector from the start
}

// planLocalManifest records whether this run is going to write the machine-local
// manifest, so the gitignore pass that precedes the write already lists it: the
// file on disk is not there yet on a first run, and the two runs would otherwise
// leave different .gitignore blocks.
func (g *Generator) planLocalManifest(outputs []config.OutputFile) {
	g.localManifestPending = false
	if g.localSkipped {
		return
	}
	for _, output := range outputs {
		if !output.IsDir && !output.PartiallyOwned && output.LocalOnly {
			g.localManifestPending = true
			return
		}
	}
	// The record is also written for plain generated documents and digests, but
	// only machine-local content forces its .gitignore entry; with managed
	// ignores on, collectGitignorePaths lists it anyway.
	for _, output := range outputs {
		if !output.IsDir && len(output.MergeClaims) > 0 &&
			(output.PartiallyOwned || output.Sensitive || output.LocalOnly) {
			g.localManifestPending = true
			return
		}
	}
	committed, local := g.splitMergedClaims(outputs)
	g.localManifestPending = len(local) > len(committed)
}

// readManifest reads a manifest at most once per run, so a corrupt one is
// reported once and every consumer sees the same content.
func (g *Generator) readManifest(path string) generatedManifest {
	if manifest, ok := g.manifests[path]; ok {
		return manifest
	}
	var manifest generatedManifest
	if path == g.localManifestPath() && g.localManifestUntrusted() {
		manifest = generatedManifest{}
	} else {
		manifest = readManifestFile(g.log(), g.config.ReadExisting, path)
	}
	if g.manifests == nil {
		g.manifests = map[string]generatedManifest{}
	}
	g.manifests[path] = manifest
	return manifest
}

// localManifestUntrusted reports whether the machine-local manifest must be
// ignored: a repository can commit that file with forged claims and digests, and
// .gitignore does not stop a tracked one. It warns once per Generator.
func (g *Generator) localManifestUntrusted() bool {
	reason := g.git().UntrustedLocalFileContext(g.ctx, g.localManifestPath())
	if _, committed := workspace.CommitOf(g.config.Workspace); committed && reason == "" {
		// A commit holds whatever its author chose: a machine-local record found
		// there was committed, not written by this machine.
		if _, err := g.config.StatExisting(g.localManifestPath()); err == nil {
			reason = "it is part of a commit"
		}
	}
	if reason == "" {
		return false
	}
	g.warnOnce("Ignoring "+g.localManifestRel()+": "+reason+"; its claims and digests are not trusted",
		"fix", g.localManifestFix(reason))
	return true
}

func (g *Generator) localManifestRel() string {
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" {
		return rel
	}
	return generatedLocalManifestName
}

func (g *Generator) localManifestFix(reason string) string {
	if reason == "git tracks it" {
		return "git rm --cached " + g.localManifestRel()
	}
	return "delete it and run generate again"
}

// localManifestTracked reports whether git tracks the machine-local manifest. A
// run never writes claims into such a file: a hostile repository could have
// pre-filled it, and the write would turn into a tracked change.
func (g *Generator) localManifestTracked() bool {
	if !g.git().IsTrackedContext(g.ctx, g.localManifestPath()) {
		return false
	}
	g.warnOnce("Not writing "+g.localManifestRel()+": git tracks it, and it must stay machine-local",
		"fix", "git rm --cached "+g.localManifestRel())
	return true
}

func readManifestFile(log logger.Logger, read jsonmerge.Reader, path string) generatedManifest {
	data, err := read(path)
	if err != nil {
		return generatedManifest{}
	}
	var manifest generatedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		logger.Or(log).Warn("Ignoring invalid generated manifest", "path", path, "error", err)
		return generatedManifest{}
	}
	return manifest
}

// previousManifestFiles returns every path the previous run generated: the
// committed manifest plus the machine-local one. Committed entries that name a
// ".local." file are ignored, because only the gitignored local manifest may
// authorize deleting machine-local outputs (older versions recorded them in the
// committed manifest, where a teammate's run would delete them).
func (g *Generator) previousManifestFiles() []string {
	var files []string
	for _, f := range g.readManifest(g.manifestPath()).Files {
		if !strings.Contains(filepath.Base(filepath.FromSlash(f)), ".local.") {
			files = append(files, f)
		}
	}
	if g.localSkipped {
		// A run that deliberately ignores local inputs must not clean up the
		// outputs of the inputs it ignored.
		return files
	}
	return append(files, g.readManifest(g.localManifestPath()).Files...)
}

// writeGeneratedManifest records the generated files so the next run can delete
// the ones that dropped out. Partially owned outputs are deliberately left out:
// the manifest exists only to drive deletion, and deleting a file ai-rulez
// merely contributed a key to would take the hand-authored remainder with it.
// Machine-local outputs go to the separate, gitignored local manifest, which is
// removed when no local output remains.
func (g *Generator) writeGeneratedManifest(outputs []config.OutputFile) error {
	committedMerged, localMerged := g.splitMergedClaims(outputs)
	shared, local := g.manifestEntries(outputs)
	if g.plan != nil {
		// The committed manifest describes the shared baseline, not this machine.
		shared = g.plan.sharedManifestFiles(g.skippedPaths)
	}
	defer func() { g.manifests = nil }()
	// Digests, like claims, are proof only when this machine recorded them: they go
	// to the machine-local manifest, never the committed one.
	if err := g.writeManifest(g.manifestPath(), shared, committedMerged, nil); err != nil {
		return err
	}
	digests := g.manifestDigests(g.config.BaseDir, append(slices.Clone(shared), local...))
	digests = g.addMergedDigests(digests, localMerged, g.wholeMergedDocuments(outputs))
	if g.localSkipped {
		// Local files were deliberately not loaded: their manifest is not ours to
		// drop. This run only adds what it rendered; the rest of the record stays.
		return g.updateLocalManifest(localMerged, digests)
	}
	if len(local) == 0 && len(localMerged) == 0 && len(digests) == 0 {
		return g.removeLocalManifest()
	}
	return g.writeManifest(g.localManifestPath(), local, localMerged, digests)
}

// manifestEntries splits the relative paths of outputs into the shared entries
// and the machine-local ones, leaving out directories, partially owned
// documents and the paths this run skipped.
func (g *Generator) manifestEntries(outputs []config.OutputFile) (shared, local []string) {
	for _, output := range outputs {
		if output.IsDir || output.PartiallyOwned {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if g.userMode && filepath.IsAbs(filepath.FromSlash(rel)) {
			// No relative form (another volume): a later run could not resolve the entry.
			g.log().Warn("Not recording a generated file the manifest cannot express", "path", rel)
			continue
		}
		switch {
		case g.skippedPaths[rel]:
		case output.LocalOnly:
			local = append(local, rel)
		default:
			shared = append(shared, rel)
		}
	}
	return shared, local
}

// removeLocalManifest deletes the machine-local manifest once no local output
// remains, unless it is tracked or sits behind a link that leaves the project.
func (g *Generator) removeLocalManifest() error {
	if !g.removalConfined(g.localManifestPath()) || g.localManifestTracked() {
		return nil
	}
	if err := os.Remove(g.localManifestPath()); err != nil && !os.IsNotExist(err) {
		return oops.With("path", g.localManifestPath()).Wrapf(err, "remove local manifest")
	}
	return nil
}

// updateLocalManifest folds this run's claims and digests into the machine-local
// manifest of a run that did not load the local inputs. A document keeps the
// earlier claims on paths this run does not render, so a server only the local
// overlay defines is still taken back by the next full run.
func (g *Generator) updateLocalManifest(merged map[string][]jsonmerge.Claim, digests map[string]string) error {
	if len(merged) == 0 && len(digests) == 0 {
		return nil
	}
	prev := g.readManifest(g.localManifestPath())
	outMerged := map[string][]jsonmerge.Claim{}
	for rel, claims := range prev.Merged {
		outMerged[rel] = claims
	}
	for rel, claims := range merged {
		kept := slices.Clone(claims)
		for i := range outMerged[rel] {
			old := outMerged[rel][i]
			if !slices.ContainsFunc(claims, func(c jsonmerge.Claim) bool { return slices.Equal(c.Path, old.Path) }) {
				kept = append(kept, old)
			}
		}
		outMerged[rel] = kept
	}
	outDigests := map[string]string{}
	for rel, sum := range prev.Digests {
		outDigests[rel] = sum
	}
	for rel, sum := range digests {
		outDigests[rel] = sum
	}
	return g.writeManifest(g.localManifestPath(), prev.Files, outMerged, outDigests)
}

// writeManifest writes a manifest after refusing a symlink that leaves the project.
func (g *Generator) writeManifest(path string, files []string, merged map[string][]jsonmerge.Claim,
	digests map[string]string) error {
	if path == g.localManifestPath() && g.localManifestTracked() {
		return nil
	}
	resolved, _, err := g.guardWrite(path)
	if err != nil {
		return err
	}
	return writeManifestFileDirs(resolved, files, merged, digests, g.manifestDirs(path))
}

func writeManifestFile(path string, files []string, merged map[string][]jsonmerge.Claim) error {
	return writeManifestFileDirs(path, files, merged, nil, nil)
}

func writeManifestFileDirs(path string, files []string, merged map[string][]jsonmerge.Claim,
	digests map[string]string, dirs *[]string) error {
	sort.Strings(files)
	files = slices.Compact(files)
	if files == nil {
		files = []string{}
	}
	data, err := json.MarshalIndent(generatedManifest{Version: "1", Files: files, Merged: merged, Digests: digests, Dirs: dirs}, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "marshal generated manifest")
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.With("dir", filepath.Dir(path)).Wrapf(err, "create manifest directory")
	}
	// path was resolved by guardWrite; the temp file + rename never truncates a
	// manifest in place or follows a link planted since.
	return config.WriteFileAtomic(path, data, 0o644)
}
