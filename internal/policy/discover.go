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

	// DiscoverOrg is --discover-org: also look for the organization policy of the
	// repository's GitHub owner (see OrgRef). [policy] discover = "org" in the user
	// config does the same.
	DiscoverOrg bool
	// ProjectDir is the repository being evaluated, for organization discovery. It
	// only selects the owner; a policy is never read from it.
	ProjectDir string
	// RemoteURL reads the origin remote of a repository (tests); nil runs git.
	RemoteURL func(dir string) (string, error)
	// OrgRawBase replaces https://raw.githubusercontent.com (tests).
	OrgRawBase string

	// Signature configures the check of a policy's signature (see SignatureOptions).
	Signature SignatureOptions
	// Log receives the discovery's reports; nil is the CLI's logger.
	Log logger.Logger
}

// logger is where discovery reports.
func (o DiscoverOptions) logger() logger.Logger { return logger.Or(o.Log) }

// context is the context of a load.
func (o DiscoverOptions) context() context.Context { return context.Background() }

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
		fmt.Sprint(o.Offline, o.TrustOnFirstUse, o.Interactive, o.DiscoverOrg), fmt.Sprintf("%p", o.HTTPClient), o.OrgRawBase, fmt.Sprint(o.Signature),
		ambient.Getenv(o.Env, EnvRequireSigned), ambient.Getenv(o.Env, EnvSignerIdentity), ambient.Getenv(o.Env, EnvSignerIssuer),
		ambient.Getenv(o.Env, EnvSignerKey), ambient.Getenv(o.Env, EnvTrustedRoot),
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
		if managedRoot != "" {
			return []string{filepath.Join(managedRoot, "ai-rulez", "policy.toml")}
		}
		goos = hostOS()
	}
	switch goos {
	case "darwin":
		return []string{"/Library/Application Support/ai-rulez/policy.toml"}
	case "windows":
		return []string{programData(opts.Env) + `\ai-rulez\policy.toml`}
	}
	return []string{"/etc/ai-rulez/policy.toml"}
}

// defaultProgramData is where Windows keeps machine-wide application data.
const defaultProgramData = `C:\ProgramData`

// programData is the Windows machine-wide data directory: %ProgramData%, which the
// system sets, when it is an absolute path (a drive path or a UNC share), else the
// default. A relative value is refused because it would resolve against the
// directory of the repository being evaluated.
func programData(env ambient.Env) string {
	base := strings.TrimRight(strings.TrimSpace(ambient.Getenv(env, "ProgramData")), `\/`)
	if len(base) > 3 && base[1] == ':' && isLetter(base[0]) && (base[2] == '\\' || base[2] == '/') {
		return base
	}
	if strings.HasPrefix(base, `\\`) && len(base) > 2 {
		return base
	}
	return defaultProgramData
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// loader carries the state of one Discover call.
type loader struct {
	opts     DiscoverOptions
	ctx      context.Context
	maxStale time.Duration
	layers   []Layer
	seen     map[string]bool
	loaded   int

	verifierDone bool
	pv           *verifier
	pvErr        error

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
	l := &loader{opts: opts, ctx: opts.context(), maxStale: stale, seen: map[string]bool{}}
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
		layer, err = l.loadLayer(origin, ref)
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

// loadLayer reads and parses one policy file, checking the digest it is pinned to
// and, when a trust set is configured, the signature next to it.
func (l *loader) loadLayer(origin string, ref Ref) (Layer, error) {
	data, err := readPolicyFile(ref.Location)
	if err != nil {
		return Layer{}, &UnavailableError{Origin: origin, Path: ref.Location, Err: err}
	}
	bundle, err := readSidecarFile(ref.Location)
	if err != nil {
		return Layer{}, err
	}
	signer, err := l.checkSignature(ref, data, bundle)
	if err != nil {
		return Layer{}, err
	}
	layer, err := buildLayer(origin, ref, data, "")
	layer.Signer = signer
	return layer, err
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
			l.opts.logger().Debug("policy cache unavailable", "error", err)
		} else {
			l.cache = c
		}
	}
	return l.cache
}

// remoteMode says how the content of a policy URL is vouched for.
type remoteMode int

const (
	// modePinned: the reference carries (or the user cache recorded) a digest.
	modePinned remoteMode = iota
	// modeSigned: no digest, but a trusted signer's signature vouches for it.
	modeSigned
	// modeFirstUse: no digest; trust-on-first-use records the one fetched.
	modeFirstUse
)

// loadRemote fetches an https policy, verifies its digest (or its signature) and
// keeps the last good copy. See ParseRef and DigestError for the pin rules.
func (l *loader) loadRemote(origin string, ref Ref) (Layer, error) {
	disp := ref.Display()
	unavailable := func(err error) (Layer, error) {
		return Layer{}, &UnavailableError{Origin: origin, Path: disp, Err: err}
	}
	pv, err := l.verifier()
	if err != nil {
		return Layer{}, err
	}
	mode := modePinned
	if ref.Digest == "" {
		if c := l.openCache(); c != nil {
			if d, ok := c.tofuGet(ref.Location); ok {
				ref.Digest = d
			}
		}
	}
	if ref.Digest == "" {
		switch {
		case pv != nil:
			mode = modeSigned
		case !l.opts.TrustOnFirstUse:
			return Layer{}, &DigestError{Path: disp}
		case !l.opts.Interactive:
			return Layer{}, &ParseError{Path: disp, Msg: "--policy-trust-tofu needs an interactive terminal; in CI pin the digest with @sha256:<hex>"}
		case l.openCache() == nil:
			return unavailable(errors.New("--policy-trust-tofu needs the user cache to record the digest, and there is none"))
		case l.opts.offline():
			return unavailable(errors.New("offline: the first use of an unpinned URL needs the network"))
		default:
			mode = modeFirstUse
		}
	}
	data, bundle, note, err := l.obtain(ref, pv, mode)
	if err != nil {
		var pe *ParseError
		var se *SignatureError
		if errors.As(err, &pe) || errors.As(err, &se) {
			return Layer{}, err
		}
		return unavailable(err)
	}
	signer, err := l.checkSignature(ref, data, bundle)
	if err != nil {
		return Layer{}, err
	}
	if mode == modeSigned && signer == "" {
		return Layer{}, &DigestError{Path: disp}
	}
	if mode == modeFirstUse {
		ref.Digest = digest(data)
		if err := l.openCache().tofuRecord(ref.Location, ref.Digest); err != nil {
			return unavailable(fmt.Errorf("cannot record the digest: %w", err))
		}
		l.opts.logger().Warn("Recorded the digest of an unpinned policy URL on first use; verify it, then pin it with @sha256 in your configuration",
			"policy", disp, "digest", ref.Digest)
	}
	layer, err := buildLayer(origin, ref, data, note)
	if err != nil {
		return Layer{}, err
	}
	layer.Signer = signer
	if note == "" {
		if c := l.openCache(); c != nil {
			if perr := c.put(ref.Location, ref.Digest, data, bundle, l.opts.Clock.Now()); perr != nil {
				l.opts.logger().Debug("cannot cache the policy", "policy", disp, "error", perr)
			}
		}
	}
	return layer, nil
}

// obtain gets the body of a policy URL, and its signature when a trust set is
// configured: from the network, or from the cache when the URL cannot be reached
// (or --policy-offline).
func (l *loader) obtain(ref Ref, pv *verifier, mode remoteMode) (data, bundle []byte, note string, err error) {
	if l.opts.offline() {
		return l.fromCache(ref, mode, errors.New("offline"))
	}
	client := l.opts.httpClient()
	data, err = fetch(l.ctx, client, ref)
	var ne *networkError
	if errors.As(err, &ne) {
		return l.fromCache(ref, mode, ne)
	}
	if err != nil || pv == nil {
		return data, nil, "", err
	}
	bundle, err = fetchSidecar(l.ctx, client, ref)
	if errors.As(err, &ne) {
		if pv.require || mode == modeSigned {
			return l.fromCache(ref, mode, ne) // a signature is needed, so a stale bundle may stand in
		}
		l.opts.logger().Warn("The policy signature could not be fetched; the digest pin still vouches for the policy", "policy", ref.Display(), "reason", ne.Error())
		return data, nil, "", nil
	}
	return data, bundle, "", err
}

// fromCache stands in for an unreachable URL with the last good copy of the
// pinned digest (or of the signed policy), if it is younger than max_stale;
// otherwise AR742 (fail closed).
func (l *loader) fromCache(ref Ref, mode remoteMode, cause error) (data, bundle []byte, note string, err error) {
	c := l.openCache()
	if c == nil || (ref.Digest == "" && mode != modeSigned) {
		return nil, nil, "", fmt.Errorf("%w, and there is no cached copy", cause)
	}
	body, bun, at, ok := c.get(ref.Location, ref.Digest)
	if !ok {
		return nil, nil, "", fmt.Errorf("%v, and there is no cached copy", cause)
	}
	// Past the cache lookup the cause is flattened (%v): an expired or offline
	// answer must not keep a statusError that LoadOrg would read as "no policy".
	age := l.opts.Clock.Now().Sub(at)
	if l.maxStale < 0 || age > l.maxStale {
		return nil, nil, "", fmt.Errorf("%v, and the cached copy fetched %s ago is older than max_stale (%s)", cause, age.Round(time.Minute), staleText(l.maxStale))
	}
	note = fmt.Sprintf("cached copy fetched %s (%v)", at.UTC().Format(time.RFC3339), cause)
	l.opts.logger().Warn("Using the cached organization policy because the URL cannot be reached",
		"policy", ref.Display(), "fetched", at.UTC().Format(time.RFC3339), "reason", cause.Error())
	return body, bun, note, nil
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
