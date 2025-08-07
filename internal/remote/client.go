package remote

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPConfig configures the HTTP client for remote fetching
type HTTPConfig struct {
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	ResponseTimeout time.Duration
	MaxRedirects    int
	UserAgent       string
	TLSConfig       *tls.Config
	Headers         map[string]string
	MaxBodySize     int64
}

// DefaultHTTPConfig returns sensible default configuration for HTTP operations
func DefaultHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Timeout:         30 * time.Second,
		ConnectTimeout:  10 * time.Second,
		ResponseTimeout: 20 * time.Second,
		MaxRedirects:    5,
		UserAgent:       fmt.Sprintf("ai-rulez/%s", getVersion()),
		Headers: map[string]string{
			"Accept":          "text/yaml, application/yaml, text/plain, */*",
			"Accept-Encoding": "gzip, deflate",
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

// Client wraps http.Client with remote reference specific configuration
type Client struct {
	http      *http.Client
	validator URLValidatorInterface
	config    *HTTPConfig
}

// NewClient creates a new HTTP client with the specified configuration
func NewClient(config *HTTPConfig) *Client {
	if config == nil {
		config = DefaultHTTPConfig()
	}

	// Create custom transport with timeouts and connection limits
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: config.TLSConfig,

		// Connection pool settings
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     60 * time.Second,

		// Timeouts
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: config.ResponseTimeout,

		// Disable HTTP/2 for better compatibility and predictable behavior
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	// Create HTTP client with custom transport
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("too many redirects (limit: %d)", config.MaxRedirects)
			}

			// Validate redirect URL for SSRF protection
			validator := NewURLValidator()
			if err := validator.Validate(req.URL.String()); err != nil {
				return fmt.Errorf("redirect to unsafe URL: %w", err)
			}

			return nil
		},
	}

	return &Client{
		http:      httpClient,
		validator: NewURLValidator(),
		config:    config,
	}
}

// Fetch retrieves content from the specified URL with validation and timeout
func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
	// Validate URL for SSRF protection
	if err := c.validator.Validate(url); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom headers
	for key, value := range c.config.Headers {
		req.Header.Set(key, value)
	}

	// Set User-Agent
	req.Header.Set("User-Agent", c.config.UserAgent)

	// Execute request
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Limit response body size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, c.config.MaxBodySize)

	// Read response body
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if we hit the size limit
	if int64(len(body)) == c.config.MaxBodySize {
		return nil, fmt.Errorf("response body too large (limit: %d bytes)", c.config.MaxBodySize)
	}

	return body, nil
}

// FetchWithHeaders retrieves content with custom headers for this request
func (c *Client) FetchWithHeaders(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	// Validate URL for SSRF protection
	if err := c.validator.Validate(url); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers first
	for key, value := range c.config.Headers {
		req.Header.Set(key, value)
	}

	// Set custom headers (these override defaults)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Set User-Agent (can be overridden by custom headers)
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	// Execute request
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Limit response body size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, c.config.MaxBodySize)

	// Read response body
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if we hit the size limit
	if int64(len(body)) == c.config.MaxBodySize {
		return nil, fmt.Errorf("response body too large (limit: %d bytes)", c.config.MaxBodySize)
	}

	return body, nil
}

// Close closes idle connections in the HTTP client
func (c *Client) Close() {
	if transport, ok := c.http.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}
