package remote

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewTestClient creates a client that bypasses URL validation for testing purposes
func NewTestClient(config *HTTPConfig) *Client {
	client := NewClient(config)

	// Replace validator with a test validator that allows localhost
	client.validator = &testURLValidator{}

	return client
}

// NewTestClientWithRedirectValidation creates a test client that still validates redirects
func NewTestClientWithRedirectValidation(config *HTTPConfig) *Client {
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
			return nil // Allow all redirects in tests
		},
	}

	return &Client{
		http:      httpClient,
		validator: &testURLValidator{},
		config:    config,
	}
}

// testURLValidator allows all URLs for testing
type testURLValidator struct{}

func (v *testURLValidator) Validate(url string) error {
	// Allow all URLs in tests
	return nil
}
