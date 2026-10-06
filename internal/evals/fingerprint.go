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

// commandProgramStamp names the program behind a shell command line by the
// content hash of its first word when that word is a file: editing the script a
// runner command points at must not reuse results the old script produced. A
// first word that is not a file (a shell builtin, a program not found) stamps as "".
func commandProgramStamp(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	first := strings.Trim(fields[0], `"'`)
	if path := findProgram(first); path != "" {
		return fileHash(path)
	}
	return ""
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
