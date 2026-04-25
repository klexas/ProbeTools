package crawl

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/klexas/ProbeTools/scanner/internal/discovery"
	"github.com/klexas/ProbeTools/scanner/internal/extract"
	"github.com/klexas/ProbeTools/scanner/internal/fetch"
	"github.com/klexas/ProbeTools/scanner/internal/model"
	"github.com/klexas/ProbeTools/scanner/internal/site"
)

type Crawler struct {
	target          *url.URL
	maxPages        int
	maxDepth        int
	fetcher         fetch.Fetcher
	extractor       *extract.HTMLExtractor
	analyzer        *discovery.Analyzer
	inspectedScript map[string]struct{}
}

type queueEntry struct {
	url      string
	depth    int
	referrer string
}

func NewCrawler(target *url.URL, maxPages int, maxDepth int, fetcher fetch.Fetcher, extractor *extract.HTMLExtractor, analyzer *discovery.Analyzer) *Crawler {
	return &Crawler{
		target:    target,
		maxPages:  maxPages,
		maxDepth:  maxDepth,
		fetcher:   fetcher,
		extractor: extractor,
		analyzer:  analyzer,
		inspectedScript: make(map[string]struct{}),
	}
}

func (c *Crawler) Run(ctx context.Context) ([]model.Request, []model.Page, []model.BackendCall, []string, error) {
	c.inspectedScript = make(map[string]struct{})
	queue := []queueEntry{{url: normalizeForQueue(c.target), depth: 0}}
	visited := make(map[string]struct{}, c.maxPages)
	queuedDepths := map[string]int{normalizeForQueue(c.target): 0}
	requests := make([]model.Request, 0, c.maxPages)
	pages := make([]model.Page, 0, c.maxPages)
	warnings := make([]string, 0)

	for len(queue) > 0 && len(visited) < c.maxPages {
		current := queue[0]
		queue = queue[1:]

		if _, exists := visited[current.url]; exists {
			continue
		}
		visited[current.url] = struct{}{}

		pageURL, err := url.Parse(current.url)
		if err != nil {
			warnings = append(warnings, "skip invalid queued URL "+current.url)
			continue
		}

		result, err := c.fetcher.Get(ctx, pageURL)
		if err != nil {
			requests = append(requests, model.Request{
				Method:   "GET",
				URL:      current.url,
				Host:     pageURL.Host,
				Path:     pageURL.Path,
				Depth:    current.depth,
				Referrer: current.referrer,
				Error:    err.Error(),
			})
			warnings = append(warnings, err.Error())
			continue
		}

		finalURL := normalizeForQueue(result.URL)
		visited[finalURL] = struct{}{}
		queuedDepths[finalURL] = current.depth

		requests = append(requests, model.Request{
			Method:      "GET",
			URL:         finalURL,
			Host:        result.URL.Host,
			Path:        result.URL.Path,
			Depth:       current.depth,
			StatusCode:  result.StatusCode,
			ContentType: result.ContentType,
			Referrer:    current.referrer,
		})

		page := model.Page{
			URL:         finalURL,
			Depth:       current.depth,
			StatusCode:  result.StatusCode,
			ContentType: result.ContentType,
		}

		if result.StatusCode >= 400 {
			pages = append(pages, page)
			continue
		}

		if result.ContentType != "text/html" && result.ContentType != "application/xhtml+xml" {
			pages = append(pages, page)
			continue
		}

		extracted, err := c.extractor.Extract(result.URL, result.Body)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("extract %s: %v", result.URL.String(), err))
			pages = append(pages, page)
			continue
		}

		page.DiscoveredCallCount = c.analyzer.Observe(result.URL, extracted)
		pages = append(pages, page)

		scriptRequests, scriptLinks, scriptWarnings := c.extractLinksFromScripts(ctx, extracted.ScriptRefs, current.depth, finalURL)
		requests = append(requests, scriptRequests...)
		if len(scriptWarnings) > 0 {
			warnings = append(warnings, scriptWarnings...)
		}

		if current.depth >= c.maxDepth {
			continue
		}

		followLinks := append([]extract.Reference{}, extracted.Links...)
		followLinks = append(followLinks, scriptLinks...)

		for _, link := range followLinks {
			if link.URL == nil || !site.SameSiteHost(link.URL.Host, result.URL.Host) {
				continue
			}

			next := normalizeForQueue(link.URL)
			nextDepth := current.depth + 1
			if _, exists := visited[next]; exists {
				continue
			}
			if existingDepth, exists := queuedDepths[next]; exists && existingDepth <= nextDepth {
				continue
			}

			queuedDepths[next] = nextDepth
			queue = append(queue, queueEntry{url: next, depth: nextDepth, referrer: finalURL})
		}
	}

	sort.Slice(requests, func(i, j int) bool {
		if requests[i].Depth == requests[j].Depth {
			if requests[i].URL == requests[j].URL {
				return requests[i].Referrer < requests[j].Referrer
			}
			return requests[i].URL < requests[j].URL
		}
		return requests[i].Depth < requests[j].Depth
	})

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].URL < pages[j].URL
	})

	sort.Strings(warnings)

	return requests, pages, c.analyzer.Finalize(ctx), warnings, nil
}

func (c *Crawler) extractLinksFromScripts(ctx context.Context, scripts []extract.Reference, depth int, referrer string) ([]model.Request, []extract.Reference, []string) {
	if len(scripts) == 0 {
		return nil, nil, nil
	}

	requests := make([]model.Request, 0, len(scripts))
	routes := make([]extract.Reference, 0)
	warnings := make([]string, 0)

	for _, script := range scripts {
		if script.URL == nil || !site.SameSiteHost(script.URL.Host, c.target.Host) {
			continue
		}

		scriptURL := normalizeForQueue(script.URL)
		if _, exists := c.inspectedScript[scriptURL]; exists {
			continue
		}
		c.inspectedScript[scriptURL] = struct{}{}

		result, err := c.fetcher.Get(ctx, script.URL)
		if err != nil {
			requests = append(requests, model.Request{
				Method:   "GET",
				URL:      scriptURL,
				Host:     script.URL.Host,
				Path:     script.URL.Path,
				Depth:    depth,
				Referrer: referrer,
				Error:    err.Error(),
			})
			warnings = append(warnings, err.Error())
			continue
		}

		requests = append(requests, model.Request{
			Method:      "GET",
			URL:         normalizeForQueue(result.URL),
			Host:        result.URL.Host,
			Path:        result.URL.Path,
			Depth:       depth,
			StatusCode:  result.StatusCode,
			ContentType: result.ContentType,
			Referrer:    referrer,
		})

		if !looksLikeScript(result) {
			continue
		}

		routes = append(routes, extract.ExtractRoutesFromScript(result.URL, result.Body)...)
	}

	return requests, routes, warnings
}

func looksLikeScript(result fetch.PageResult) bool {
	if result.ContentType == "application/javascript" || result.ContentType == "text/javascript" {
		return true
	}

	return strings.HasSuffix(strings.ToLower(result.URL.Path), ".js")
}

func normalizeForQueue(target *url.URL) string {
	clone := *target
	clone.Fragment = ""
	if clone.Path == "" {
		clone.Path = "/"
	}
	return clone.String()
}

