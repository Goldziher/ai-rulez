package policy

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzParseRef: whatever the input, a reference that parses as remote is https
// with a host and no credentials, a digest is well formed, and no error repeats
// a query string or password.
func FuzzParseRef(f *testing.F) {
	hex := strings.Repeat("a1", 32)
	for _, seed := range []string{
		"/etc/ai-rulez/policy.toml", "https://p.example.org/p.toml@sha256:" + hex, "https://u:pw@p.example.org/p.toml",
		"http://p.example.org/p.toml?token=SECRETQ", "file:///etc/p.toml", "https:///x", "https://p.example.org/p.toml?token=SECRETQ#f",
		"https://[::1]/p.toml", "https://p.example.org:443/%zz", "ftp://x@sha256:" + hex, "@sha256:" + hex, "",
	} {
		f.Add(seed, "")
		f.Add(seed, "sha256:"+hex)
	}
	f.Fuzz(func(t *testing.T, raw, pin string) {
		ref, err := ParseRef(raw, pin)
		if err != nil {
			if u, perr := url.Parse(digestSuffix.ReplaceAllString(raw, "")); perr == nil && strings.Contains(raw, "://") {
				secrets := []string{u.RawQuery, u.Fragment}
				if u.User != nil {
					pw, _ := u.User.Password()
					secrets = append(secrets, pw, u.User.Username())
				}
				for _, secret := range secrets {
					if len(secret) > 7 && !strings.Contains(u.Scheme+u.Host+u.Path+pin, secret) && strings.Contains(err.Error(), secret) {
						t.Fatalf("the error repeats %q from %q: %v", secret, raw, err)
					}
				}
			}
			return
		}
		if ref.Digest != "" && !digestPattern.MatchString(ref.Digest) {
			t.Fatalf("malformed digest accepted: %q", ref.Digest)
		}
		if !ref.Remote {
			if strings.Contains(ref.Location, "://") {
				t.Fatalf("a location with a scheme parsed as a file: %q", ref.Location)
			}
			return
		}
		u, perr := url.Parse(ref.Location)
		if perr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			t.Fatalf("an unsafe URL was accepted: %q", ref.Location)
		}
		if strings.ContainsAny(ref.Display(), "?#") {
			t.Fatalf("display leaks the query or fragment: %q", ref.Display())
		}
	})
}
