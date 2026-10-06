package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultRunnerTimeout bounds one runner invocation (one skill) when no timeout is set.
const DefaultRunnerTimeout = 30 * time.Minute

// maxStampBytes bounds how much of a runner program is hashed.
const maxStampBytes = 128 << 20

// commandProgramStamp stamps every file a shell command line names: the program
// and each argument that resolves to a regular file, by content hash. Editing
// run.py in `python run.py` must not reuse results the old script produced, so
// the first word alone is not enough. Words that are not files (flags, builtins,
// programs not found) contribute nothing.
func commandProgramStamp(command string) string {
	var stamps []string
	for i, word := range shellFields(command) {
		var path string
		if i == 0 {
			path = findProgram(word)
		} else {
			path = findFileArg(word)
		}
		if path == "" {
			continue
		}
		if h := fileHash(path); h != "" {
			stamps = append(stamps, filepath.Base(path)+"="+h)
		}
	}
	return strings.Join(stamps, ",")
}

// findFileArg resolves an argument to a regular file when it names one; a
// --flag=path argument is looked at after the "=".
func findFileArg(arg string) string {
	if _, value, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(arg, "-") {
		arg = value
	}
	if arg == "" || strings.HasPrefix(arg, "-") {
		return ""
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return ""
	}
	if info, statErr := os.Stat(abs); statErr == nil && info.Mode().IsRegular() {
		return abs
	}
	return ""
}

// shellFields splits a command line into words the way a POSIX shell would for
// the common cases: whitespace separates, single and double quotes group, and a
// backslash escapes the next character outside single quotes. It does not expand
// anything.
func shellFields(line string) []string {
	var (
		fields  []string
		cur     strings.Builder
		inWord  bool
		quote   rune
		escaped bool
	)
	flush := func() {
		if inWord {
			fields = append(fields, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	flush()
	return fields
}

// executableStamp identifies an executable by its resolved path, size and
// modification time (hashing a large binary on every run would be wasteful), or ""
// when it cannot be found.
func executableStamp(bin string) string {
	path := findProgram(bin)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return path + ":" + strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10)
}

// findProgram resolves name to a regular file: a path as given, else PATH.
func findProgram(name string) string {
	if name == "" {
		return ""
	}
	if strings.ContainsAny(name, `/\`) {
		if abs, err := filepath.Abs(name); err == nil {
			if info, statErr := os.Stat(abs); statErr == nil && info.Mode().IsRegular() {
				return abs
			}
		}
		return ""
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

// fileHash returns "sha256:<hex>" of the file, "" when unreadable. A file over
// maxStampBytes is stamped by size and modification time instead.
func fileHash(path string) string {
	f, err := os.Open(path) //nolint:gosec // the runner program the user chose
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	if info.Size() > maxStampBytes {
		return "stat:" + strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
