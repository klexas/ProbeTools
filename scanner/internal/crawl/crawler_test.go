package crawl

import (
	"context"
	"net/url"
	"testing"

	"github.com/klexas/ProbeTools/scanner/internal/discovery"
	"github.com/klexas/ProbeTools/scanner/internal/extract"
	"github.com/klexas/ProbeTools/scanner/internal/fetch"
)

type stubFetcher struct {
	pages map[string]fetch.PageResult
}

func (s stubFetcher) Get(_ context.Context, target *url.URL) (fetch.PageResult, error) {
	return s.pages[target.String()], nil
}

func TestCrawlerRespectsMaxDepth(t *testing.T) {
	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	rootURL, _ := url.Parse("https://example.com/")
	levelOneURL, _ := url.Parse("https://example.com/level-one")
	childURL, _ := url.Parse("https://example.com/level-one/child")

	fetcher := stubFetcher{
		pages: map[string]fetch.PageResult{
			"https://example.com/": {
				URL:         rootURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><a href="/level-one">one</a></body></html>`),
			},
			"https://example.com/level-one": {
				URL:         levelOneURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><a href="/level-one/child">child</a></body></html>`),
			},
			"https://example.com/level-one/child": {
				URL:         childURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><p>child</p></body></html>`),
			},
		},
	}

	crawler := NewCrawler(
		target,
		10,
		1,
		fetcher,
		extract.NewHTMLExtractor(),
		discovery.NewAnalyzer(target, false, nil),
	)

	requests, pages, _, warnings, err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("run crawler: %v", err)
	}

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}

	if pages[0].URL != "https://example.com/" || pages[0].Depth != 0 {
		t.Fatalf("unexpected root page %+v", pages[0])
	}

	if pages[1].URL != "https://example.com/level-one" || pages[1].Depth != 1 {
		t.Fatalf("unexpected level one page %+v", pages[1])
	}
}

func TestCrawlerFollowsCanonicalRedirectHost(t *testing.T) {
	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	rootURL, _ := url.Parse("https://www.example.com/")
	nextURL, _ := url.Parse("https://www.example.com/next")

	fetcher := stubFetcher{
		pages: map[string]fetch.PageResult{
			"https://example.com/": {
				URL:         rootURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><a href="/next">next</a></body></html>`),
			},
			"https://www.example.com/next": {
				URL:         nextURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><p>next</p></body></html>`),
			},
		},
	}

	crawler := NewCrawler(
		target,
		10,
		1,
		fetcher,
		extract.NewHTMLExtractor(),
		discovery.NewAnalyzer(target, false, nil),
	)

	requests, pages, _, warnings, err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("run crawler: %v", err)
	}

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}

	if requests[1].URL != "https://www.example.com/next" {
		t.Fatalf("expected redirected host follow-up request, got %+v", requests[1])
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
}

func TestCrawlerDiscoversLinksFromScripts(t *testing.T) {
	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	rootURL, _ := url.Parse("https://example.com/")
	scriptURL, _ := url.Parse("https://example.com/assets/app.js")
	loginURL, _ := url.Parse("https://example.com/login")

	fetcher := stubFetcher{
		pages: map[string]fetch.PageResult{
			"https://example.com/": {
				URL:         rootURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><head><script src="/assets/app.js"></script></head><body><div id="root"></div></body></html>`),
			},
			"https://example.com/assets/app.js": {
				URL:         scriptURL,
				StatusCode:  200,
				ContentType: "application/javascript",
				Body:        []byte(`const routes=["/login","/assets/logo.svg","/api/session"];`),
			},
			"https://example.com/login": {
				URL:         loginURL,
				StatusCode:  200,
				ContentType: "text/html",
				Body:        []byte(`<html><body><p>login</p></body></html>`),
			},
		},
	}

	crawler := NewCrawler(
		target,
		10,
		1,
		fetcher,
		extract.NewHTMLExtractor(),
		discovery.NewAnalyzer(target, false, nil),
	)

	requests, pages, _, warnings, err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("run crawler: %v", err)
	}

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 requests including script fetch, got %d", len(requests))
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	if requests[1].URL != "https://example.com/assets/app.js" {
		t.Fatalf("expected script request to be recorded, got %+v", requests[1])
	}

	if pages[1].URL != "https://example.com/login" {
		t.Fatalf("expected JS-discovered login route, got %+v", pages[1])
	}
}
