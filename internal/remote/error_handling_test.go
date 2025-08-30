package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ErrorHandling(t *testing.T) {
	t.Run("ssrf_protection_errors", func(t *testing.T) {
		client := NewClient(nil)
		ctx := context.Background()

		testCases := []struct {
			name        string
			url         string
			expectError string
		}{
			{
				name:        "localhost_blocked",
				url:         "http://localhost:8080/config.yaml",
				expectError: "URL blocked for security reasons",
			},
			{
				name:        "private_ip_blocked",
				url:         "http://192.168.1.1/config.yaml",
				expectError: "URL blocked for security reasons",
			},
			{
				name:        "loopback_blocked",
				url:         "http://127.0.0.1/config.yaml",
				expectError: "URL blocked for security reasons",
			},
			{
				name:        "metadata_service_blocked",
				url:         "http://169.254.169.254/latest/meta-data/",
				expectError: "URL blocked for security reasons",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := client.Fetch(ctx, tc.url)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectError)

				var richErr *errors.RichError
				require.ErrorAs(t, err, &richErr)
				assert.Equal(t, errors.ErrorTypeRemoteSSRF, richErr.Type)
				assert.Equal(t, tc.url, richErr.Path)
				assert.Contains(t, richErr.Context, "url")
				assert.Contains(t, richErr.Context, "block_reason")
				assert.Greater(t, len(richErr.Suggestions), 0)
			})
		}
	})

	t.Run("http_status_errors", func(t *testing.T) {
		testCases := []struct {
			statusCode          int
			statusText          string
			expectedSuggestions []string
		}{
			{
				statusCode:          400,
				statusText:          "Bad Request",
				expectedSuggestions: []string{"Bad Request (400)", "check the URL format"},
			},
			{
				statusCode:          401,
				statusText:          "Unauthorized",
				expectedSuggestions: []string{"Unauthorized (401)", "authentication may be required"},
			},
			{
				statusCode:          403,
				statusText:          "Forbidden",
				expectedSuggestions: []string{"Forbidden (403)", "permission"},
			},
			{
				statusCode:          404,
				statusText:          "Not Found",
				expectedSuggestions: []string{"Not Found (404)", "doesn't exist"},
			},
			{
				statusCode:          429,
				statusText:          "Too Many Requests",
				expectedSuggestions: []string{"Too Many Requests (429)", "rate limited"},
			},
			{
				statusCode:          500,
				statusText:          "Internal Server Error",
				expectedSuggestions: []string{"Internal Server Error (500)", "remote server has an issue"},
			},
			{
				statusCode:          502,
				statusText:          "Bad Gateway",
				expectedSuggestions: []string{"Bad Gateway (502)", "upstream server error"},
			},
			{
				statusCode:          503,
				statusText:          "Service Unavailable",
				expectedSuggestions: []string{"Service Unavailable (503)", "temporarily down"},
			},
			{
				statusCode:          504,
				statusText:          "Gateway Timeout",
				expectedSuggestions: []string{"Gateway Timeout (504)", "request timed out"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.statusText, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.statusCode)
					w.Write([]byte(tc.statusText))
				}))
				defer server.Close()

				client := NewTestClient(nil)
				ctx := context.Background()

				_, err := client.Fetch(ctx, server.URL)
				require.Error(t, err)

				var richErr *errors.RichError
				require.ErrorAs(t, err, &richErr)
				assert.Equal(t, errors.ErrorTypeRemoteHTTP, richErr.Type)
				assert.Equal(t, server.URL, richErr.Path)
				assert.Equal(t, tc.statusCode, richErr.Context["status_code"])
				assert.Contains(t, richErr.Context["status"], tc.statusText)

				suggestionsText := strings.Join(richErr.Suggestions, " ")
				for _, expectedSuggestion := range tc.expectedSuggestions {
					assert.Contains(t, suggestionsText, expectedSuggestion,
						"Expected suggestion '%s' not found in: %v", expectedSuggestion, richErr.Suggestions)
				}
			})
		}
	})

	t.Run("timeout_errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("delayed response"))
		}))
		defer server.Close()

		config := &HTTPConfig{
			Timeout:      50 * time.Millisecond,
			MaxRedirects: 5,
			UserAgent:    "test-agent",
			Headers:      map[string]string{"Accept": "text/yaml"},
			MaxBodySize:  1024 * 1024,
		}
		client := NewTestClient(config)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)

		var richErr *errors.RichError
		require.ErrorAs(t, err, &richErr)
		assert.Equal(t, errors.ErrorTypeRemoteTimeout, richErr.Type)
		assert.Equal(t, server.URL, richErr.Path)
		assert.Equal(t, config.Timeout, richErr.Context["timeout"])
		assert.Greater(t, len(richErr.Suggestions), 0)

		suggestionsText := strings.Join(richErr.Suggestions, " ")
		assert.Contains(t, suggestionsText, "too long")
	})

	t.Run("network_errors", func(t *testing.T) {
		client := NewTestClient(nil)
		ctx := context.Background()

		_, err := client.Fetch(ctx, "http://this-domain-does-not-exist-12345.com/config")
		require.Error(t, err)

		var richErr *errors.RichError
		require.ErrorAs(t, err, &richErr)
		assert.Equal(t, errors.ErrorTypeRemoteNetwork, richErr.Type)
		assert.Equal(t, "http://this-domain-does-not-exist-12345.com/config", richErr.Path)
	})

	t.Run("context_cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)

		var richErr *errors.RichError
		require.ErrorAs(t, err, &richErr)
		assert.Equal(t, errors.ErrorTypeRemoteTimeout, richErr.Type)
		assert.Contains(t, err.Error(), "request timed out")
	})

	t.Run("fetchWithHeaders_errors", func(t *testing.T) {
		client := NewClient(nil)
		ctx := context.Background()

		headers := map[string]string{"Authorization": "Bearer token"}
		_, err := client.FetchWithHeaders(ctx, "http://localhost:8080/config", headers)
		require.Error(t, err)

		var richErr *errors.RichError
		require.ErrorAs(t, err, &richErr)
		assert.Equal(t, errors.ErrorTypeRemoteSSRF, richErr.Type)
	})
}

func TestErrorFormatting(t *testing.T) {
	t.Run("ssrf_error_formatting", func(t *testing.T) {
		err := errors.RemoteSSRFError("http://localhost:8080/test", "localhost addresses not allowed")

		assert.Contains(t, err.Error(), "validate URL")
		assert.Contains(t, err.Error(), "URL blocked for security reasons")
		assert.Contains(t, err.Error(), "localhost addresses not allowed")

		detailed := fmt.Sprintf("%+v", err)
		assert.Contains(t, detailed, "Context:")
		assert.Contains(t, detailed, "Suggestions:")
		assert.Contains(t, detailed, "url: http://localhost:8080/test")
		assert.Contains(t, detailed, "block_reason: localhost addresses not allowed")
	})

	t.Run("http_error_formatting", func(t *testing.T) {
		err := errors.RemoteHTTPError("https://example.com/config.yaml", 404, "Not Found")

		assert.Contains(t, err.Error(), "HTTP request")
		assert.Contains(t, err.Error(), "HTTP 404: Not Found")

		detailed := fmt.Sprintf("%+v", err)
		assert.Contains(t, detailed, "status_code: 404")
		assert.Contains(t, detailed, "status: Not Found")
		assert.Contains(t, detailed, "Not Found (404)")
	})

	t.Run("timeout_error_formatting", func(t *testing.T) {
		err := errors.RemoteTimeoutError("https://slow.example.com/config.yaml", 30*time.Second)

		assert.Contains(t, err.Error(), "request timeout")
		assert.Contains(t, err.Error(), "request timed out")

		detailed := fmt.Sprintf("%+v", err)
		assert.Contains(t, detailed, "timeout: 30s")
	})
}

func TestErrorChaining(t *testing.T) {
	t.Run("remote_config_fetch_chaining", func(t *testing.T) {
		originalErr := fmt.Errorf("connection timeout")
		err := errors.RemoteConfigFetch("https://example.com/config.yaml", originalErr)

		assert.ErrorIs(t, err, originalErr)
		assert.Contains(t, err.Error(), "fetch remote config")
		assert.Contains(t, err.Error(), "connection timeout")
	})

	t.Run("remote_config_parse_chaining", func(t *testing.T) {
		originalErr := fmt.Errorf("yaml: line 5: invalid syntax")
		err := errors.RemoteConfigParse("https://example.com/config.yaml", originalErr)

		assert.ErrorIs(t, err, originalErr)
		assert.Contains(t, err.Error(), "parse remote config")
		assert.Contains(t, err.Error(), "yaml: line 5")

		assert.Contains(t, err.Context, "parse_error")
	})
}
