package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteIntegration(t *testing.T) {
	t.Run("end_to_end_scenarios", func(t *testing.T) {
		t.Run("successful_remote_fetch_with_caching", func(t *testing.T) {
			requestCount := 0
			yamlContent := `metadata:
  name: "Remote Config"
  version: "1.0.0"
rules:
  - name: "Remote Rule"
    content: "This rule comes from remote server"
    priority: 5`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set("Content-Type", "text/yaml")
				w.Header().Set("ETag", "remote-etag-123")
				w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(yamlContent))
			}))
			defer server.Close()

			client := NewTestClient(nil)
			ctx := context.Background()

			content1, err := client.Fetch(ctx, server.URL)
			require.NoError(t, err)
			assert.Equal(t, yamlContent, string(content1))
			assert.Equal(t, 1, requestCount)

			content2, err := client.Fetch(ctx, server.URL)
			require.NoError(t, err)
			assert.Equal(t, yamlContent, string(content2))
			assert.Equal(t, 1, requestCount)

			if client.cache != nil {
				stats := client.cache.Stats()
				assert.True(t, stats.MemoryEntries > 0 || stats.DiskEntries > 0)
			}
		})

		t.Run("server_error_handling", func(t *testing.T) {
			errorScenarios := []struct {
				name       string
				statusCode int
				response   string
			}{
				{"not_found", http.StatusNotFound, "Config not found"},
				{"server_error", http.StatusInternalServerError, "Server error"},
				{"unauthorized", http.StatusUnauthorized, "Unauthorized access"},
				{"forbidden", http.StatusForbidden, "Access forbidden"},
				{"bad_gateway", http.StatusBadGateway, "Bad gateway"},
			}

			for _, scenario := range errorScenarios {
				t.Run(scenario.name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(scenario.statusCode)
						w.Write([]byte(scenario.response))
					}))
					defer server.Close()

					client := NewTestClient(nil)
					ctx := context.Background()

					_, err := client.Fetch(ctx, server.URL)
					require.Error(t, err)
					assert.Contains(t, err.Error(), fmt.Sprintf("HTTP %d", scenario.statusCode))
				})
			}
		})

		t.Run("network_timeout_scenarios", func(t *testing.T) {
			t.Run("slow_server_timeout", func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("slow response"))
				}))
				defer server.Close()

				config := &HTTPConfig{
					Timeout:      50 * time.Millisecond,
					MaxRedirects: 5,
					UserAgent:    "test-agent",
					MaxBodySize:  1024 * 1024,
				}
				client := NewTestClient(config)
				ctx := context.Background()

				_, err := client.Fetch(ctx, server.URL)
				require.Error(t, err)
				assert.True(t,
					strings.Contains(err.Error(), "timeout") ||
						strings.Contains(err.Error(), "context deadline exceeded"))
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
				assert.Contains(t, err.Error(), "request timed out")
			})
		})

		t.Run("large_content_handling", func(t *testing.T) {
			t.Run("content_within_limits", func(t *testing.T) {
				content := strings.Repeat("a", 500*1024)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(content))
				}))
				defer server.Close()

				client := NewTestClient(nil)
				ctx := context.Background()

				response, err := client.Fetch(ctx, server.URL)
				require.NoError(t, err)
				assert.Equal(t, content, string(response))
			})

			t.Run("content_exceeds_limits", func(t *testing.T) {
				largeContent := strings.Repeat("b", 2*1024*1024)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(largeContent))
				}))
				defer server.Close()

				config := DefaultHTTPConfig()
				config.MaxBodySize = 1024 * 1024
				client := NewTestClient(config)
				ctx := context.Background()

				_, err := client.Fetch(ctx, server.URL)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "response body too large")
			})
		})
	})

	t.Run("performance_integration_tests", func(t *testing.T) {
		t.Run("concurrent_requests_performance", func(t *testing.T) {
			content := "performance test content"
			requestCount := 0
			mu := sync.Mutex{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requestCount++
				mu.Unlock()

				w.Header().Set("ETag", "perf-etag")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(content))
			}))
			defer server.Close()

			client := NewTestClient(nil)
			ctx := context.Background()

			const numRequests = 50
			var wg sync.WaitGroup
			results := make(chan string, numRequests)
			errors := make(chan error, numRequests)

			start := time.Now()

			for i := 0; i < numRequests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					response, err := client.Fetch(ctx, server.URL)
					if err != nil {
						errors <- err
					} else {
						results <- string(response)
					}
				}()
			}

			wg.Wait()
			elapsed := time.Since(start)

			close(results)
			close(errors)

			successCount := 0
			for result := range results {
				assert.Equal(t, content, result)
				successCount++
			}

			errorCount := 0
			for err := range errors {
				t.Errorf("Request failed: %v", err)
				errorCount++
			}

			assert.Equal(t, numRequests, successCount+errorCount)
			assert.Equal(t, 0, errorCount)

			assert.Less(t, elapsed, 5*time.Second, "Concurrent requests took too long")

			mu.Lock()
			finalRequestCount := requestCount
			mu.Unlock()
			assert.LessOrEqual(t, finalRequestCount, numRequests,
				"Request count should not exceed number of requests")
		})
	})
}

func TestRemoteComplexScenarios(t *testing.T) {
	t.Run("multi_server_federation", func(t *testing.T) {
		servers := make([]*httptest.Server, 3)
		contents := []string{
			`metadata:
  name: "Config Part 1"
rules:
  - name: "Rule from Server 1"
    content: "First server rule"`,
			`rules:
  - name: "Rule from Server 2"
    content: "Second server rule"`,
			`sections:
  - title: "Section from Server 3"
    content: "Third server section"`,
		}

		for i := 0; i < 3; i++ {
			content := contents[i]
			servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/yaml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(content))
			}))
		}
		defer func() {
			for _, server := range servers {
				server.Close()
			}
		}()

		client := NewTestClient(nil)
		ctx := context.Background()

		var responses []string
		for _, server := range servers {
			content, err := client.Fetch(ctx, server.URL)
			require.NoError(t, err)
			responses = append(responses, string(content))
		}

		for i, response := range responses {
			assert.Contains(t, response, fmt.Sprintf("Server %d", i+1))
		}
	})

	t.Run("real_world_github_like_server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("ETag", "\"github-etag-123\"")
			w.Header().Set("Cache-Control", "max-age=300")
			w.Header().Set("X-Content-Type-Options", "nosniff")

			yamlContent := `metadata:
  name: "GitHub Config"
  version: "2.0.0"
rules:
  - name: "GitHub Rule"
    content: "Configuration from GitHub repository"
    priority: 8`

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(yamlContent))
		}))
		defer server.Close()

		client := NewTestClient(nil)
		ctx := context.Background()

		content, err := client.Fetch(ctx, server.URL)
		require.NoError(t, err)
		assert.Contains(t, string(content), "GitHub Config")
		assert.Contains(t, string(content), "GitHub Rule")
	})
}

func BenchmarkRemoteIntegration(b *testing.B) {
	content := strings.Repeat("benchmark content ", 50)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "benchmark-etag")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	b.Run("full_stack_fetch", func(b *testing.B) {
		client := NewTestClient(nil)
		ctx := context.Background()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if client.cache != nil {
				client.cache.ClearMemory()
			}

			_, err := client.Fetch(ctx, server.URL)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cached_fetch", func(b *testing.B) {
		client := NewTestClient(nil)
		ctx := context.Background()

		client.Fetch(ctx, server.URL)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_, err := client.Fetch(ctx, server.URL)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
