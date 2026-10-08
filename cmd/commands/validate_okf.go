package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
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
	for _, f := range append(b.CheckRoot(), b.Validate()...) {
		if okfTreePath(f.Path) && !okfResourceDirFinding(f) {
			out = append(out, f)
		}
	}
	return b, out, true
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

// validateOKFTree reports the OKF findings of the project to w and returns true
// when they reach the --fail-on threshold (error by default).
func validateOKFTree(cfg *config.Config, w io.Writer) bool {
	b, findings, ok := okfTreeFindings(cfg)
	if !ok {
		return false
	}
	if err := writeOKFFindings(w, cfg.ConfigDir, b, findings, false); err != nil {
		fmtError(err)
		return true
	}
	threshold := okf.SeverityError
	switch validateFailOn {
	case failOnWarning:
		threshold = okf.SeverityWarning
	case "info":
		threshold = okf.SeverityInfo
	case okfFailNone:
		return false
	}
	return okfFails(findings, threshold)
}
