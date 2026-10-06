package signing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore/pkg/oauthflow"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

const (
	// DefaultOIDCIssuer is the public-good Sigstore OAuth issuer, used by the
	// interactive flow.
	DefaultOIDCIssuer = "https://oauth2.sigstore.dev/auth"
	oidcClientID      = "sigstore"
	oidcAudience      = "sigstore"
	maxTokenBody      = 1 << 16
	tokenTimeout      = 30 * time.Second
)

// ResolveIDToken finds the OIDC token keyless signing exchanges for a
// certificate, in order: the environment variable tokenEnv (never a flag, so the
// token cannot land in shell history or a process list), the ambient GitHub
// Actions token (needs `permissions: id-token: write`), then, when interactive,
// the Sigstore browser flow. The token is returned, never logged.
func ResolveIDToken(ctx context.Context, env ambient.Env, tokenEnv string, interactive bool) (string, error) {
	if tokenEnv != "" {
		tok := ambient.Getenv(env, tokenEnv)
		if tok == "" {
			return "", oops.Errorf("the environment variable %s is empty or unset", tokenEnv)
		}
		return tok, nil
	}
	reqURL, reqToken := ambient.Getenv(env, "ACTIONS_ID_TOKEN_REQUEST_URL"), ambient.Getenv(env, "ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL != "" && reqToken != "" {
		return githubToken(ctx, reqURL, reqToken)
	}
	if !interactive {
		return "", oops.Hint("set --identity-token-env to a variable holding an OIDC token, or run in GitHub Actions with `permissions: id-token: write`").
			Errorf("no OIDC identity token available for keyless signing")
	}
	tok, err := oauthflow.OIDConnect(DefaultOIDCIssuer, oidcClientID, "", "", oauthflow.DefaultIDTokenGetter)
	if err != nil {
		return "", oops.Wrapf(err, "obtain an OIDC token in the browser")
	}
	return tok.RawString, nil
}

// githubToken asks the Actions runtime for an ID token with the sigstore audience.
func githubToken(ctx context.Context, requestURL, requestToken string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", oops.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL is not an https URL")
	}
	q := u.Query()
	q.Set("audience", oidcAudience)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", oops.Wrapf(err, "build the token request")
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // the URL comes from the Actions runtime environment
	if err != nil {
		return "", oops.Wrapf(err, "request the GitHub Actions identity token")
	}
	defer resp.Body.Close() //nolint:errcheck // read-only
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenBody))
	if err != nil {
		return "", oops.Wrapf(err, "read the GitHub Actions identity token")
	}
	if resp.StatusCode != http.StatusOK {
		return "", oops.Errorf("the GitHub Actions token endpoint answered %s", resp.Status)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil || strings.TrimSpace(out.Value) == "" {
		return "", oops.Errorf("the GitHub Actions token endpoint returned no token")
	}
	return out.Value, nil
}
