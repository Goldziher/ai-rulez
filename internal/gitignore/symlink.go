package gitignore

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// IsSymlink reports whether baseDir/.gitignore is a symbolic link. Git does not
// read a symlinked .gitignore from the work tree, so entries written through the
// link would protect nothing.
func IsSymlink(baseDir string) bool {
	info, err := os.Lstat(filepath.Join(baseDir, ".gitignore"))
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// FallbackMarkers are the lines delimiting the project's block in
// .git/info/exclude that stands in for a symlinked .gitignore.
func FallbackMarkers(baseDir string) (begin, end string) {
	key := gitutil.Resolve(baseDir)
	return "# BEGIN ai-rulez (symlinked .gitignore): " + key, "# END ai-rulez (symlinked .gitignore): " + key
}

// ReplaceViaExclude makes the project's block in .git/info/exclude hold exactly
// patterns, for a project whose .gitignore is a symlink git ignores. An empty
// list removes the block. Outside a repository there is nothing git could
// commit, so it only warns.
func ReplaceViaExclude(log logger.Logger, baseDir string, patterns []string) error {
	return writeExcludeFallback(log, baseDir, patterns, false)
}

// ensureViaExclude adds patterns to that block, keeping its other entries.
func ensureViaExclude(log logger.Logger, baseDir string, patterns []string) error {
	return writeExcludeFallback(log, baseDir, patterns, true)
}

func writeExcludeFallback(log logger.Logger, baseDir string, patterns []string, keep bool) error {
	exclude := gitutil.InfoExcludePath(baseDir)
	if exclude == "" {
		logger.Or(log).Warn(".gitignore is a symbolic link and this is not a git repository; not writing ignore entries through the link")
		return nil
	}
	logger.Or(log).Warn(".gitignore is a symbolic link, which git does not read; writing ignore entries to .git/info/exclude instead",
		"path", exclude)

	data, err := gitutil.ReadIgnoreFile(exclude)
	if err != nil && !os.IsNotExist(err) {
		return oops.With("path", exclude).Wrapf(err, "read git exclude file")
	}
	content := string(data)
	begin, end := FallbackMarkers(baseDir)

	entries := map[string]bool{}
	if keep {
		entries = blockEntries(content, begin, end)
	}
	top := gitutil.TopLevel(baseDir)
	prefix := gitutil.RepoRelative(top, baseDir)
	for _, p := range patterns {
		if p = strings.TrimSpace(p); p != "" && !strings.ContainsAny(p, "\r\n") {
			if a := anchored(prefix, p); a != "" {
				entries[a] = true
			}
		}
	}

	block := ""
	if len(entries) > 0 {
		sorted := make([]string, 0, len(entries))
		for e := range entries {
			sorted = append(sorted, e)
		}
		sort.Strings(sorted)
		block = begin + "\n" + strings.Join(sorted, "\n") + "\n" + end + "\n"
	}
	updated := ReplaceMarkedBlock(content, begin, end, block)
	if updated == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return oops.With("path", exclude).Wrapf(err, "create git info directory")
	}
	if err := gitutil.WriteFileAtomic(exclude, []byte(updated), 0o644); err != nil {
		return oops.With("path", exclude).Wrapf(err, "write git exclude file")
	}
	return nil
}

// anchored rewrites a .gitignore pattern of a project at repoPrefix (relative to
// the work tree root; "" or "." for the root) so it means the same in the
// repository's exclude file, where patterns are relative to the repository
// root. A pattern with a slash before its last character is relative to the
// project directory; one without matches at any depth below it. It returns ""
// for a negation, which must not leak out of the project's block.
func anchored(repoPrefix, pattern string) string {
	if strings.HasPrefix(pattern, "!") {
		return ""
	}
	if repoPrefix == "" || repoPrefix == "." {
		return pattern
	}
	if strings.Contains(strings.TrimSuffix(pattern, "/"), "/") {
		return "/" + repoPrefix + "/" + strings.TrimPrefix(pattern, "/")
	}
	return "/" + repoPrefix + "/**/" + pattern
}

func blockEntries(content, begin, end string) map[string]bool {
	entries := map[string]bool{}
	in := false
	for _, line := range strings.Split(content, "\n") {
		switch t := strings.TrimSpace(line); {
		case t == begin:
			in = true
		case t == end:
			in = false
		case in && t != "":
			entries[t] = true
		}
	}
	return entries
}
