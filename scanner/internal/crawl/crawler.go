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
)

type Crawler struct {
	target    *url.URL
	maxPages  int
	fetcher   fetch.Fetcher
	extractor *extract.HTMLExtractor
	analyzer  *discovery.Analyzer
}

func NewCrawler(target *url.URL, maxPages int, fetcher fetch.Fetcher, extractor *extract.HTMLExtractor, analyzer *discovery.Analyzer) *Crawler {
	return &Crawler{
		target:    target,
		maxPages:  maxPages,
		fetcher:   fetcher,
		extractor: extractor,
		analyzer:  analyzer,
	}
}

func (c *Crawler) Run(ctx context.Context) ([]model.Page, []model.BackendCall, []string, error) {
	queue := []string{normalizeForQueue(c.target)}
	visited := make(map[string]struct{}, c.maxPages)
	pages := make([]model.Page, 0, c.maxPages)
	warnings := make([]string, 0)

	for len(queue) > 0 && len(visited) < c.maxPages {
		current := queue[0]
		queue = queue[1:]

		if _, exists := visited[current]; exists {
			continue
		}
		visited[current] = struct{}{}

		pageURL, err := url.Parse(current)
		if err != nil {
			warnings = append(warnings, "skip invalid queued URL "+current)
			continue
		}

		result, err := c.fetcher.Get(ctx, pageURL)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}

		page := model.Page{
			URL:         result.URL.String(),
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

		for _, link := range extracted.Links {
			if link.URL == nil || !sameHost(link.URL.Host, c.target.Host) {
				continue
			}

			next := normalizeForQueue(link.URL)
			if _, exists := visited[next]; exists {
				continue
			}
			queue = append(queue, next)
		}
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].URL < pages[j].URL
	})

	sort.Strings(warnings)

	return pages, c.analyzer.Finalize(ctx), warnings, nil
}

func normalizeForQueue(target *url.URL) string {
	clone := *target
	clone.Fragment = ""
	if clone.Path == "" {
		clone.Path = "/"
	}
	return clone.String()
}

func sameHost(left, right string) bool {
	return strings.EqualFold(left, right)
}
