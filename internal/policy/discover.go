package policy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Environment variables that anchor and tune policy loading. They belong to the
// machine or the CI image, never to the repository.
const (
	// EnvPolicy names the policy (a file or an https URL, optionally @sha256:<hex>).
	EnvPolicy = "AI_RULEZ_POLICY"
	// EnvPolicyDigest pins the digest of the AI_RULEZ_POLICY policy.
	EnvPolicyDigest = "AI_RULEZ_POLICY_DIGEST"
	// EnvPolicyMaxStale is how long a cached policy may stand in for an unreachable URL.
	EnvPolicyMaxStale = "AI_RULEZ_POLICY_MAX_STALE"
	// EnvPolicyOffline ("1" or "true") makes policy loading use the cache only.
	EnvPolicyOffline = "AI_RULEZ_POLICY_OFFLINE"
)

// Origins of a layer, strongest anchor first.
const (
	OriginFlag    = "flag"
	OriginEnv     = "env"
	OriginManaged = "managed"
)

// DiscoverOptions says where to look for policy. A policy is never discovered
// from the repository being evaluated: every anchor is outside it.
type DiscoverOptions struct {
	// Flag is the value of --policy: a file, or an https URL, either with an
	// @sha256:<hex> suffix.
	Flag string
	// FlagDigest is --policy-digest: the digest the Flag policy must have.
	FlagDigest string
	// Env is the environment AI_RULEZ_POLICY is read from; nil is the real one.
	Env ambient.Env
	// GOOS selects the managed path; empty is runtime.GOOS.
	GOOS string
	// ManagedPaths replaces the managed locations (tests).
	ManagedPaths []string
	// Mode is --policy-mode: ModeEnforce (default, "") or ModeWarn.
	Mode string

	// HTTPClient fetches policy URLs; nil is a default client (tests inject one).
	// Its redirects are always limited to https on the same host.
	HTTPClient *http.Client
	// Offline makes URL policies come from the cache only (--policy-offline, or
	// AI_RULEZ_POLICY_OFFLINE).
	Offline bool
	// MaxStale is how long a cached copy may stand in for an unreachable URL: a
	// duration such as "7d" or "168h"; "0" allows none; "" is DefaultMaxStale
	// (or AI_RULEZ_POLICY_MAX_STALE).
	MaxStale string
	// TrustOnFirstUse (--policy-trust-tofu) accepts the digest of an unpinned URL
	// policy once and records it in the user cache. It needs Interactive.
	TrustOnFirstUse bool
	// Interactive says a person is at a terminal (set by the command).
	Interactive bool
	// Clock is the time source for cache ages; the zero value is the wall clock.
	Clock ambient.Clock
}

// Policy modes (--policy-mode).
const (
	ModeEnforce = "enforce"
	ModeWarn    = "warn"
)

// ValidMode reports whether mode is a policy mode ("" is the default, enforce).
func ValidMode(mode string) bool { return mode == "" || mode == ModeEnforce || mode == ModeWarn }

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

// cacheKey is what an Enforcer compares to decide the anchors changed.
func (o DiscoverOptions) cacheKey() string {
	return strings.Join([]string{
		o.Flag, o.FlagDigest, o.envPolicy(), ambient.Getenv(o.Env, EnvPolicyDigest), ambient.Getenv(o.Env, EnvPolicyMaxStale),
		ambient.Getenv(o.Env, EnvPolicyOffline), o.GOOS, o.Mode, o.MaxStale,
		fmt.Sprint(o.Offline, o.TrustOnFirstUse, o.Interactive), fmt.Sprintf("%p", o.HTTPClient),
	}, "\x00")
}

func (o DiscoverOptions) offline() bool {
	if o.Offline {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(ambient.Getenv(o.Env, EnvPolicyOffline))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// maxStale resolves how long a cached policy may be used: negative for never.
func (o DiscoverOptions) maxStale() (time.Duration, error) {
	raw := strings.TrimSpace(o.MaxStale)
	if raw == "" {
		raw = strings.TrimSpace(ambient.Getenv(o.Env, EnvPolicyMaxStale))
	}
	switch raw {
	case "":
		return DefaultMaxStale, nil
	case "0", "0s", "0h", "0d":
		return -1, nil
	}
	d, err := config.ParseApprovalMaxAge(raw)
	if err != nil {
		return 0, &ParseError{Path: "--policy-max-stale", Msg: err.Error()}
	}
	return d, nil
}

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

// loader carries the state of one Discover call.
type loader struct {
	opts     DiscoverOptions
	ctx      context.Context
	maxStale time.Duration
	layers   []Layer
	seen     map[string]bool
	loaded   int

	cacheOnce bool
	cache     *cache
}

// Discover loads every policy layer that applies, strongest anchor first: the
// --policy flag, AI_RULEZ_POLICY, then the managed location. A flag or
// variable that is set but cannot be loaded is an error (fail closed); an
// absent managed file is not, but a present one that is unusable is. A policy
// may be a file or an https URL pinned to a digest (see ParseRef).
func Discover(opts DiscoverOptions) ([]Layer, error) {
	stale, err := opts.maxStale()
	if err != nil {
		return nil, err
	}
	l := &loader{opts: opts, ctx: context.Background(), maxStale: stale, seen: map[string]bool{}}
	if p := strings.TrimSpace(opts.Flag); p != "" {
		if err := l.add(OriginFlag, p, opts.FlagDigest, true); err != nil {
			return nil, err
		}
	}
	if p := strings.TrimSpace(opts.envPolicy()); p != "" {
		if err := l.add(OriginEnv, p, ambient.Getenv(opts.Env, EnvPolicyDigest), true); err != nil {
			return nil, err
		}
	}
	for _, p := range managedPaths(opts) {
		if err := l.add(OriginManaged, p, "", false); err != nil {
			return nil, err
		}
	}
	return l.layers, nil
}

// add loads one anchor. required says a missing file is an error.
func (l *loader) add(origin, raw, pin string, required bool) error {
	ref, err := ParseRef(raw, pin)
	if err != nil {
		return err
	}
	key := ref.Location
	if !ref.Remote {
		abs, err := filepath.Abs(ref.Location)
		if err != nil {
			return &UnavailableError{Origin: origin, Path: ref.Location, Err: err}
		}
		ref.Location, key = abs, abs
	}
	if l.seen[key] {
		return nil
	}
	layer, err := l.loadOne(origin, ref)
	if err != nil {
		if !required && !ref.Remote && os.IsNotExist(unwrapPathError(err)) {
			return nil
		}
		return err
	}
	chain, _, err := l.expand(layer, ref, nil)
	if err != nil {
		return err
	}
	for _, c := range chain {
		if !l.seen[c.key] {
			l.seen[c.key] = true
			l.layers = append(l.layers, c)
		}
	}
	return nil
}

// loadOne loads one policy (file or URL) without its extends. The number of
// layers one load may pull in is bounded.
func (l *loader) loadOne(origin string, ref Ref) (Layer, error) {
	if l.loaded++; l.loaded > maxLayers {
		return Layer{}, &ParseError{Path: ref.Display(), Msg: fmt.Sprintf("more than %d policies are pulled in by extends", maxLayers)}
	}
	var layer Layer
	var err error
	if ref.Remote {
		layer, err = l.loadRemote(origin, ref)
	} else {
		layer, err = loadLayer(origin, ref)
	}
	layer.key = ref.identity()
	return layer, err
}

func unwrapPathError(err error) error {
	if u, ok := err.(*UnavailableError); ok { //nolint:errorlint // one level, our own type
		return u.Err
	}
	return err
}

// loadLayer reads and parses one policy file, checking the digest it is pinned to.
func loadLayer(origin string, ref Ref) (Layer, error) {
	data, err := readPolicyFile(ref.Location)
	if err != nil {
		return Layer{}, &UnavailableError{Origin: origin, Path: ref.Location, Err: err}
	}
	return buildLayer(origin, ref, data, "")
}

// buildLayer checks the pin and parses data into a layer.
func buildLayer(origin string, ref Ref, data []byte, note string) (Layer, error) {
	got := digest(data)
	if ref.Digest != "" && got != ref.Digest {
		return Layer{}, &DigestError{Path: ref.Display(), Want: ref.Digest, Got: got}
	}
	doc, err := parseDoc(ref.Display(), data)
	if err != nil {
		return Layer{}, err
	}
	return Layer{Origin: origin, Path: ref.Display(), Name: doc.name, Digest: got, Policy: doc.policy, Note: note, extendsRaw: doc.extends}, nil
}

// openCache opens the user cache once; nil when there is none (a policy cache
// is a convenience, so its absence only removes the fallback).
func (l *loader) openCache() *cache {
	if !l.cacheOnce {
		l.cacheOnce = true
		c, err := openCache(l.opts.Env)
		if err != nil {
			logger.Debug("policy cache unavailable", "error", err)
		} else {
			l.cache = c
		}
	}
	return l.cache
}

// loadRemote fetches an https policy, verifies its digest and keeps the last
// good copy. See ParseRef and DigestError for the pin rules.
func (l *loader) loadRemote(origin string, ref Ref) (Layer, error) {
	disp := ref.Display()
	unavailable := func(err error) (Layer, error) {
		return Layer{}, &UnavailableError{Origin: origin, Path: disp, Err: err}
	}
	first := false
	if ref.Digest == "" {
		if c := l.openCache(); c != nil {
			if d, ok := c.tofuGet(ref.Location); ok {
				ref.Digest = d
			}
		}
	}
	if ref.Digest == "" {
		switch {
		case !l.opts.TrustOnFirstUse:
			return Layer{}, &DigestError{Path: disp}
		case !l.opts.Interactive:
			return Layer{}, &ParseError{Path: disp, Msg: "--policy-trust-tofu needs an interactive terminal; in CI pin the digest with @sha256:<hex>"}
		case l.openCache() == nil:
			return unavailable(errors.New("--policy-trust-tofu needs the user cache to record the digest, and there is none"))
		case l.opts.offline():
			return unavailable(errors.New("offline: the first use of an unpinned URL needs the network"))
		}
		first = true
	}
	var data []byte
	var note string
	var err error
	if l.opts.offline() {
		data, note, err = l.fromCache(ref, errors.New("offline"))
	} else {
		data, err = fetch(l.ctx, l.opts.httpClient(), ref)
		var ne *networkError
		if errors.As(err, &ne) {
			data, note, err = l.fromCache(ref, ne)
		}
	}
	if err != nil {
		var pe *ParseError
		if errors.As(err, &pe) {
			return Layer{}, err
		}
		return unavailable(err)
	}
	if first {
		ref.Digest = digest(data)
		if err := l.openCache().tofuRecord(ref.Location, ref.Digest); err != nil {
			return unavailable(fmt.Errorf("cannot record the digest: %w", err))
		}
		logger.Warn("Recorded the digest of an unpinned policy URL on first use; verify it, then pin it with @sha256 in your configuration",
			"policy", disp, "digest", ref.Digest)
	}
	layer, err := buildLayer(origin, ref, data, note)
	if err != nil {
		return Layer{}, err
	}
	if note == "" {
		if c := l.openCache(); c != nil {
			if perr := c.put(ref.Location, ref.Digest, data, l.opts.Clock.Now()); perr != nil {
				logger.Debug("cannot cache the policy", "policy", disp, "error", perr)
			}
		}
	}
	return layer, nil
}

// fromCache stands in for an unreachable URL with the last good copy of the
// pinned digest, if it is younger than max_stale; otherwise AR742 (fail closed).
func (l *loader) fromCache(ref Ref, cause error) (data []byte, note string, err error) {
	c := l.openCache()
	if c == nil || ref.Digest == "" {
		return nil, "", fmt.Errorf("%w, and there is no cached copy", cause)
	}
	body, at, ok := c.get(ref.Location, ref.Digest)
	if !ok {
		return nil, "", fmt.Errorf("%w, and there is no cached copy of %s", cause, ref.Digest)
	}
	age := l.opts.Clock.Now().Sub(at)
	if l.maxStale < 0 || age > l.maxStale {
		return nil, "", fmt.Errorf("%w, and the cached copy fetched %s ago is older than max_stale (%s)", cause, age.Round(time.Minute), staleText(l.maxStale))
	}
	note = fmt.Sprintf("cached copy fetched %s (%v)", at.UTC().Format(time.RFC3339), cause)
	logger.Warn("Using the cached organization policy because the URL cannot be reached",
		"policy", ref.Display(), "fetched", at.UTC().Format(time.RFC3339), "reason", cause.Error())
	return body, note, nil
}

func staleText(d time.Duration) string {
	if d < 0 {
		return "0"
	}
	return formatAge(d)
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
