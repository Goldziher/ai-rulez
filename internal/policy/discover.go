package policy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// EnvPolicy names the policy file for managed machines and CI images.
const EnvPolicy = "AI_RULEZ_POLICY"

// Origins of a layer, strongest anchor first.
const (
	OriginFlag    = "flag"
	OriginEnv     = "env"
	OriginManaged = "managed"
)

// DiscoverOptions says where to look for policy. A policy is never discovered
// from the repository being evaluated: every anchor is outside it.
type DiscoverOptions struct {
	// Flag is the value of --policy.
	Flag string
	// Env is the environment AI_RULEZ_POLICY is read from; nil is the real one.
	Env ambient.Env
	// GOOS selects the managed path; empty is runtime.GOOS.
	GOOS string
	// ManagedPaths replaces the managed locations (tests).
	ManagedPaths []string
}

// UnavailableError is a demanded policy that cannot be read (AR742).
type UnavailableError struct {
	Origin string
	Path   string
	Err    error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("%s: policy %s (from %s) cannot be used: %v; ai-rulez fails closed instead of running without it",
		lint.CodePolicyUnavailable, e.Path, e.Origin, e.Err)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

func (o DiscoverOptions) envPolicy() string { return ambient.Getenv(o.Env, EnvPolicy) }

// managedPaths returns the managed policy locations of a platform.
func managedPaths(opts DiscoverOptions) []string {
	if len(opts.ManagedPaths) > 0 {
		return opts.ManagedPaths
	}
	goos := opts.GOOS
	if goos == "" {
		goos = hostOS()
	}
	switch goos {
	case "darwin":
		return []string{"/Library/Application Support/ai-rulez/policy.toml"}
	case "windows":
		base := ambient.Getenv(opts.Env, "ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return []string{base + `\ai-rulez\policy.toml`}
	}
	return []string{"/etc/ai-rulez/policy.toml"}
}

// Discover loads every policy layer that applies, strongest anchor first: the
// --policy flag, AI_RULEZ_POLICY, then the managed location. A flag or
// variable that is set but cannot be loaded is an error (fail closed); an
// absent managed file is not, but a present one that is unusable is.
func Discover(opts DiscoverOptions) ([]Layer, error) {
	var layers []Layer
	seen := map[string]bool{}
	add := func(origin, path string, required bool) error {
		if strings.Contains(path, "://") {
			return &ParseError{Path: path, Msg: "policy URLs are not supported in this version; pass a file path"}
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return &UnavailableError{Origin: origin, Path: path, Err: err}
		}
		if seen[abs] {
			return nil
		}
		layer, err := loadLayer(origin, abs)
		if err != nil {
			if !required && os.IsNotExist(unwrapPathError(err)) {
				return nil
			}
			return err
		}
		seen[abs] = true
		layers = append(layers, layer)
		return nil
	}
	if p := strings.TrimSpace(opts.Flag); p != "" {
		if err := add(OriginFlag, p, true); err != nil {
			return nil, err
		}
	}
	if p := strings.TrimSpace(opts.envPolicy()); p != "" {
		if err := add(OriginEnv, p, true); err != nil {
			return nil, err
		}
	}
	for _, p := range managedPaths(opts) {
		if err := add(OriginManaged, p, false); err != nil {
			return nil, err
		}
	}
	return layers, nil
}

func unwrapPathError(err error) error {
	if u, ok := err.(*UnavailableError); ok { //nolint:errorlint // one level, our own type
		return u.Err
	}
	return err
}

// loadLayer reads and parses one policy file.
func loadLayer(origin, path string) (Layer, error) {
	data, err := readPolicyFile(path)
	if err != nil {
		return Layer{}, &UnavailableError{Origin: origin, Path: path, Err: err}
	}
	name, p, err := Parse(path, data)
	if err != nil {
		return Layer{}, err
	}
	return Layer{Origin: origin, Path: path, Name: name, Digest: digest(data), Policy: p}, nil
}

// readPolicyFile reads a regular file of at most maxPolicyBytes.
func readPolicyFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path is an operator-chosen anchor, never repository content
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPolicyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPolicyBytes {
		return nil, fmt.Errorf("larger than %d KiB", maxPolicyBytes>>10)
	}
	return data, nil
}
