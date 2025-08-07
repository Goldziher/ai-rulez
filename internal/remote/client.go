package remote

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// HTTPConfig configures the HTTP client for remote fetching
type HTTPConfig struct {
	Timeout      time.Duration
	MaxRedirects int
	UserAgent    string
	Headers      map[string]string
	MaxBodySize  int64
}

// DefaultHTTPConfig returns sensible default configuration for HTTP operations
func DefaultHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Timeout:      30 * time.Second,
		MaxRedirects: 5,
		UserAgent:    fmt.Sprintf("ai-rulez/%s", getVersion()),
		Headers: map[string]string{
			"Accept": "text/yaml, application/yaml, text/plain, */*",
		},
		MaxBodySize: 10 * 1024 * 1024, // 10MB max file size
	}
}

// getVersion returns the current version, falling back to "dev" if not available
func getVersion() string {
	// Try to get version from config metadata or build info
	// For now, return a placeholder - this will be integrated with actual version info
	return "dev"
}

// URLValidatorInterface defines the interface for URL validation
type URLValidatorInterface interface {
	Validate(url string) error
}

// Client wraps resty.Client with remote reference specific configuration
type Client struct {
	resty     *resty.Client
	validator URLValidatorInterface
	config    *HTTPConfig
	cache     *Cache
}

// NewClient creates a new HTTP client with the specified configuration
func NewClient(config *HTTPConfig) *Client {
	if config == nil {
		config = DefaultHTTPConfig()
	}

	// Create resty client with configuration
	client := resty.New()

	// Set timeout
	client.SetTimeout(config.Timeout)

	// Set User-Agent
	client.SetHeader("User-Agent", config.UserAgent)

	// Set custom headers
	for key, value := range config.Headers {
		client.SetHeader(key, value)
	}

	// Configure redirects with SSRF validation
	validator := NewURLValidator()
	client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(config.MaxRedirects))
	client.OnBeforeRequest(func(c *resty.Client, r *resty.Request) error {
		// Validate URL for SSRF protection
		return validator.Validate(r.URL)
	})

	// Add response size validation hook
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		if int64(len(r.Body())) > config.MaxBodySize {
			return fmt.Errorf("response body too large (limit: %d bytes)", config.MaxBodySize)
		}
		return nil
	})

	return &Client{
		resty:     client,
		validator: validator,
		config:    config,
		cache:     NewCache(nil), // Use default cache config
	}
}

// Fetch retrieves content from the specified URL with validation and timeout
func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
	// Validate URL for SSRF protection (done again in OnBeforeRequest hook)
	if err := c.validator.Validate(url); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Check cache first
	if c.cache != nil {
		if entry, found := c.cache.Get(ctx, url); found {
			return entry.Content, nil
		}
	}

	// Execute GET request with context
	resp, err := c.resty.R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Check status code
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.Status())
	}

	body := resp.Body()

	// Store in cache if available
	if c.cache != nil {
		etag := resp.Header().Get("ETag")
		lastModified := resp.Header().Get("Last-Modified")
		if err := c.cache.Set(ctx, url, body, etag, lastModified); err != nil {
			// Log error but don't fail the request
			// In a production implementation, you might want to use a logger here
			_ = err
		}
	}

	return body, nil
}

// FetchWithHeaders retrieves content with custom headers for this request
func (c *Client) FetchWithHeaders(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	// Validate URL for SSRF protection
	if err := c.validator.Validate(url); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Note: We don't cache requests with custom headers as they might affect the response
	// In a more sophisticated implementation, we could include headers in the cache key

	// Execute GET request with custom headers and context
	resp, err := c.resty.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Check status code
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.Status())
	}

	return resp.Body(), nil
}

// Close closes idle connections in the HTTP client
func (c *Client) Close() {
	// Resty handles connection cleanup automatically
	// But we can set the client to nil to help with garbage collection
	c.resty = nil
}
