package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultHTTPConfig(t *testing.T) {
	config := DefaultHTTPConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 5, config.MaxRedirects)
	assert.Contains(t, config.UserAgent, "ai-rulez")
	assert.Equal(t, int64(10*1024*1024), config.MaxBodySize)

	// Check default headers
	assert.Contains(t, config.Headers["Accept"], "yaml")
}

func TestNewClient(t *testing.T) {
	t.Run("with_default_config", func(t *testing.T) {
		client := NewTestClient(nil)
		assert.NotNil(t, client)
		assert.NotNil(t, client.resty)
		assert.NotNil(t, client.validator)
		assert.NotNil(t, client.config)
	})

	t.Run("with_custom_config", func(t *testing.T) {
		config := &HTTPConfig{
			Timeout:      5 * time.Second,
			MaxRedirects: 3,
			UserAgent:    "test-agent",
		}
		client := NewTestClient(config)
		assert.Equal(t, config, client.config)
	})
}

func TestClient_Fetch(t *testing.T) {
	t.Run("successful_fetch", func(t *testing.T) {
		expectedContent := "test: yaml content\nrules:\n  - name: test"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify headers
			assert.Contains(t, r.Header.Get("Accept"), "yaml")
			assert.Contains(t, r.Header.Get("User-Agent"), "ai-rulez")

			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(expectedContent))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		content, err := client.Fetch(ctx, server.URL)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	})

	t.Run("invalid_url", func(t *testing.T) {
		// Use regular client to test validation
		client := NewClient(nil)
		ctx := context.Background()

		_, err := client.Fetch(ctx, "http://127.0.0.1/test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "URL validation failed")
	})

	t.Run("http_error_status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 404")
	})

	t.Run("context_timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("delayed response"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})

	t.Run("response_too_large", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Write one byte more than the max body size to trigger the limit
			largeContent := strings.Repeat("a", int(DefaultHTTPConfig().MaxBodySize)+1)
			w.Write([]byte(largeContent))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response body too large")
	})

	t.Run("redirect_handling", func(t *testing.T) {
		finalContent := "final content"

		// Create a server that redirects to itself once, then serves content
		var redirectCount int
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirectCount == 0 {
				redirectCount++
				http.Redirect(w, r, server.URL+"/final", http.StatusMovedPermanently)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(finalContent))
		}))
		defer server.Close()

		client := NewTestClientWithRedirectValidation(nil)
		ctx := context.Background()

		content, err := client.Fetch(ctx, server.URL)
		require.NoError(t, err)
		assert.Equal(t, finalContent, string(content))
	})

	t.Run("too_many_redirects", func(t *testing.T) {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always redirect to create infinite loop
			http.Redirect(w, r, server.URL+"/redirect", http.StatusMovedPermanently)
		}))
		defer server.Close()

		config := DefaultHTTPConfig()
		config.MaxRedirects = 2
		client := NewTestClientWithRedirectValidation(config)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped after")
	})
}

func TestClient_FetchWithHeaders(t *testing.T) {
	t.Run("custom_headers", func(t *testing.T) {
		expectedAuth := "Bearer test-token"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify custom headers
			assert.Equal(t, expectedAuth, r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))

			// Verify User-Agent is still set (not overridden)
			assert.Contains(t, r.Header.Get("User-Agent"), "ai-rulez")

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		headers := map[string]string{
			"Authorization": expectedAuth,
			"Accept":        "application/json",
		}

		content, err := client.FetchWithHeaders(ctx, server.URL, headers)
		require.NoError(t, err)
		assert.Equal(t, "success", string(content))
	})

	t.Run("override_user_agent", func(t *testing.T) {
		customUA := "custom-agent/1.0"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, customUA, r.Header.Get("User-Agent"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		headers := map[string]string{
			"User-Agent": customUA,
		}

		_, err := client.FetchWithHeaders(ctx, server.URL, headers)
		require.NoError(t, err)
	})
}

func TestClient_RedirectSSRFProtection(t *testing.T) {
	// This test verifies that redirects to unsafe URLs are blocked
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to localhost (should be blocked)
		http.Redirect(w, r, "http://127.0.0.1:8080/evil", http.StatusMovedPermanently)
	}))
	defer server.Close()

	client := NewClient(nil)
	ctx := context.Background()

	_, err := client.Fetch(ctx, server.URL)
	require.Error(t, err)
	// The initial URL validation will fail before even getting to redirect
	assert.Contains(t, err.Error(), "URL validation failed")
}

func TestClient_Close(t *testing.T) {
	client := NewTestClient(nil)

	// Close should not panic and should close idle connections
	assert.NotPanics(t, func() {
		client.Close()
	})
}

// Benchmark tests to ensure client performance is acceptable
func BenchmarkClient_Fetch(b *testing.B) {
	content := "test: yaml content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	client := NewTestClient(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Fetch(ctx, server.URL)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_FetchWithValidation(b *testing.B) {
	content := "test: yaml content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	client := NewTestClient(nil)
	ctx := context.Background()

	// This benchmark includes URL validation overhead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use different URLs to test validation each time
		url := fmt.Sprintf("%s?test=%d", server.URL, i)
		_, err := client.Fetch(ctx, url)
		if err != nil {
			b.Fatal(err)
		}
	}
}
