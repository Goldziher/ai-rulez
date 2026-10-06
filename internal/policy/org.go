package policy

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

const (
	// OriginOrg is the origin of a layer discovered from the organization's
	// <owner>/.github repository.
	OriginOrg = "org"
	// OrgPolicyFile is the file, at the root of <owner>/.github, that holds the
	// organization's policy.
	OrgPolicyFile = "ai-rulez-policy.toml"
	// defaultOrgRawBase serves raw files of GitHub repositories.
	defaultOrgRawBase = "https://raw.githubusercontent.com"
	// orgHost is the only forge organization discovery supports.
	orgHost = "github.com"
	// maxUserConfigBytes bounds the user config file read for [policy].
	maxUserConfigBytes = 1 << 20
)

// ownerPattern is a GitHub user or organization name.
var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

// UserSettings is the [policy] table of the user config file
// (~/.config/ai-rulez/config.toml). It lives outside any repository, so it can
// say where to look for a policy and whom to trust; a repository's own config has
// no such table and cannot set any of it.
type UserSettings struct {
	// Discover is "org" to look for the organization policy of the repository's
	// GitHub owner (see OrgRef); "" for no discovery.
	Discover string
	// Digests maps a lower-cased owner to the digest its organization policy is
	// pinned to.
	Digests map[string]string
	// RequireSignature makes an unsigned policy an error.
	RequireSignature bool
	// TrustedRoot is a Sigstore trusted root file; TLog the transparency-log mode.
	TrustedRoot, TLog string
	// Signers are the signers trusted to sign a policy.
	Signers []UserSigner
}

type userPolicyDoc struct {
	Policy *struct {
		Discover         string            `toml:"discover"`
		Digests          map[string]string `toml:"digests"`
		RequireSignature bool              `toml:"require_signature"`
		TrustedRoot      string            `toml:"trusted_root"`
		TLog             string            `toml:"tlog"`
		Signers          []struct {
			Identity       string `toml:"identity"`
			IdentityRegexp string `toml:"identity_regexp"`
			Issuer         string `toml:"issuer"`
			KeyFile        string `toml:"key_file"`
		} `toml:"signers"`
	} `toml:"policy"`
}

// LoadUserSettings reads the [policy] table of the user config. A missing file or
// table is empty settings; a file that cannot be parsed, or a bad value, is an
// error (fail closed: a broken file must not switch discovery off).
func LoadUserSettings(env ambient.Env) (UserSettings, error) {
	path := config.UserConfigFile(func(k string) string { return ambient.Getenv(env, k) })
	if path == "" {
		return UserSettings{}, nil
	}
	f, err := os.Open(path) //nolint:gosec // the user's own config file
	if errors.Is(err, os.ErrNotExist) {
		return UserSettings{}, nil
	}
	if err != nil {
		return UserSettings{}, &UnavailableError{Origin: "user config", Path: path, Err: err}
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxUserConfigBytes+1))
	if err != nil || len(data) > maxUserConfigBytes {
		return UserSettings{}, &UnavailableError{Origin: "user config", Path: path, Err: errors.New("unreadable or larger than 1 MiB")}
	}
	var doc userPolicyDoc
	if err := toml.Unmarshal(data, &doc); err != nil {
		return UserSettings{}, &ParseError{Path: path, Msg: "the user config is not valid TOML: " + err.Error()}
	}
	var us UserSettings
	if doc.Policy == nil {
		return us, nil
	}
	if us.Discover = strings.ToLower(strings.TrimSpace(doc.Policy.Discover)); us.Discover != "" && us.Discover != "org" {
		return UserSettings{}, &ParseError{Path: path, Msg: fmt.Sprintf("[policy] discover = %q is not supported (use \"org\")", doc.Policy.Discover)}
	}
	us.RequireSignature, us.TrustedRoot, us.TLog = doc.Policy.RequireSignature, strings.TrimSpace(doc.Policy.TrustedRoot), strings.TrimSpace(doc.Policy.TLog)
	for i, s := range doc.Policy.Signers {
		signer := UserSigner{Identity: strings.TrimSpace(s.Identity), IdentityRegexp: strings.TrimSpace(s.IdentityRegexp), Issuer: strings.TrimSpace(s.Issuer), KeyFile: strings.TrimSpace(s.KeyFile)}
		if err := checkUserSigner(signer); err != nil {
			return UserSettings{}, &ParseError{Path: path, Msg: fmt.Sprintf("[[policy.signers]] entry %d: %v", i+1, err)}
		}
		us.Signers = append(us.Signers, signer)
	}
	for owner, d := range doc.Policy.Digests {
		d = strings.ToLower(strings.TrimSpace(d))
		if !digestPattern.MatchString(d) || !ownerPattern.MatchString(owner) {
			return UserSettings{}, &ParseError{Path: path, Msg: fmt.Sprintf("[policy.digests] %q = %q: want an owner name and sha256:<hex>", owner, d)}
		}
		if us.Digests == nil {
			us.Digests = map[string]string{}
		}
		us.Digests[strings.ToLower(owner)] = d
	}
	return us, nil
}

// OrgWanted reports whether organization discovery is on: --discover-org, or
// [policy] discover = "org" in the user config.
func OrgWanted(opts DiscoverOptions) (bool, error) {
	if opts.DiscoverOrg {
		return true, nil
	}
	us, err := LoadUserSettings(opts.Env)
	return us.Discover == "org", err
}

// ownerFromRemote reads the GitHub owner out of a git remote URL
// (https://github.com/o/r.git, git@github.com:o/r.git, ssh://git@github.com/o/r).
func ownerFromRemote(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	var host, path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil {
			return "", errors.New("the origin remote is not a URL")
		}
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	case strings.Contains(remote, "@") && strings.Contains(remote, ":"):
		afterAt := remote[strings.Index(remote, "@")+1:]
		host, path, _ = strings.Cut(afterAt, ":")
	default:
		return "", errors.New("the origin remote is not a GitHub URL")
	}
	if !strings.EqualFold(host, orgHost) {
		return "", fmt.Errorf("the origin remote is on %q; organization discovery supports %s only", host, orgHost)
	}
	owner, _, found := strings.Cut(path, "/")
	if !found || !ownerPattern.MatchString(owner) {
		return "", errors.New("the origin remote has no valid GitHub owner")
	}
	return strings.ToLower(owner), nil
}

// originURL reads remote.origin.url of the repository at dir.
func originURL(dir string) (string, error) {
	out, err := gitutil.CommandNoContext(dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", errors.New("the repository has no origin remote")
	}
	return strings.TrimSpace(string(out)), nil
}

// orgRawBase is the base URL of raw GitHub files (a test server in tests).
func (o DiscoverOptions) orgRawBase() string {
	if o.OrgRawBase != "" {
		return strings.TrimRight(o.OrgRawBase, "/")
	}
	return defaultOrgRawBase
}

// OrgRef derives where the organization policy of the repository at opts.ProjectDir
// lives: the OrgPolicyFile at the root of <owner>/.github, from the owner of the
// origin remote. ok is false when it cannot be derived (no remote, not GitHub);
// that is an error only when discovery was demanded on the command line.
//
// The owner comes from the repository's own git configuration, so a repository
// can point its origin at an owner whose policy is looser. Discovery is therefore
// a convenience layer, never the anchor of an enforcement: it can only add
// restrictions, and the layers that bind (--policy, AI_RULEZ_POLICY, the managed
// path) do not depend on it.
func OrgRef(opts DiscoverOptions) (ref Ref, ok bool, err error) {
	if opts.ProjectDir == "" {
		return Ref{}, false, nil
	}
	remote := opts.RemoteURL
	if remote == nil {
		remote = originURL
	}
	raw, rerr := remote(opts.ProjectDir)
	var owner string
	if rerr == nil {
		owner, rerr = ownerFromRemote(raw)
	}
	if rerr != nil {
		if opts.DiscoverOrg {
			return Ref{}, false, &UnavailableError{Origin: OriginOrg, Path: opts.ProjectDir, Err: rerr}
		}
		logger.Warn("Organization policy discovery skipped", "project", opts.ProjectDir, "reason", rerr.Error())
		return Ref{}, false, nil
	}
	us, err := LoadUserSettings(opts.Env)
	if err != nil {
		return Ref{}, false, err
	}
	ref = Ref{Location: opts.orgRawBase() + "/" + owner + "/.github/HEAD/" + OrgPolicyFile, Remote: true, Digest: us.Digests[owner]}
	return ref, true, nil
}

// LoadOrg fetches the organization policy at ref (see OrgRef) with the rules of
// every policy URL: a digest is required (from [policy.digests] in the user
// config, a digest recorded by --policy-trust-tofu, never from the repository).
// An owner without a policy file (HTTP 404) has no organization policy: no layers.
func LoadOrg(opts DiscoverOptions, ref Ref) ([]Layer, error) {
	stale, err := opts.maxStale()
	if err != nil {
		return nil, err
	}
	l := &loader{opts: opts, ctx: opts.context(), maxStale: stale, seen: map[string]bool{}}
	layer, err := l.loadOne(OriginOrg, ref)
	if err != nil {
		var se *statusError
		if errors.As(err, &se) && se.code == 404 && !l.orgPinned(ref) {
			logger.Info("No organization policy published", "policy", ref.Display())
			return nil, nil
		}
		var de *DigestError
		if errors.As(err, &de) && de.Want == "" {
			de.Hint = "pin it in the user config ([policy.digests] <owner> = \"sha256:<hex>\") or record it once with --policy-trust-tofu in a terminal"
		}
		return nil, err
	}
	chain, _, err := l.expand(layer, ref, nil)
	return chain, err
}

// orgPinned reports whether the organization policy at ref is anchored: a pinned
// digest, a digest recorded by trust-on-first-use, or a required signature. A 404
// for an anchored policy is a withdrawn or hidden policy, not "no policy": it must
// fail closed (AR742), or whoever controls the answer could switch the org layer off.
func (l *loader) orgPinned(ref Ref) bool {
	if ref.Digest != "" {
		return true
	}
	if c := l.openCache(); c != nil {
		if _, ok := c.tofuGet(ref.Location); ok {
			return true
		}
	}
	pv, err := l.verifier()
	return err != nil || (pv != nil && pv.require)
}

// checkUserSigner validates one [[policy.signers]] entry: a certificate identity
// (exact or an anchored regexp) with its issuer, or a public key file.
func checkUserSigner(s UserSigner) error {
	byIdentity := s.Identity != "" || s.IdentityRegexp != ""
	switch {
	case byIdentity && s.KeyFile != "":
		return errors.New("trust an identity or a key_file, not both")
	case !byIdentity && s.KeyFile == "":
		return errors.New("needs identity, identity_regexp or key_file")
	case s.Identity != "" && s.IdentityRegexp != "":
		return errors.New("use identity or identity_regexp, not both")
	case byIdentity && s.Issuer == "":
		return errors.New("an identity entry needs an issuer")
	case s.KeyFile != "" && s.Issuer != "":
		return errors.New("a key_file entry has no issuer")
	}
	if s.IdentityRegexp != "" {
		if err := config.ValidateIdentityRegexp(s.IdentityRegexp); err != nil {
			return fmt.Errorf("AR722: %w", err)
		}
	}
	return nil
}
