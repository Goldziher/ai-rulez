package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The cache key of a staged scanner covers its binary and its argument list. A
// scanner that is a script run through an interpreter (python3 scan.py) is not
// its binary, so the script is hashed into the key as well, and cache_files adds
// the files a scanner loads on its own (rules, configuration).

const (
	// maxHashedFile bounds a file read into the key; a larger one is keyed by size and time.
	maxHashedFile = 64 << 20
	// maxCacheFiles bounds how many files the cache_files globs may match.
	maxCacheFiles = 20000
)

// isInterpreter reports whether the command is a known interpreter, whose first
// file argument is the program it runs: a shell, python (python3.12, pypy3),
// node, deno, bun, ruby, perl, php, lua, R or PowerShell, and uv or pipx for
// `uv run script.py` and `pipx run script.py`.
func isInterpreter(command string) bool {
	name := strings.ToLower(path.Base(strings.ReplaceAll(command, `\`, "/")))
	for _, ext := range []string{extExe, ".cmd", ".bat"} {
		name = strings.TrimSuffix(name, ext)
	}
	switch strings.TrimRight(name, "0123456789.") {
	case "sh", "bash", "zsh", "dash", "ksh", "fish",
		"python", "py", "pypy", "node", "nodejs", "deno", "bun", "tsx", "ts-node",
		"ruby", "perl", "php", "lua", "rscript", "pwsh", "powershell", "uv", "pipx":
		return true
	}
	return false
}

// interpreterScript returns the absolute path of the file an interpreter command
// runs: the first argument that does not start with a dash and names an existing
// regular file (a relative one is looked up from root). It is "" when command[0]
// is not a known interpreter or no argument names a file.
func interpreterScript(command []string, root string) string {
	if len(command) < 2 || !isInterpreter(command[0]) {
		return ""
	}
	for _, arg := range command[1:] {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		p := filepath.FromSlash(strings.ReplaceAll(arg, phProject, filepath.ToSlash(root)))
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// cacheFilesProblems lists what is wrong with an entry's cache_files patterns.
func cacheFilesProblems(patterns []string) []string {
	return globListProblems("cache_files", "project", patterns)
}

func hasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// keyFiles digests what the result of sc depends on besides its staged content:
// the interpreter script and the cache_files matches, as sorted "name:hash"
// strings. A file that cannot be read is keyed as unreadable, so it still
// changes the key when it appears or disappears.
func keyFiles(sc resolvedScanner, root string) []string {
	var out []string
	if script := interpreterScript(sc.Command, root); script != "" {
		out = append(out, "script:"+hashFile(script))
	}
	for _, rel := range expandCacheGlobs(root, sc.CacheFiles) {
		out = append(out, rel+":"+hashFile(filepath.Join(root, filepath.FromSlash(rel))))
	}
	return out
}

func hashFile(p string) string {
	info, err := os.Stat(p)
	if err != nil {
		return cacheStateUnrd
	}
	if info.Size() > maxHashedFile {
		return fmt.Sprintf("big:%d:%d", info.Size(), info.ModTime().UnixNano())
	}
	f, err := os.Open(p) //nolint:gosec // a file the configuration names
	if err != nil {
		return cacheStateUnrd
	}
	defer f.Close() //nolint:errcheck // read only
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return cacheStateUnrd
	}
	return hex.EncodeToString(h.Sum(nil))
}

// expandCacheGlobs lists the regular files below root that match any pattern, as
// sorted root-relative slash paths. Symbolic links and .git are not followed.
func expandCacheGlobs(root string, patterns []string) []string {
	seen := map[string]bool{}
	for _, pattern := range patterns {
		g, ok := newGlob(pattern)
		if !ok || hasDotDot(strings.ReplaceAll(pattern, `\`, "/")) {
			continue
		}
		start := filepath.Join(root, filepath.FromSlash(literalPrefix(pattern)))
		_ = filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // a missing prefix matches nothing
			if err != nil {
				return nil //nolint:nilerr // an unreadable entry matches nothing
			}
			if d.IsDir() {
				if d.Name() == dirGit || len(seen) >= maxCacheFiles {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil //nolint:nilerr // outside root
			}
			if rel = filepath.ToSlash(rel); g.match(rel) {
				seen[rel] = true
			}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for rel := range seen {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

// literalPrefix is the leading directory of pattern that has no glob characters,
// so the walk starts below it ("rules/*.yaml" starts in rules).
func literalPrefix(pattern string) string {
	segs := strings.Split(strings.Trim(strings.ReplaceAll(pattern, `\`, "/"), "/"), "/")
	var keep []string
	for _, s := range segs[:max(len(segs)-1, 0)] {
		if strings.ContainsAny(s, "*?[{") {
			break
		}
		keep = append(keep, s)
	}
	return strings.Join(keep, "/")
}
