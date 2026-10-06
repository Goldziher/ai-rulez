package llm

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
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
func userHome() string {
	h, err := os.UserHomeDir()
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
	home := userHome()
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
func secretPathFor(opts Options) string {
	if opts.SecretPath != "" {
		return opts.SecretPath
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "ai-rulez", cacheSecretFile)
	}
	home := userHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "ai-rulez", cacheSecretFile)
}

// loadOrCreateSecret reads the cache secret, creating it (mode 0600, parent
// 0700) on first use. A missing, truncated or symlinked secret file is replaced
// or refused rather than trusted: a replaced secret simply invalidates every old
// entry, which then fail their MAC and are removed.
func loadOrCreateSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("no user directory for the cache secret")
	}
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() {
			return nil, errors.New("cache secret is not a regular file")
		}
		if b, rerr := readSecret(path); rerr == nil && len(b) == cacheSecretBytes {
			return b, nil
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
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
	if err := os.Link(tmp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			if b, rerr := readSecret(path); rerr == nil && len(b) == cacheSecretBytes {
				return b, nil
			}
		}
		return nil, err
	}
	return secret, nil
}

func readSecret(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // fixed per-user path
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, cacheSecretBytes+1))
}
