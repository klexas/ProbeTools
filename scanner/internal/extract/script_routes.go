package extract

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/klexas/ProbeTools/scanner/internal/model"
)

var routePattern = regexp.MustCompile(`["'\x60](\/[A-Za-z0-9][A-Za-z0-9\-._~\/]*)["'\x60]`)

var staticRouteExtensions = map[string]struct{}{
	".avif":  {},
	".bmp":   {},
	".css":   {},
	".gif":   {},
	".ico":   {},
	".jpeg":  {},
	".jpg":   {},
	".js":    {},
	".json":  {},
	".map":   {},
	".mp4":   {},
	".pdf":   {},
	".png":   {},
	".svg":   {},
	".txt":   {},
	".webp":  {},
	".woff":  {},
	".woff2": {},
}

func ExtractRoutesFromScript(base *url.URL, body []byte) []Reference {
	matches := routePattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	routes := make([]Reference, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		raw := strings.TrimSpace(string(match[1]))
		if !shouldTreatAsRoute(raw) {
			continue
		}

		resolved := resolveURL(base, raw)
		if resolved == nil {
			continue
		}

		routes = append(routes, Reference{
			URL:    resolved,
			Source: model.SourceLink,
			Method: "GET",
		})
	}

	routes = uniqueReferences(routes)
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].URL.String() < routes[j].URL.String()
	})

	return routes
}

func shouldTreatAsRoute(raw string) bool {
	if raw == "/" {
		return true
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "//") ||
		strings.HasPrefix(lower, "/api/") ||
		strings.HasPrefix(lower, "/assets/") ||
		strings.HasPrefix(lower, "/static/") {
		return false
	}

	ext := strings.ToLower(path.Ext(lower))
	if _, exists := staticRouteExtensions[ext]; exists {
		return false
	}

	if strings.ContainsAny(lower, "{}*+$") {
		return false
	}

	return true
}
