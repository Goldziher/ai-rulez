package llm

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// Cache location and secret. They live in the user's directories, outside the
// repository, so a checkout (or a cache restored into one) cannot plant entries.
// The paths mirror config.CacheDir and config.UserConfigDir; this package cannot
// import internal/config (it imports this one).
const (
	cacheSecretFile  = "llm-cache.key"
	cacheSecretBytes = 32
)

// userHome returns the home directory or "".
func userHome(env ambient.Env) string {
	h, err := ambient.OrOS(env).UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// CacheDirFor returns the response cache directory of the project whose
// ai-rulez config directory is configDir: ~/.cache/ai-rulez/llm/<project hash>.
// opts.CacheDir overrides it. It returns "" when the cache cannot be placed
// (no config directory, or no home directory); there is no fallback to the
// shared temp directory.
func CacheDirFor(opts Options) string {
	if opts.CacheDir != "" {
		return opts.CacheDir
	}
	if opts.ConfigDir == "" {
		return ""
	}
	home := userHome(opts.Env)
	if home == "" {
		return ""
	}
	abs, err := filepath.Abs(opts.ConfigDir)
	if err != nil {
		abs = opts.ConfigDir
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return filepath.Join(home, ".cache", "ai-rulez", "llm", hex.EncodeToString(sum[:8]))
}

// secretPathFor returns the per-user cache secret path: opts.SecretPath, else
// $XDG_CONFIG_HOME/ai-rulez/llm-cache.key, else ~/.config/ai-rulez/llm-cache.key.
//
// XDG_CONFIG_HOME comes from the environment, so a repository whose .envrc or
// dev-shell sets it can relocate the secret into the checkout. The consequence
// is bounded (the key authenticates cached responses and committed eval results
// for that shell only, and is still created 0600 in a non-shared directory), but
// keep XDG_CONFIG_HOME out of untrusted environment files.
func secretPathFor(opts Options) string {
	if opts.SecretPath != "" {
		return opts.SecretPath
	}
	return UserSecretPathIn(opts.Env, cacheSecretFile)
}

// UserSecretPath returns the path of the per-user secret file name in the user
// config directory ($XDG_CONFIG_HOME/ai-rulez, else ~/.config/ai-rulez), or ""
// when there is no home directory.
func UserSecretPath(name string) string { return UserSecretPathIn(nil, name) }

// UserSecretPathIn is UserSecretPath reading the environment and home directory
// from env (nil: the real ones).
func UserSecretPathIn(env ambient.Env, name string) string {
	if xdg := ambient.Getenv(env, "XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "ai-rulez", name)
	}
	home := userHome(env)
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "ai-rulez", name)
}

// LoadSecretFile reads the 32-byte secret at path, creating it on first use. It
// is the helper behind the cache secret and the eval-result MAC key.
func LoadSecretFile(path string) ([]byte, error) { return loadOrCreateSecret(path) }

// secretDirTooOpen reports whether the secret directory is writable by group or
// others. Windows reports 0777 for every directory, so the mode says nothing
// there and the check is skipped (the file check at readSecret does the same).
func secretDirTooOpen(mode os.FileMode, goos string) bool {
	return goos != "windows" && mode.Perm()&0o022 != 0
}

// loadOrCreateSecret reads the secret, creating it (mode 0600, parent 0700) on
// first use. A missing, truncated, group/world-accessible or symlinked secret
// file is replaced or refused rather than trusted: a replaced secret simply
// invalidates every old entry, which then fail their MAC and are removed. The
// replacement is a rename of a private temp file, never remove-then-create, so a
// secret another process created in the meantime is re-validated and kept.
func loadOrCreateSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("no user directory for the cache secret")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		return nil, err
	} else if secretDirTooOpen(info.Mode(), runtime.GOOS) {
		return nil, errors.New("cache secret directory is writable by group or others")
	}
	existed := false
	if _, err := os.Lstat(path); err == nil {
		existed = true
		b, rerr := readSecret(path)
		if rerr == nil {
			return b, nil
		}
		if errors.Is(rerr, errSecretNotRegular) {
			return nil, rerr
		}
	}
	secret := make([]byte, cacheSecretBytes)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return nil, err
	}
	// Write a private temp file then link it into place, so a concurrent first
	// use never reads a half-written secret and exactly one creator wins.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".key-*") // mode 0600
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck,gosec // temp file
	_, werr := tmp.Write(secret)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return nil, werr
	}
	if existed {
		// An unusable file (empty, short, loose mode) is replaced atomically, after
		// a last look in case another process just fixed it.
		if b, rerr := readSecret(path); rerr == nil {
			return b, nil
		} else if errors.Is(rerr, errSecretNotRegular) {
			return nil, rerr
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			return nil, err
		}
		return secret, nil
	}
	if err := os.Link(tmp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			if b, rerr := readSecret(path); rerr == nil {
				return b, nil
			}
		}
		return nil, err
	}
	return secret, nil
}

var errSecretNotRegular = errors.New("secret is not a regular file")

// readSecret reads and validates the secret file: regular (a symlink swapped in
// after the caller's Lstat is caught by comparing the opened file with a fresh
// Lstat), mode 0600 on Unix, exactly cacheSecretBytes long.
func readSecret(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errSecretNotRegular
	}
	f, err := os.Open(path) //nolint:gosec // fixed per-user path, Lstat-checked above
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errSecretNotRegular
	}
	if runtime.GOOS != "windows" && opened.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("secret file is accessible by group or others")
	}
	b, err := io.ReadAll(io.LimitReader(f, cacheSecretBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) != cacheSecretBytes {
		return nil, errors.New("secret file has the wrong length")
	}
	return b, nil
}
