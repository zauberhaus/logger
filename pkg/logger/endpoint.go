package logger

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var ErrInsecureEndpoint = errors.New("insecure endpoint: https required")

func CheckEndpoint(endpoint string, allowHTTP bool) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid endpoint %q: missing host", endpoint)
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if allowHTTP {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrInsecureEndpoint, u.Redacted())
	default:
		return fmt.Errorf("invalid endpoint %q: unsupported scheme %q", u.Redacted(), u.Scheme)
	}
}

func SecureClient(c *http.Client, allowHTTP bool) *http.Client {
	if c == nil {
		c = http.DefaultClient
	}
	if allowHTTP {
		return c
	}

	secure := *c
	next := c.CheckRedirect
	secure.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("%w: redirect to %s", ErrInsecureEndpoint, req.URL.Redacted())
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &secure
}
