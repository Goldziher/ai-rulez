package remote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/samber/oops"
)

type HTTPConfig struct {
	Timeout      time.Duration
	MaxRedirects int
	UserAgent    string
	Headers      map[string]string
	MaxBodySize  int64
}

func defaultHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Timeout:      30 * time.Second,
		MaxRedirects: 5,
		UserAgent:    fmt.Sprintf("ai-rulez/%s", getVersion()),
		Headers: map[string]string{
			"Accept": "text/yaml, application/yaml, text/plain, */*",
		},
		MaxBodySize: 10 * 1024 * 1024,
	}
}

func getVersion() string {
	return "dev"
}

type URLValidatorInterface interface {
	Validate(url string) error
}

type Client struct {
	resty     *resty.Client
	validator URLValidatorInterface
	config    *HTTPConfig
	cache     *Cache
}

func NewClient(config *HTTPConfig) *Client {
	if config == nil {
		config = defaultHTTPConfig()
	}

	client := resty.New()

	client.SetTimeout(config.Timeout)

	client.SetHeader("User-Agent", config.UserAgent)

	for key, value := range config.Headers {
		client.SetHeader(key, value)
	}

	validator := newURLValidator()
	client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(config.MaxRedirects))
	client.OnBeforeRequest(func(c *resty.Client, r *resty.Request) error {
		return validator.Validate(r.URL)
	})

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
		cache:     newCache(nil),
	}
}

// NewTestClient creates a client without URL validation for testing purposes
func NewTestClient(config *HTTPConfig) *Client {
	if config == nil {
		config = defaultHTTPConfig()
	}

	client := resty.New()

	client.SetTimeout(config.Timeout)
	client.SetHeader("User-Agent", config.UserAgent)

	for key, value := range config.Headers {
		client.SetHeader(key, value)
	}

	client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(config.MaxRedirects))

	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		if int64(len(r.Body())) > config.MaxBodySize {
			return fmt.Errorf("response body too large (limit: %d bytes)", config.MaxBodySize)
		}
		return nil
	})

	return &Client{
		resty:     client,
		validator: &testURLValidator{}, // Test validator that allows all URLs
		config:    config,
		cache:     newCache(nil),
	}
}

// testURLValidator allows all URLs for testing
type testURLValidator struct{}

func (v *testURLValidator) Validate(url string) error {
	return nil // Allow all URLs in tests
}

func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
	if err := c.validator.Validate(url); err != nil {
		return nil, oops.
			With("url", url).
			With("block_reason", err.Error()).
			Hint("Use a public URL (not localhost, private IPs, or metadata services)\nEnsure the URL uses http:// or https:// scheme\nAvoid URLs that resolve to private IP ranges or loopback addresses").
			Errorf("URL blocked for security reasons: %s", err.Error())
	}

	if c.cache != nil {
		if entry, found := c.cache.get(ctx, url); found {
			return entry.Content, nil
		}
	}

	resp, err := c.resty.R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
			return nil, oops.
				With("url", url).
				With("timeout", c.config.Timeout).
				Hint("The request took too long to complete\nCheck if the remote server is responding slowly\nTry again later when the server may be less busy\nConsider using a local copy of the configuration if available").
				Errorf("request timed out")
		}
		if strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no route") {
			return nil, oops.
				With("url", url).
				Hint("Check your network connectivity\nVerify the URL is accessible from your location\nCheck if a proxy or firewall is blocking the request").
				Wrapf(err, "network request")
		}
		return nil, oops.
			With("url", url).
			Hint("Check your network connectivity\nVerify the URL is accessible from your location\nCheck if a proxy or firewall is blocking the request").
			Wrapf(err, "network request")
	}

	if resp.StatusCode() != 200 {
		return nil, createHTTPError(url, resp.StatusCode(), resp.Status())
	}

	body := resp.Body()

	if c.cache != nil {
		etag := resp.Header().Get("ETag")
		lastModified := resp.Header().Get("Last-Modified")
		if err := c.cache.set(ctx, url, body, etag, lastModified); err != nil {
			_ = err
		}
	}

	return body, nil
}

// createHTTPError creates an error for HTTP status codes
func createHTTPError(url string, statusCode int, status string) error {
	var hint string

	switch statusCode {
	case 400:
		hint = "Bad Request (400) - check the URL format"
	case 401:
		hint = "Unauthorized (401) - authentication may be required\nCheck if you need to provide API keys or credentials"
	case 403:
		hint = "Forbidden (403) - you don't have permission to access this resource\nCheck if the resource requires special permissions"
	case 404:
		hint = "Not Found (404) - the resource doesn't exist at this URL\nVerify the URL path is correct"
	case 429:
		hint = "Too Many Requests (429) - you're being rate limited\nWait before retrying or contact the service provider"
	case 500:
		hint = "Internal Server Error (500) - the remote server has an issue\nTry again later or contact the service provider"
	case 502:
		hint = "Bad Gateway (502) - upstream server error\nThe service may be temporarily unavailable"
	case 503:
		hint = "Service Unavailable (503) - the service is temporarily down\nTry again later"
	case 504:
		hint = "Gateway Timeout (504) - the request timed out\nThe upstream server is too slow or unresponsive"
	default:
		if statusCode >= 400 && statusCode < 500 {
			hint = fmt.Sprintf("Client error (%d) - check your request", statusCode)
		} else if statusCode >= 500 {
			hint = fmt.Sprintf("Server error (%d) - the remote service has an issue", statusCode)
		}
	}

	return oops.
		With("url", url).
		With("status_code", statusCode).
		With("status", status).
		Hint(hint).
		Errorf("HTTP %d: %s", statusCode, status)
}
