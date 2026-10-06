package includes

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// TokenHostsEnv names the environment variable that lists, comma separated, the
// hosts the git access token (AI_RULEZ_GIT_TOKEN, -T) may be sent to. It is read
// from the environment only: a project file must never widen where a credential
// goes. Unset or empty, the token is sent to github.com alone.
const TokenHostsEnv = "AI_RULEZ_GIT_TOKEN_HOSTS"

// defaultTokenHosts is the allowlist when TokenHostsEnv is unset.
var defaultTokenHosts = []string{"github.com"}

var warnedHosts sync.Map

// tokenHosts returns the lower-cased hosts the token may be sent to.
func tokenHosts(host ambient.Host) []string {
	raw := strings.TrimSpace(host.GetEnv(TokenHostsEnv))
	if raw == "" {
		return defaultTokenHosts
	}
	var hosts []string
	for _, h := range strings.Split(raw, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return defaultTokenHosts
	}
	return hosts
}

// TokenHosts returns the hosts credentials may be sent to (TokenHostsEnv, else
// github.com). The forge client (internal/forge) uses the same allowlist.
func TokenHosts(host ambient.Host) []string { return append([]string(nil), tokenHosts(host)...) }

// tokenAllowedFor reports whether token may be sent to the https repository at
// rawURL, and the origin ("https://host[:port]") to scope the header to. Plain
// http, ssh, file and scp-style remotes never receive it.
func tokenAllowedFor(h ambient.Host, rawURL string) (origin string, ok bool) {
	u, err := url.Parse(strings.TrimPrefix(strings.TrimSpace(rawURL), "git+"))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range tokenHosts(h) {
		if host == allowed {
			return "https://" + strings.ToLower(u.Host), true
		}
	}
	return "", false
}

// withAuth returns env extended with a per-command git credential: an
// `http.<origin>/.extraHeader` carrying the token, scoped to the origin of
// repoURL. The token is never part of a URL, so it reaches neither argv nor
// the cloned repository's .git/config. With no token, or a host the user did not
// allow (see TokenHostsEnv), env is returned unchanged.
func withAuth(ctx context.Context, env []string, repoURL, token string) []string {
	if token == "" {
		return env
	}
	origin, ok := tokenAllowedFor(ambient.FromContext(ctx), repoURL)
	if !ok {
		if strings.HasPrefix(strings.ToLower(strings.TrimPrefix(repoURL, "git+")), "https://") {
			warnTokenWithheld(repoURL)
		}
		return env
	}
	count := 0
	for _, kv := range env {
		if v, found := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); found {
			if n, err := strconv.Atoi(v); err == nil && n > count {
				count = n
			}
		}
	}
	cred := base64.StdEncoding.EncodeToString([]byte(token + ":x-oauth-basic"))
	n := strconv.Itoa(count)
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GIT_CONFIG_COUNT=") {
			out = append(out, kv)
		}
	}
	return append(out,
		"GIT_CONFIG_COUNT="+strconv.Itoa(count+1),
		"GIT_CONFIG_KEY_"+n+"=http."+origin+"/.extraHeader",
		"GIT_CONFIG_VALUE_"+n+"=Authorization: Basic "+cred,
	)
}

func warnTokenWithheld(repoURL string) {
	u, err := url.Parse(strings.TrimPrefix(repoURL, "git+"))
	if err != nil {
		return
	}
	host := strings.ToLower(u.Hostname())
	if _, seen := warnedHosts.LoadOrStore(host, true); seen {
		return
	}
	logger.Warn("git access token not sent: host is not in the token allowlist", "host", host, "allow_with", TokenHostsEnv+"="+host)
}

// scrubLegacyCredentials rewrites the origin of a cache cloned by an earlier
// version, which embedded the token in the remote URL and so left it in
// .git/config. A cache whose config carries no credential is left untouched.
func scrubLegacyCredentials(ctx context.Context, cacheDir, cleanURL string) {
	cfg, err := os.ReadFile(filepath.Join(cacheDir, ".git", "config")) //nolint:gosec // our own cache directory
	if err != nil || !userinfoRe.Match(cfg) {
		return
	}
	res := gitRun(ctx, cacheDir, gitEnvFor(ctx), "remote", "set-url", "origin", cleanURL)
	if out, err := combined(res), gitutil.ResultErr(res); err != nil {
		logger.Warn("could not scrub a credential from the include cache; delete it", "cache_dir", cacheDir, "error", err, "output", RedactURL(string(out)))
	}
}
