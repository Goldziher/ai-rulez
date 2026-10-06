package config

import (
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// ReviewProviderDefaultHost is the allowed_hosts entry that names the provider's own endpoint,
// used when no base_url is configured.
const ReviewProviderDefaultHost = "provider-default"

// ReviewResolution is what only user scope may say about `ai-rulez review`: the hosts a judged
// run may send to, and spend ceilings the repository cannot lower or raise for you.
type ReviewResolution struct {
	// AllowedHosts is the user-scope [review] allowed_hosts (or AI_RULEZ_REVIEW_ALLOWED_HOSTS).
	AllowedHosts []string
	// MaxCostUSD and MaxCalls are the user-scope ceilings (0 = unset).
	MaxCostUSD float64
	MaxCalls   int
	// IgnoredRepoKeys names the user-scope-only keys a repository config set; they have no effect.
	IgnoredRepoKeys []string
	// UserFile is the user config path that was read ("" when none exists).
	UserFile string
}

// loadUserReview reads the [review] table of the user config file; a missing file or table is not an error.
func loadUserReview(path string) (*ReviewConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own config file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("path", path).Wrapf(err, "read user config")
	}
	var doc struct {
		Review *ReviewConfig `toml:"review"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Wrapf(err, "parse user config")
	}
	return doc.Review, nil
}

// ResolveReview reads the user-scope part of [review]. A repository config (and its local
// overlay) cannot set allowed_hosts: it would let a hostile repository extend the list of
// places a prompt may go, so the key is ignored and reported. getenv may be nil (os.Getenv).
func (c *Config) ResolveReview(getenv func(string) string) (ReviewResolution, error) {
	if getenv == nil {
		getenv = func(name string) string { return ambient.Getenv(nil, name) }
	}
	var out ReviewResolution
	if c != nil && c.Review != nil && len(c.Review.AllowedHosts) > 0 {
		out.IgnoredRepoKeys = []string{"allowed_hosts"}
	}
	path := UserConfigFile(getenv)
	user, err := loadUserReview(path)
	if err != nil {
		return out, err
	}
	if user != nil {
		out.UserFile = path
		out.AllowedHosts = user.AllowedHosts
		out.MaxCostUSD, out.MaxCalls = user.MaxCostUSD, user.MaxCalls
		if problems := (&ReviewConfig{AllowedHosts: user.AllowedHosts}).Validate(); len(problems) > 0 {
			return out, oops.Errorf("user config [review]: %s", strings.Join(problems, "; "))
		}
	}
	if v := strings.TrimSpace(getenv("AI_RULEZ_REVIEW_ALLOWED_HOSTS")); v != "" {
		out.AllowedHosts = nil
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				out.AllowedHosts = append(out.AllowedHosts, h)
			}
		}
	}
	return out, nil
}

// Caps resolves the spend ceilings of a judged run. A flag wins. Otherwise a user-scope value
// wins, and a repository value can only lower the default: a hostile repository must not be able
// to raise what a run may spend on your credentials.
func (r ReviewResolution) Caps(repo *ReviewConfig, defCost float64, defCalls int) (cost float64, calls int) {
	cost, calls = defCost, defCalls
	switch {
	case r.MaxCostUSD > 0:
		cost = r.MaxCostUSD
	case repo != nil && repo.MaxCostUSD > 0 && repo.MaxCostUSD < cost:
		cost = repo.MaxCostUSD
	}
	switch {
	case r.MaxCalls > 0:
		calls = r.MaxCalls
	case repo != nil && repo.MaxCalls > 0 && repo.MaxCalls < calls:
		calls = repo.MaxCalls
	}
	return cost, calls
}

// HostAllowed reports whether a judged run may send to host (host or host:port as written in
// base_url; "" means the provider's own endpoint). An empty allow-list allows any host.
func (r ReviewResolution) HostAllowed(host string) bool {
	if len(r.AllowedHosts) == 0 {
		return true
	}
	if host == "" {
		host = ReviewProviderDefaultHost
	}
	for _, h := range r.AllowedHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}
