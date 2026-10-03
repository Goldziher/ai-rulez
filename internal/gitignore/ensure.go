package gitignore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/samber/oops"
)

// PatternsInsideFence returns the trimmed pattern lines inside the ai-rulez
// managed BEGIN/END fence, in file order.
func PatternsInsideFence(content string) []string {
	var patterns []string
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == BeginMarker:
			inBlock = true
		case trimmed == EndMarker:
			inBlock = false
		case inBlock && trimmed != "":
			patterns = append(patterns, trimmed)
		}
	}
	return patterns
}

// hasCompleteFence reports whether a BEGIN marker is followed by an END marker.
func hasCompleteFence(content string) bool {
	begun := false
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			begun = true
		case EndMarker:
			if begun {
				return true
			}
		}
	}
	return false
}

// allPatterns returns every non-comment, non-empty line, fence or not.
func allPatterns(content string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
			out[t] = true
		}
	}
	return out
}

// EnsureEntries makes sure each pattern is ignored by baseDir/.gitignore. A
// pattern already present anywhere in the file (inside or outside the managed
// fence) is left alone; missing ones are appended to the ai-rulez managed fence,
// which is created when absent. The call is idempotent.
//
// A .gitignore that is a symbolic link is never written through (git would not
// read it): the entries go to .git/info/exclude instead, see ReplaceViaExclude.
func EnsureEntries(baseDir string, patterns []string) error {
	path := filepath.Join(baseDir, ".gitignore")
	if IsSymlink(baseDir) {
		return ensureViaExclude(baseDir, patterns)
	}
	data, err := gitutil.ReadIgnoreFileOrEmpty(path)
	if err != nil && !os.IsNotExist(err) {
		return oops.With("path", path).Wrapf(err, "read .gitignore")
	}
	content := string(data)

	inside, have, broken := knownPatterns(content)
	entries := inside
	changed := false
	for _, p := range patterns {
		if p = strings.TrimSpace(p); p != "" && !have[p] {
			entries = append(entries, p)
			have[p] = true
			changed = true
		}
	}
	if !changed {
		return nil
	}

	var block strings.Builder
	block.WriteString(BeginMarker + "\n")
	for _, p := range entries {
		block.WriteString(p + "\n")
	}
	block.WriteString(EndMarker + "\n")

	out := spliceFence(content, block.String(), broken)
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil { //nolint:gosec // .gitignore is meant to be world-readable
		return oops.With("path", path).Wrapf(err, "write .gitignore")
	}
	return nil
}

// knownPatterns returns the patterns inside the managed fence, the set of every
// pattern already present, and whether the fence is unterminated (BEGIN with no
// END). In that case every line is user content, so none is absorbed.
func knownPatterns(content string) (inside []string, have map[string]bool, broken bool) {
	if strings.Contains(content, BeginMarker) && !hasCompleteFence(content) {
		return nil, allPatterns(content), true
	}
	inside = PatternsInsideFence(content)
	have = PatternsOutsideFence(content)
	for _, p := range inside {
		have[p] = true
	}
	return inside, have, false
}

// spliceFence puts block into content: replacing the existing fence, appending a
// fresh one after an unterminated fence or existing lines, or starting the file.
func spliceFence(content, block string, broken bool) string {
	switch {
	case broken:
		return strings.TrimRight(content, "\n") + "\n\n" + block
	case strings.Contains(content, BeginMarker):
		return ReplaceFencedBlock(content, block)
	case content == "":
		return block
	}
	out := content
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + "\n" + block
}
