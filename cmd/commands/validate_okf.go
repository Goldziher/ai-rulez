package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// isOKFContentDir reports whether a top-level directory of a configuration
// directory holds markdown the OKF validation covers.
func isOKFContentDir(name string) bool {
	switch name {
	case "rules", "context", "skills", "agents", "commands", "checks", "domains":
		return true
	}
	return false
}

// isOKFResourceDir reports whether a directory holds the supporting files of a
// skill or command. They keep the Agent Skills layout and carry no OKF
// frontmatter, so they are not concepts.
func isOKFResourceDir(name string) bool {
	switch name {
	case "references", "scripts", "assets":
		return true
	}
	return false
}

// okfTreeFindings runs "okf validate" on a project whose configuration directory
// is an OKF bundle, that is one with an index.md at its root (see "migrate okf").
// Only the content directories are checked; the second result is false when the
// directory is not a bundle, or cannot be read, and nothing was validated.
func okfTreeFindings(cfg *config.Config) (*okf.Bundle, []okf.Finding, bool) {
	if cfg == nil || cfg.ConfigDir == "" {
		return nil, nil, false
	}
	if info, err := os.Stat(filepath.Join(cfg.ConfigDir, okf.IndexFile)); err != nil || info.IsDir() {
		return nil, nil, false
	}
	b, err := okf.Load(os.DirFS(cfg.ConfigDir))
	if err != nil {
		return nil, nil, false
	}
	var out []okf.Finding
	for _, f := range append(append(b.CheckRoot(), b.Validate()...), okfbridge.KindFindings(b)...) {
		if okfTreePath(f.Path) && !okfResourceDirFinding(f) {
			out = append(out, demoteFrontmatterless(f))
		}
	}
	return b, out, true
}

// staleOKFIndexFindings reports, as AR9B0, every index.md of an OKF configuration
// directory that lists an old title or description (or lacks an entry), which is
// what `migrate okf --check` reports and `generate` rewrites. An index that
// already carries an AR9B0 finding in seen is not reported twice.
func staleOKFIndexFindings(ctx context.Context, cfg *config.Config, seen []okf.Finding) []okf.Finding {
	stale, err := staleOKFIndexes(ctx, cfg)
	if err != nil {
		logger.Warn("Skipped the OKF index freshness check", "error", err)
		return nil
	}
	listed := map[string]bool{}
	for _, f := range seen {
		if f.Code == okf.CodeIndexMismatch {
			listed[f.Path] = true
		}
	}
	var out []okf.Finding
	for _, rel := range stale {
		if !listed[rel] {
			out = append(out, okf.NewFinding(okf.CodeIndexMismatch, rel, 1,
				"the index lists a title or description that differs from its concepts; run `ai-rulez generate` to refresh it"))
		}
	}
	return out
}

// okfNoFrontmatterPrefix starts the message okf.Validate gives a concept that has
// no frontmatter block at all.
const okfNoFrontmatterPrefix = "no frontmatter block"

// demoteFrontmatterless turns the "needs a type" error for a file with no
// frontmatter block into a warning. A rule or context file may be plain
// markdown: generate, migrate and every other command accept it, so validate must
// not fail the run for it (a block with a missing or unparseable type stays an
// error). "migrate okf" adds the type.
func demoteFrontmatterless(f okf.Finding) okf.Finding {
	if f.Code == okf.CodeTypeInvalid && strings.HasPrefix(f.Message, okfNoFrontmatterPrefix) {
		f.Severity = okf.SeverityWarning
		f.Message += " (plain markdown is accepted; run `ai-rulez migrate okf` to add it)"
	}
	return f
}

// okfResourceDirFinding reports whether a finding asks an index to list a skill
// or command resource directory; migrated trees keep those unlisted on purpose.
func okfResourceDirFinding(f okf.Finding) bool {
	if f.Code != okf.CodeIndexMismatch {
		return false
	}
	for _, dir := range []string{"references", "scripts", "assets"} {
		if strings.HasPrefix(f.Message, "subdirectory "+dir+"/ ") {
			return true
		}
	}
	return false
}

// okfTreePath reports whether a finding path belongs to the content OKF models:
// the root index, or a file in a content directory that is not a resource.
func okfTreePath(p string) bool {
	if p == okf.IndexFile || p == "" {
		return true
	}
	parts := strings.Split(p, "/")
	if !isOKFContentDir(parts[0]) {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if isOKFResourceDir(part) {
			return false
		}
	}
	return true
}
