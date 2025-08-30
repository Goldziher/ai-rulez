package remote

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

func NewTestClient(config *HTTPConfig) *Client {
	if config == nil {
		config = DefaultHTTPConfig()
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
		validator: &testURLValidator{},
		config:    config,
		cache:     NewCache(nil),
	}
}

func NewTestClientWithRedirectValidation(config *HTTPConfig) *Client {
	if config == nil {
		config = DefaultHTTPConfig()
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
		validator: &testURLValidator{},
		config:    config,
		cache:     NewCache(nil),
	}
}

type testURLValidator struct{}

func (v *testURLValidator) Validate(url string) error {
	return nil
}
