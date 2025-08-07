package remote

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

// NewTestClient creates a client that bypasses URL validation for testing purposes
func NewTestClient(config *HTTPConfig) *Client {
	if config == nil {
		config = DefaultHTTPConfig()
	}

	// Create resty client with configuration but NO OnBeforeRequest hook
	client := resty.New()

	// Set timeout
	client.SetTimeout(config.Timeout)

	// Set User-Agent
	client.SetHeader("User-Agent", config.UserAgent)

	// Set custom headers
	for key, value := range config.Headers {
		client.SetHeader(key, value)
	}

	// Configure redirects without SSRF validation (for testing)
	client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(config.MaxRedirects))

	// Add response size validation hook (same as production client)
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		if int64(len(r.Body())) > config.MaxBodySize {
			return fmt.Errorf("response body too large (limit: %d bytes)", config.MaxBodySize)
		}
		return nil
	})

	return &Client{
		resty:     client,
		validator: &testURLValidator{},
		config:    config,
		cache:     NewCache(nil), // Use default cache for tests
	}
}

// NewTestClientWithRedirectValidation creates a test client that allows redirects but bypasses URL validation
func NewTestClientWithRedirectValidation(config *HTTPConfig) *Client {
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

	// Configure redirects without SSRF validation (for testing)
	client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(config.MaxRedirects))

	// Add response size validation hook (same as production client)
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		if int64(len(r.Body())) > config.MaxBodySize {
			return fmt.Errorf("response body too large (limit: %d bytes)", config.MaxBodySize)
		}
		return nil
	})

	return &Client{
		resty:     client,
		validator: &testURLValidator{},
		config:    config,
		cache:     NewCache(nil), // Use default cache for tests
	}
}

// testURLValidator allows all URLs for testing
type testURLValidator struct{}

func (v *testURLValidator) Validate(url string) error {
	// Allow all URLs in tests
	return nil
}
