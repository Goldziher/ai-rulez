package remote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/errors"
	"github.com/go-resty/resty/v2"
)

type HTTPConfig struct {
	Timeout      time.Duration
	MaxRedirects int
	UserAgent    string
	Headers      map[string]string
	MaxBodySize  int64
}

func DefaultHTTPConfig() *HTTPConfig {
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
		config = DefaultHTTPConfig()
	}

	client := resty.New()

	client.SetTimeout(config.Timeout)

	client.SetHeader("User-Agent", config.UserAgent)

	for key, value := range config.Headers {
		client.SetHeader(key, value)
	}

	validator := NewURLValidator()
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
		cache:     NewCache(nil),
	}
}

func (c *Client) Fetch(ctx context.Context, url string) ([]byte, error) {
	if err := c.validator.Validate(url); err != nil {
		return nil, errors.RemoteSSRFError(url, err.Error())
	}

	if c.cache != nil {
		if entry, found := c.cache.Get(ctx, url); found {
			return entry.Content, nil
		}
	}

	resp, err := c.resty.R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
			return nil, errors.RemoteTimeoutError(url, c.config.Timeout)
		}
		if strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no route") {
			return nil, errors.RemoteNetworkError(url, err)
		}
		return nil, errors.RemoteNetworkError(url, err)
	}

	if resp.StatusCode() != 200 {
		return nil, errors.RemoteHTTPError(url, resp.StatusCode(), resp.Status())
	}

	body := resp.Body()

	if c.cache != nil {
		etag := resp.Header().Get("ETag")
		lastModified := resp.Header().Get("Last-Modified")
		if err := c.cache.Set(ctx, url, body, etag, lastModified); err != nil {
			_ = err
		}
	}

	return body, nil
}

func (c *Client) FetchWithHeaders(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	if err := c.validator.Validate(url); err != nil {
		return nil, errors.RemoteSSRFError(url, err.Error())
	}

	resp, err := c.resty.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get(url)
	if err != nil {
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
			return nil, errors.RemoteTimeoutError(url, c.config.Timeout)
		}
		if strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no route") {
			return nil, errors.RemoteNetworkError(url, err)
		}
		return nil, errors.RemoteNetworkError(url, err)
	}

	if resp.StatusCode() != 200 {
		return nil, errors.RemoteHTTPError(url, resp.StatusCode(), resp.Status())
	}

	return resp.Body(), nil
}

func (c *Client) Close() {
	c.resty = nil
}
