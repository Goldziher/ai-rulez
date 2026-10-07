package policy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

const (
	// maxRedirects bounds the redirects a policy URL may follow (same host only).
	maxRedirects = 5
	// fetchTimeout bounds one policy fetch.
	fetchTimeout = 20 * time.Second
	// DefaultMaxStale is how long a cached policy may stand in for an unreachable
	// URL.
	DefaultMaxStale = 7 * 24 * time.Hour
)

// digestPattern is the pinned digest form: sha256: and 64 hex digits.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// digestSuffix is a digest pinned onto a reference: path-or-url@sha256:<hex>.
var digestSuffix = regexp.MustCompile(`@(sha256:[0-9A-Fa-f]{64})$`)

// Ref is a parsed policy reference: a file path or an https URL, with the
// digest it is pinned to ("" when unpinned).
type Ref struct {
	// Location is the file path or URL, without the digest suffix.
	Location string
	// Digest is "sha256:<hex>" or "".
	Digest string
	// Remote is true for an https URL.
	Remote bool
}

// Display is the reference as shown to people: a URL without its query or
// fragment (which may carry a token), a path as is.
func (r Ref) Display() string {
	if !r.Remote {
		return r.Location
	}
	return redactURL(r.Location)
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(invalid URL)"
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String()
}

// DigestError is a policy whose content does not match the digest it must have,
// or a policy URL with no digest at all (AR741).
type DigestError struct {
	Path string
	// Hint replaces the default advice for a policy with no digest.
	Hint string
	// Want is the pinned digest, "" when the reference has none.
	Want string
	Got  string
}

func (e *DigestError) Error() string {
	if e.Want == "" && e.Hint != "" {
		return fmt.Sprintf("%s: policy %s has no digest; %s. A URL policy is never loaded unpinned, because whoever controls the URL would control the policy",
			lint.CodePolicyDigestMismatch, e.Path, e.Hint)
	}
	if e.Want == "" {
		return fmt.Sprintf("%s: policy %s has no digest; pin it with @sha256:<hex> (or --policy-digest), or record it once with --policy-trust-tofu in a terminal. "+
			"A URL policy is never loaded unpinned, because whoever controls the URL would control the policy",
			lint.CodePolicyDigestMismatch, e.Path)
	}
	return fmt.Sprintf("%s: policy %s has digest %s, but %s is pinned; review the change, then update the pin where it is configured",
		lint.CodePolicyDigestMismatch, e.Path, e.Got, e.Want)
}

// ParseRef reads one reference: path, https URL, either with an @sha256:<hex>
// suffix. pin, when not empty, is a digest given separately (--policy-digest);
// it must agree with a suffix. Anything with a scheme other than https is
// refused, URLs may not embed credentials, and a digest must be well formed.
func ParseRef(raw, pin string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	ref := Ref{Location: raw}
	if m := digestSuffix.FindStringSubmatchIndex(raw); m != nil {
		ref.Digest = strings.ToLower(raw[m[2]:m[3]])
		ref.Location = raw[:m[0]]
	}
	if pin = strings.ToLower(strings.TrimSpace(pin)); pin != "" {
		if !digestPattern.MatchString(pin) {
			return Ref{}, &ParseError{Path: redactURL(raw), Msg: fmt.Sprintf("%q is not a digest (use sha256: and 64 hex digits)", pin)}
		}
		if ref.Digest != "" && ref.Digest != pin {
			return Ref{}, &DigestError{Path: ref.Location, Want: pin, Got: ref.Digest}
		}
		ref.Digest = pin
	}
	if ref.Location == "" {
		return Ref{}, &ParseError{Path: raw, Msg: "empty policy reference"}
	}
	if !strings.Contains(ref.Location, "://") {
		return ref, nil
	}
	u, err := url.Parse(ref.Location)
	if err != nil {
		return Ref{}, &ParseError{Path: "(policy URL)", Msg: "not a valid URL"}
	}
	switch {
	case u.Scheme != "https":
		return Ref{}, &ParseError{Path: redactURL(ref.Location), Msg: fmt.Sprintf("a policy URL must be https, not %s: (file: and http: are refused)", u.Scheme)}
	case u.User != nil:
		return Ref{}, &ParseError{Path: redactURL(ref.Location), Msg: "a policy URL must not embed credentials"}
	case u.Host == "":
		return Ref{}, &ParseError{Path: redactURL(ref.Location), Msg: "a policy URL needs a host"}
	}
	ref.Remote = true
	return ref, nil
}

// networkError is a failure to reach a policy URL: the cached copy may stand in
// for it. Anything else (a bad digest, an oversized or invalid body) is final.
type networkError struct{ err error }

func (e *networkError) Error() string { return e.err.Error() }
func (e *networkError) Unwrap() error { return e.err }

// statusError is an HTTP status other than 200.
type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

// httpClient returns the client a fetch uses: the injected one (tests), else a
// default; either way redirects are limited to https on the same host.
func (o DiscoverOptions) httpClient() *http.Client {
	var c http.Client
	if o.HTTPClient != nil {
		c = *o.HTTPClient
	}
	if c.Timeout == 0 {
		c.Timeout = fetchTimeout
	}
	c.CheckRedirect = sameHostRedirect
	return &c
}

// sameHostRedirect follows a redirect only to https on the host that was asked:
// a policy host that redirects elsewhere is a host that no longer vouches for
// the content.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to %s: refused (https only)", req.URL.Scheme)
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("redirect to another host (%s): refused", req.URL.Hostname())
	}
	return nil
}

// fetch reads a policy URL: https, no credentials sent, status 200, at most
// maxPolicyBytes. A failure to get an answer is a *networkError; an answer that
// cannot be a policy is a *ParseError.
func fetch(ctx context.Context, client *http.Client, ref Ref) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.Location, http.NoBody)
	if err != nil {
		return nil, &ParseError{Path: ref.Display(), Msg: "not a valid URL"}
	}
	req.Header.Set("User-Agent", "ai-rulez-policy")
	req.Header.Set("Accept", "application/toml, text/plain, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, &networkError{err: errors.New(scrubURLError(err))}
	}
	defer resp.Body.Close() //nolint:errcheck // read-only
	if resp.StatusCode != http.StatusOK {
		return nil, &networkError{err: &statusError{code: resp.StatusCode}}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPolicyBytes+1))
	if err != nil {
		return nil, &networkError{err: errors.New(scrubURLError(err))}
	}
	if len(data) > maxPolicyBytes {
		return nil, &ParseError{Path: ref.Display(), Msg: fmt.Sprintf("larger than %d KiB", maxPolicyBytes>>10)}
	}
	return data, nil
}

// scrubURLError drops the URL (with any query) that net/http puts in its errors.
func scrubURLError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}
