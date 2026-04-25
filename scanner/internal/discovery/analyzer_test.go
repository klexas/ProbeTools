package discovery

import (
	"context"
	"net/url"
	"testing"

	"github.com/klexas/ProbeTools/scanner/internal/extract"
	"github.com/klexas/ProbeTools/scanner/internal/model"
)

func TestAnalyzerKeepsLikelyBackendCalls(t *testing.T) {
	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	pageURL, err := url.Parse("https://example.com/home")
	if err != nil {
		t.Fatalf("parse page url: %v", err)
	}

	apiURL, _ := url.Parse("https://example.com/api/users")
	loginURL, _ := url.Parse("https://example.com/login")
	staticURL, _ := url.Parse("https://example.com/assets/app.js")

	analyzer := NewAnalyzer(target, false, nil)
	matches := analyzer.Observe(pageURL, extract.Extracted{
		Candidates: []extract.Reference{
			{URL: apiURL, Source: model.SourceInlineScript, Method: "GET"},
			{URL: loginURL, Source: model.SourceForm, Method: "POST"},
			{URL: staticURL, Source: model.SourceScriptRef, Method: "GET"},
		},
	})

	if matches != 2 {
		t.Fatalf("expected 2 matched backend calls, got %d", matches)
	}

	calls := analyzer.Finalize(context.Background())
	if len(calls) != 2 {
		t.Fatalf("expected 2 backend calls, got %d", len(calls))
	}

	if calls[0].URL != "https://example.com/api/users" {
		t.Fatalf("unexpected first call: %s", calls[0].URL)
	}

	if calls[1].URL != "https://example.com/login" {
		t.Fatalf("unexpected second call: %s", calls[1].URL)
	}
}
