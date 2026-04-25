package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PageResult struct {
	URL         *url.URL
	StatusCode  int
	ContentType string
	Body        []byte
}

type Fetcher interface {
	Get(ctx context.Context, target *url.URL) (PageResult, error)
}

type HTTPClient struct {
	client    *http.Client
	userAgent string
}

func NewHTTPClient(timeout time.Duration, userAgent string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
		userAgent: userAgent,
	}
}

func (c *HTTPClient) Get(ctx context.Context, target *url.URL) (PageResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return PageResult{}, fmt.Errorf("build GET request for %s: %w", target.String(), err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return PageResult{}, fmt.Errorf("GET %s: %w", target.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return PageResult{}, fmt.Errorf("read %s: %w", target.String(), err)
	}

	return PageResult{
		URL:         resp.Request.URL,
		StatusCode:  resp.StatusCode,
		ContentType: normalizeContentType(resp.Header.Get("Content-Type")),
		Body:        body,
	}, nil
}

func normalizeContentType(contentType string) string {
	if contentType == "" {
		return ""
	}

	parts := strings.Split(contentType, ";")
	return strings.TrimSpace(strings.ToLower(parts[0]))
}
