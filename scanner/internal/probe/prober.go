package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/klexas/ProbeTools/scanner/internal/model"
)

type Prober interface {
	Probe(ctx context.Context, endpoint *url.URL) model.ProbeResult
}

type HTTPProber struct {
	client    *http.Client
	userAgent string
}

func NewHTTPProber(timeout time.Duration, userAgent string) *HTTPProber {
	return &HTTPProber{
		client: &http.Client{
			Timeout: timeout,
		},
		userAgent: userAgent,
	}
}

func (p *HTTPProber) Probe(ctx context.Context, endpoint *url.URL) model.ProbeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
	if err != nil {
		return model.ProbeResult{
			Attempted: true,
			Method:    http.MethodHead,
			Error:     fmt.Sprintf("build probe request: %v", err),
		}
	}

	req.Header.Set("User-Agent", p.userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return model.ProbeResult{
			Attempted: true,
			Method:    http.MethodHead,
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	return model.ProbeResult{
		Attempted:  true,
		Method:     http.MethodHead,
		StatusCode: resp.StatusCode,
		FinalURL:   resp.Request.URL.String(),
	}
}
