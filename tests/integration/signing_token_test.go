package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// actionsTokenServer imitates the GitHub Actions ID-token endpoint over TLS.
// It records the audience and the bearer token of the last request.
type actionsTokenServer struct {
	srv      *httptest.Server
	hits     atomic.Int32
	audience atomic.Value
	bearer   atomic.Value
}

func newActionsTokenServer(t *testing.T, status int, body string) *actionsTokenServer {
	t.Helper()
	a := &actionsTokenServer{}
	a.audience.Store("")
	a.bearer.Store("")
	a.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.hits.Add(1)
		a.audience.Store(r.URL.Query().Get("audience"))
		a.bearer.Store(r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
	}))
	t.Cleanup(a.srv.Close)
	// ResolveIDToken uses http.DefaultClient; trust this server's certificate for
	// the duration of the test only.
	prev := http.DefaultTransport
	http.DefaultTransport = a.srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = prev })
	return a
}

// TestResolveIDToken covers RV-TESTGAP-6: keyless signing's token
// acquisition (signing/token.go) had no test.
func TestResolveIDToken(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		env         func(url string) map[string]string
		tokenEnv    string
		want        string
		wantErr     string
		wantRequest bool
	}{
		{
			name: "a named variable wins and nothing is requested", tokenEnv: "MY_OIDC",
			env: func(url string) map[string]string {
				return map[string]string{"MY_OIDC": "tok-from-env", "ACTIONS_ID_TOKEN_REQUEST_URL": url, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			want: "tok-from-env",
		},
		{
			name: "a named variable that is empty is an error", tokenEnv: "MY_OIDC",
			env:     func(string) map[string]string { return map[string]string{} },
			wantErr: "MY_OIDC is empty or unset",
		},
		{
			name: "the Actions runtime answers with a token for the sigstore audience", status: http.StatusOK, body: `{"value":"tok-from-actions"}`,
			env: func(url string) map[string]string {
				return map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": url + "/token?api-version=2.0", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			want:        "tok-from-actions",
			wantRequest: true,
		},
		{
			name: "a plain-http request URL is refused before any request",
			env: func(url string) map[string]string {
				return map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": strings.Replace(url, "https://", "http://", 1), "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			wantErr: "not an https URL",
		},
		{
			name: "a non-200 answer is an error", status: http.StatusForbidden, body: `{"message":"no id-token permission"}`,
			env: func(url string) map[string]string {
				return map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": url, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			wantErr:     "403",
			wantRequest: true,
		},
		{
			name: "an empty token is an error", status: http.StatusOK, body: `{"value":"  "}`,
			env: func(url string) map[string]string {
				return map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": url, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			wantErr:     "returned no token",
			wantRequest: true,
		},
		{
			name: "an oversized answer is cut and rejected", status: http.StatusOK, body: `{"value":"` + strings.Repeat("a", 1<<17) + `"}`,
			env: func(url string) map[string]string {
				return map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": url, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt-secret-xyz"}
			},
			wantErr:     "returned no token",
			wantRequest: true,
		},
		{
			name:    "without a token source and not interactive there is nothing to sign with",
			env:     func(string) map[string]string { return map[string]string{} },
			wantErr: "no OIDC identity token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newActionsTokenServer(t, tt.status, tt.body)
			env := ambient.MapEnv{Vars: tt.env(srv.srv.URL), Home: t.TempDir()}

			// Act
			got, err := signing.ResolveIDToken(context.Background(), env, tt.tokenEnv, false)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "rt-secret-xyz", "the request token is never echoed")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			assert.Equal(t, tt.wantRequest, srv.hits.Load() > 0, "token endpoint contacted")
			if tt.wantRequest {
				assert.Equal(t, "sigstore", srv.audience.Load())
				assert.Equal(t, "Bearer rt-secret-xyz", srv.bearer.Load())
			}
		})
	}
}
