package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
			out = append(out, demoteFrontmatterless(f))
		}
	}
	return b, out, true
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

// validateOKFTree reports the OKF findings of the project and returns true when
// they reach the --fail-on threshold (error by default). Text goes to w. A
// structured --format gets the OKF document on stdout (or --output) only when
// the run fails here, since a clean run goes on to print the lint report.
func validateOKFTree(cfg *config.Config, w io.Writer) bool {
	b, findings, ok := okfTreeFindings(cfg)
	if !ok {
		return false
	}
	failed := validateFailOn != okfFailNone && okfFails(findings, okfFailThreshold())
	if structuredFormat(validateFormat) {
		if failed {
			if err := writeOKFTreeJSON(cfg.ConfigDir, b, findings); err != nil {
				renderStderr(err)
			}
		}
		return failed
	}
	if err := writeOKFTreeText(w, cfg.ConfigDir, b, findings, viper.GetBool("quiet")); err != nil {
		renderStderr(err)
		return true
	}
	return failed
}

func okfFailThreshold() okf.Severity {
	switch validateFailOn {
	case failOnWarning:
		return okf.SeverityWarning
	case failOnInfo:
		return okf.SeverityInfo
	}
	return okf.SeverityError
}

// writeOKFTreeText prints the findings and, unless quiet, the summary line that
// writeOKFFindings always ends with.
func writeOKFTreeText(w io.Writer, dir string, b *okf.Bundle, findings []okf.Finding, quiet bool) error {
	if !quiet {
		return writeOKFFindings(w, dir, b, findings, false)
	}
	var buf bytes.Buffer
	if err := writeOKFFindings(&buf, dir, b, findings, false); err != nil {
		return err
	}
	lines := strings.SplitAfter(buf.String(), "\n")
	// SplitAfter leaves an empty last element; the summary is the one before it.
	_, err := io.WriteString(w, strings.Join(lines[:max(len(lines)-2, 0)], ""))
	return err //nolint:wrapcheck // writer error
}

// writeOKFTreeJSON writes the OKF document to --output (atomically) or stdout.
func writeOKFTreeJSON(dir string, b *okf.Bundle, findings []okf.Finding) error {
	if validateOutput == "" {
		return writeOKFFindings(os.Stdout, dir, b, findings, true)
	}
	var buf bytes.Buffer
	if err := writeOKFFindings(&buf, dir, b, findings, true); err != nil {
		return err
	}
	return oops.With("path", validateOutput).Wrapf(gitutil.WriteFileAtomic(validateOutput, buf.Bytes(), 0o644), "write report")
}
