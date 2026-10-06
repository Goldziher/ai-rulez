package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

// committedSBOMNames are the files `validate --strict` checks for drift (AR753)
// when they exist at the project root and were made by `ai-rulez sbom`. The
// document records how it was made, so the check can rebuild it without flags.
var committedSBOMNames = []string{"ai-bom.cdx.json", "sbom.cdx.json"}

// maxCommittedSBOMBytes bounds the committed document read for the drift check.
const maxCommittedSBOMBytes = 32 << 20

// sbomFindingsFor builds the SBOM of cfg and returns its findings for
// `validate --strict`: AR750 and AR751 for MCP packages and remote sources that
// cannot be given an exact version or a package URL, AR752 for a lock that no
// longer matches the sources, and AR753 for a committed CycloneDX SBOM that
// differs from a fresh one. Nothing is written and the network is not used. An
// SBOM that cannot be built is skipped with a warning: `ai-rulez sbom` reports it.
func sbomFindingsFor(cfg *config.Config) []lint.SBOMFinding {
	bom, err := sbom.Build(cfg, Version, sbom.Options{Now: time.Now()})
	if err != nil {
		logger.Warn("Skipped the SBOM checks", "error", err)
		return nil
	}
	var out []lint.SBOMFinding
	for _, f := range bom.Findings {
		if f.Code == sbom.CodeUnpinned || f.Code == sbom.CodeUnknownCoords {
			out = append(out, lint.SBOMFinding{Code: f.Code, Message: fmt.Sprintf("%s: %s", f.Subject, f.Message)})
		}
	}
	if bom.LockPresent && !bom.LockInSync {
		out = append(out, lint.SBOMFinding{Code: sbom.CodeLockStale, Path: lockRelPath(cfg),
			Message: "the lock does not match the sources, so an SBOM would not describe what it pins; run `ai-rulez lock`, or `ai-rulez lock --diff` to see what changed"})
	}
	return append(out, committedSBOMDrift(cfg)...)
}

func lockRelPath(cfg *config.Config) string {
	rel, err := filepath.Rel(cfg.BaseDir, filepath.Join(cfg.ConfigDir, "ai-rulez.lock"))
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// committedSBOMDrift compares each committed ai-rulez SBOM at the project root
// with one rebuilt the way the document says it was made.
func committedSBOMDrift(cfg *config.Config) []lint.SBOMFinding {
	var out []lint.SBOMFinding
	for _, name := range committedSBOMNames {
		path := filepath.Join(cfg.BaseDir, name)
		data, ok := readCommittedSBOM(path)
		if !ok {
			continue
		}
		recipe, ok := sbom.RecipeOf(data)
		if !ok {
			continue
		}
		at := recipe.Timestamp
		if t, found := committedTime(path); found {
			at = t
		}
		// Whether approvals were left out is not recorded: the document is current
		// when either reading of it matches.
		var diffs []string
		matched := false
		for _, noApprovals := range []bool{false, true} {
			o := recipe
			o.NoApprovals, o.Now = noApprovals, at
			fresh, err := renderCycloneDX(cfg, o)
			if err != nil {
				logger.Warn("Skipped the committed SBOM check", "file", name, "error", err)
				matched = true
				break
			}
			d, err := sbom.Drift(data, fresh)
			if err != nil || len(d) == 0 {
				matched = true
				break
			}
			if diffs == nil {
				diffs = d
			}
		}
		if matched {
			continue
		}
		out = append(out, lint.SBOMFinding{Code: sbom.CodeDrift, Path: name,
			Message: fmt.Sprintf("%s differs from the SBOM generated now (%s); regenerate it with `ai-rulez sbom -o %s`, with the flags it was made with", name, strings.Join(firstDiffs(diffs, 3), "; "), name)})
	}
	return out
}

func firstDiffs(diffs []string, n int) []string {
	if len(diffs) > n {
		return append(append([]string(nil), diffs[:n]...), fmt.Sprintf("and %d more", len(diffs)-n))
	}
	return diffs
}

func renderCycloneDX(cfg *config.Config, o sbom.Options) ([]byte, error) {
	bom, err := sbom.Build(cfg, Version, o)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	var buf bytes.Buffer
	if err := sbom.Render(&buf, bom, sbom.FormatCycloneDX); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return buf.Bytes(), nil
}

func readCommittedSBOM(path string) ([]byte, bool) {
	f, err := os.Open(path) //nolint:gosec // a fixed name at the project root
	if err != nil {
		return nil, false
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCommittedSBOMBytes {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCommittedSBOMBytes))
	return data, err == nil
}
