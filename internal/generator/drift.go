package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
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
		kind = rawState(abs, output)
		return kind, kind != "", true
	}
	final := g.finalContent(output)
	if g.isUnmanagedRuleFile(abs, final) {
		return "", false, false
	}
	existing, err := os.ReadFile(abs)
	if err != nil {
		return DriftMissing, true, true
	}
	rewrite = !g.canSkipWrite(abs, output, final)
	switch {
	case g.config.GetHeaderHashes() != config.HeaderHashesNone && bodyEdited(string(existing), abs):
		return DriftEdited, rewrite, true
	case rewrite:
		return DriftStale, true, true
	}
	return "", false, true
}

func rawState(abs string, output config.OutputFile) DriftKind {
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
	if _, err := os.Stat(abs); err != nil {
		return DriftMissing
	}
	if !rawWriteCanSkip(abs, output.RawContent, mode) {
		return DriftStale
	}
	return ""
}

// maxHashedTrailingNewlines bounds the trailing-newline shapes bodyEdited tries.
const maxHashedTrailingNewlines = 3

// bodyEdited reports whether content, read from path, no longer matches the
// Content-Hash stored in its header. A file without a stored hash cannot be
// judged and is not reported.
func bodyEdited(content, path string) bool {
	stored, _, _ := scanStoredHashes(path)
	if stored == "" {
		return false
	}
	// The hash is taken before generate normalizes the file to one trailing
	// newline, so any run of trailing newlines in the rendering may have been
	// collapsed; accept the few shapes a renderer produces.
	trimmed := strings.TrimRight(stripHeader(content, path), "\n")
	for newlines := 0; newlines <= maxHashedTrailingNewlines; newlines++ {
		if templates.HashContent(trimmed+strings.Repeat("\n", newlines)) == stored {
			return false
		}
	}
	return true
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
		if _, statErr := os.Stat(g.manifestPath()); statErr != nil {
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
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			drift = append(drift, Drift{Path: rel, Kind: DriftMissing})
			continue
		}
		if stored, _, _ := scanStoredHashes(abs); stored != "" {
			checked++
			if bodyEdited(string(data), abs) {
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
