package signing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestResolveIDToken(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]string
		tokenEnv    string
		interactive bool
		want        string
		wantErr     string
	}{
		{name: "named variable", vars: map[string]string{"MY_TOKEN": "tok"}, tokenEnv: "MY_TOKEN", want: "tok"},
		{
			name: "named variable wins over the Actions runtime", tokenEnv: "MY_TOKEN",
			vars: map[string]string{"MY_TOKEN": "tok", "ACTIONS_ID_TOKEN_REQUEST_URL": "https://x", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"},
			want: "tok",
		},
		{name: "named variable unset", vars: map[string]string{}, tokenEnv: "MY_TOKEN", wantErr: "MY_TOKEN is empty or unset"},
		{name: "named variable empty", vars: map[string]string{"MY_TOKEN": ""}, tokenEnv: "MY_TOKEN", wantErr: "MY_TOKEN is empty or unset"},
		{name: "nothing available, not interactive", vars: map[string]string{}, wantErr: "no OIDC identity token available"},
		{
			name: "Actions request URL needs only the URL and token pair",
			vars: map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "https://x"}, wantErr: "no OIDC identity token available",
		},
		{
			name:    "Actions request URL must be https",
			vars:    map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "http://169.254.169.254/token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"},
			wantErr: "not an https URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := ambient.MapEnv{Vars: tt.vars, Home: t.TempDir()}

			// Act
			got, err := ResolveIDToken(context.Background(), env, tt.tokenEnv, tt.interactive)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitHubActionsToken(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr string
	}{
		{name: "token returned", status: http.StatusOK, body: `{"value":"jwt"}`, want: "jwt"},
		{name: "endpoint refuses", status: http.StatusForbidden, body: `{}`, wantErr: "answered 403"},
		{name: "no value", status: http.StatusOK, body: `{"value":"  "}`, wantErr: "returned no token"},
		{name: "not JSON", status: http.StatusOK, body: `<html>`, wantErr: "returned no token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var gotAuth, gotAudience string
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth, gotAudience = r.Header.Get("Authorization"), r.URL.Query().Get("audience")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			// Act
			got, err := githubToken(context.Background(), srv.Client(), srv.URL+"/token?api-version=2.0", "request-token")

			// Assert
			assert.Equal(t, "Bearer request-token", gotAuth)
			assert.Equal(t, oidcAudience, gotAudience)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "request-token")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitHubActionsTokenUnreachable(t *testing.T) {
	// Arrange
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	client := srv.Client()
	url := srv.URL
	srv.Close()

	// Act
	_, err := githubToken(context.Background(), client, url, "request-token")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request the GitHub Actions identity token")
}
