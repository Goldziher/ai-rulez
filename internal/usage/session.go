package usage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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
// file (mode 0600) with 16 random bytes when missing. It returns "" on failure.
func loadSalt(path string) string {
	if salt := os.Getenv(SaltEnv); salt != "" {
		return salt
	}
	if path == "" {
		return ""
	}
	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // the machine-local salt file
		return strings.TrimSpace(string(data))
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	salt := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return ""
	}
	// O_EXCL: when two hooks race, one wins and the other reads the winner's salt.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the machine-local salt file
	if err != nil {
		if data, readErr := os.ReadFile(path); readErr == nil { //nolint:gosec // the machine-local salt file
			return strings.TrimSpace(string(data))
		}
		return ""
	}
	_, writeErr := file.WriteString(salt + "\n")
	if closeErr := file.Close(); writeErr != nil || closeErr != nil {
		return ""
	}
	return salt
}
