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
	config := defaultHTTPConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 5, config.MaxRedirects)
	assert.Contains(t, config.UserAgent, "ai-rulez")
	assert.Equal(t, int64(10*1024*1024), config.MaxBodySize)
	assert.Contains(t, config.Headers["Accept"], "yaml")
}

func TestClient_Fetch(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		expectedContent := "test: yaml content\nrules:\n  - name: test"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	t.Run("http error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &HTTPConfig{
			Timeout:      10 * time.Millisecond,
			MaxRedirects: 5,
			UserAgent:    "test",
			MaxBodySize:  1024,
		}
		client := NewTestClient(config)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		assert.Error(t, err)
	})

	t.Run("redirect handling", func(t *testing.T) {
		redirectCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirectCount < 2 {
				redirectCount++
				http.Redirect(w, r, fmt.Sprintf("/redirect%d", redirectCount), http.StatusMovedPermanently)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("final content"))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		content, err := client.Fetch(ctx, server.URL)
		require.NoError(t, err)
		assert.Equal(t, "final content", string(content))
	})

	t.Run("too many redirects", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/redirect", http.StatusMovedPermanently)
		}))
		defer server.Close()

		config := &HTTPConfig{
			Timeout:      5 * time.Second,
			MaxRedirects: 2,
			UserAgent:    "test",
		}
		client := NewTestClient(config)
		ctx := context.Background()

		_, err := client.Fetch(ctx, server.URL)
		assert.Error(t, err)
	})
}

func TestCache_Operations(t *testing.T) {
	ctx := context.Background()
	cache := newCache(nil)

	t.Run("set and get", func(t *testing.T) {
		url := "https://example.com/test"
		content := []byte("test content")

		cache.set(ctx, url, content, "", "")

		entry, found := cache.get(ctx, url)
		assert.True(t, found)
		assert.Equal(t, content, entry.Content)
	})

	t.Run("miss", func(t *testing.T) {
		_, found := cache.get(ctx, "https://example.com/nonexistent")
		assert.False(t, found)
	})

	t.Run("ttl expiration", func(t *testing.T) {
		config := &CacheConfig{
			MemoryTTL:        50 * time.Millisecond,
			MaxMemoryEntries: 100,
			DiskCacheDir:     t.TempDir(),
		}
		cache := newCache(config)

		url := "https://example.com/expire"
		cache.set(ctx, url, []byte("expires"), "", "")

		_, found := cache.get(ctx, url)
		assert.True(t, found)

		time.Sleep(60 * time.Millisecond)

		_, found = cache.get(ctx, url)
		assert.False(t, found)
	})
}

func TestURLValidator(t *testing.T) {
	validator := newURLValidator()

	t.Run("valid URLs", func(t *testing.T) {
		validURLs := []string{
			"https://example.com/config.yaml",
			"http://github.com/user/repo/file.yaml",
			"https://raw.githubusercontent.com/user/repo/main/config.yaml",
		}

		for _, url := range validURLs {
			err := validator.Validate(url)
			assert.NoError(t, err, "URL should be valid: %s", url)
		}
	})

	t.Run("blocked private IPs", func(t *testing.T) {
		blockedURLs := []string{
			"http://localhost/config.yaml",
			"http://127.0.0.1/config.yaml",
			"http://192.168.1.1/config.yaml",
			"http://10.0.0.1/config.yaml",
			"http://172.16.0.1/config.yaml",
			"http://[::1]/config.yaml",
		}

		for _, url := range blockedURLs {
			err := validator.Validate(url)
			assert.Error(t, err, "URL should be blocked: %s", url)
		}
	})

	t.Run("blocked metadata endpoints", func(t *testing.T) {
		blockedURLs := []string{
			"http://169.254.169.254/latest/meta-data/",
			"http://metadata.google.internal/computeMetadata/v1/",
		}

		for _, url := range blockedURLs {
			err := validator.Validate(url)
			assert.Error(t, err, "Metadata endpoint should be blocked: %s", url)
		}
	})

	t.Run("invalid URLs", func(t *testing.T) {
		invalidURLs := []string{
			"not-a-url",
			"ftp://example.com/file",
			"://missing-scheme.com",
			"",
		}

		for _, url := range invalidURLs {
			err := validator.Validate(url)
			assert.Error(t, err, "URL should be invalid: %s", url)
		}
	})
}

func TestClient_WithCache(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("ETag", "test-etag")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("cached content"))
	}))
	defer server.Close()

	client := NewTestClient(nil)
	ctx := context.Background()

	content1, err := client.Fetch(ctx, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "cached content", string(content1))
	assert.Equal(t, 1, requestCount)

	content2, err := client.Fetch(ctx, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "cached content", string(content2))
	assert.Equal(t, 1, requestCount)
}

func TestClient_ContentTypes(t *testing.T) {
	contentTypes := []string{
		"text/yaml",
		"application/yaml",
		"text/x-yaml",
		"application/x-yaml",
		"text/plain",
	}

	for _, contentType := range contentTypes {
		t.Run(strings.ReplaceAll(contentType, "/", "_"), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("test: content"))
			}))
			defer server.Close()

			client := NewTestClient(nil)
			ctx := context.Background()

			content, err := client.Fetch(ctx, server.URL)
			require.NoError(t, err)
			assert.Equal(t, "test: content", string(content))
		})
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		expectErr  bool
	}{
		{"ok", http.StatusOK, false},
		{"not_found", http.StatusNotFound, true},
		{"unauthorized", http.StatusUnauthorized, true},
		{"forbidden", http.StatusForbidden, true},
		{"internal_error", http.StatusInternalServerError, true},
		{"bad_gateway", http.StatusBadGateway, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte("response"))
			}))
			defer server.Close()

			client := NewTestClient(nil)
			ctx := context.Background()

			_, err := client.Fetch(ctx, server.URL)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
