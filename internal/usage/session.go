package usage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const saltFileName = "usage.salt"

// HashSession returns the salted session identifier written to the log: the first
// 16 hex digits of sha256(salt, NUL, id). It is stable for one session on one
// machine and cannot be mapped back to the harness's id without the salt.
func HashSession(salt, id string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + id))
	return hex.EncodeToString(sum[:8])
}

// hashedSession returns the salted hash of a session id, or "" when there is no
// id or no salt can be obtained. A raw id is never returned.
func hashedSession(id string, options RecordOptions, cwd string) string {
	if id == "" {
		return ""
	}
	salt := loadSalt(saltPathFor(options, cwd))
	if salt == "" {
		return ""
	}
	return HashSession(salt, id)
}

func saltPathFor(options RecordOptions, cwd string) string {
	switch {
	case options.SaltPath != "":
		return options.SaltPath
	case options.LogPath != "":
		return filepath.Join(filepath.Dir(options.LogPath), saltFileName)
	case cwd != "":
		return filepath.Join(cwd, ".ai-rulez", "local", saltFileName)
	}
	return ""
}

// loadSalt returns the salt from the environment or the salt file, creating the
// file (mode 0600) with 16 random bytes when missing or empty. It returns "" on
// failure.
//
// The salt is written to a private temporary file and hard-linked into place, so
// a concurrent reader sees either no file or the complete salt, never an empty
// one; a crash leaves no truncated file behind.
func loadSalt(path string) string {
	if salt := ambient.Getenv(nil, SaltEnv); salt != "" {
		return salt
	}
	if path == "" {
		return ""
	}
	if salt := readSalt(path); salt != "" {
		return salt
	}
	if err := safefs.EnsureParent(path); err != nil {
		return "" // a symlinked directory is never written through
	}
	// An empty file (from an older crash) is replaced; a missing one is created.
	if _, err := os.Stat(path); err == nil {
		if salt := readSalt(path); salt != "" {
			return salt // a concurrent hook just wrote it
		}
		_ = os.Remove(path) //nolint:errcheck // a concurrent writer may have replaced it; the read below settles it
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	salt := hex.EncodeToString(raw)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".usage-salt-*") // mode 0600
	if err != nil {
		return ""
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // the link below is the real file
	_, writeErr := tmp.WriteString(salt + "\n")
	if closeErr := tmp.Close(); writeErr != nil || closeErr != nil {
		return ""
	}
	// Link fails when a concurrent hook won the race; then use the winner's salt.
	if err := os.Link(tmp.Name(), path); err != nil {
		return readSalt(path)
	}
	return salt
}

// readSalt returns the trimmed contents of the salt file, tightening its mode to
// 0600 when it is looser. "" means missing, empty, unreadable or not a regular
// file: a symlinked salt (a repository can plant one) is never read or chmodded.
func readSalt(path string) string {
	data, err := safefs.ReadRegular(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// LoadSalt returns the session-hash salt for a salt file path (or the
// AI_RULEZ_USAGE_SALT variable), creating the file with mode 0600 when missing.
// It returns "" when no salt can be obtained.
func LoadSalt(path string) string { return loadSalt(path) }

// DefaultSaltPath is the salt file beside a usage log.
func DefaultSaltPath(logPath string) string {
	return filepath.Join(filepath.Dir(logPath), saltFileName)
}
