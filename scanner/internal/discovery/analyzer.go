package discovery

import (
	"context"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/klexas/ProbeTools/scanner/internal/extract"
	"github.com/klexas/ProbeTools/scanner/internal/model"
	"github.com/klexas/ProbeTools/scanner/internal/probe"
	"github.com/klexas/ProbeTools/scanner/internal/site"
)

var backendMarkers = []string{
	"/api/",
	"/graphql",
	"/ajax",
	"/rest",
	"/rpc",
	"/service",
	"/services",
	"/backend",
	"/auth",
	"/login",
	"/logout",
	"/token",
	"/session",
	"/upload",
	"/webhook",
}

var backendExtensions = []string{
	".php",
	".asp",
	".aspx",
	".ashx",
	".cgi",
	".do",
	".action",
	".json",
}

var staticExtensions = []string{
	".css",
	".gif",
	".ico",
	".jpeg",
	".jpg",
	".js",
	".map",
	".png",
	".svg",
	".txt",
	".webp",
	".woff",
	".woff2",
}

type Analyzer struct {
	targetHost  string
	enableProbe bool
	prober      probe.Prober
	calls       map[string]*model.BackendCall
}

func NewAnalyzer(target *url.URL, enableProbe bool, prober probe.Prober) *Analyzer {
	return &Analyzer{
		targetHost:  strings.ToLower(target.Host),
		enableProbe: enableProbe,
		prober:      prober,
		calls:       make(map[string]*model.BackendCall),
	}
}

func (a *Analyzer) Observe(pageURL *url.URL, extracted extract.Extracted) int {
	matches := 0

	for _, candidate := range extracted.Candidates {
		if candidate.URL == nil || !site.SameSiteHost(candidate.URL.Host, a.targetHost) {
			continue
		}

		reasons, include := shouldInclude(candidate)
		if !include {
			continue
		}

		key := candidate.URL.String()
		call, exists := a.calls[key]
		if !exists {
			call = &model.BackendCall{
				URL:  key,
				Host: candidate.URL.Host,
				Path: candidate.URL.Path,
			}
			a.calls[key] = call
		}

		addMethod(call, candidate.Method)
		addSource(call, candidate.Source)
		addString(&call.EvidencePages, pageURL.String())
		for _, reason := range reasons {
			addString(&call.Reasons, reason)
		}
		matches++
	}

	return matches
}

func (a *Analyzer) Finalize(ctx context.Context) []model.BackendCall {
	keys := make([]string, 0, len(a.calls))
	for key := range a.calls {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]model.BackendCall, 0, len(keys))
	for _, key := range keys {
		call := a.calls[key]
		sort.Strings(call.Methods)
		sort.Slice(call.Sources, func(i, j int) bool {
			return call.Sources[i] < call.Sources[j]
		})
		sort.Strings(call.EvidencePages)
		sort.Strings(call.Reasons)

		if a.enableProbe && a.prober != nil && isProbeSafe(call.Methods) {
			call.Probe = a.prober.Probe(ctx, mustParseURL(call.URL))
		}

		result = append(result, *call)
	}

	return result
}

func shouldInclude(candidate extract.Reference) ([]string, bool) {
	if candidate.Source == model.SourceForm {
		return []string{"form action"}, true
	}

	lowerPath := strings.ToLower(candidate.URL.Path)
	if lowerPath == "" {
		lowerPath = "/"
	}

	ext := strings.ToLower(path.Ext(lowerPath))
	for _, value := range staticExtensions {
		if ext == value {
			return nil, false
		}
	}

	reasons := make([]string, 0, 3)

	for _, marker := range backendMarkers {
		if strings.Contains(lowerPath, marker) {
			reasons = append(reasons, "path contains "+marker)
		}
	}

	for _, value := range backendExtensions {
		if ext == value {
			reasons = append(reasons, "dynamic extension "+value)
		}
	}

	if candidate.URL.RawQuery != "" {
		reasons = append(reasons, "has query string")
	}

	if candidate.Source == model.SourceInlineScript {
		reasons = append(reasons, "referenced in inline script")
	}

	if candidate.Source == model.SourceScriptRef && len(reasons) > 0 {
		reasons = append(reasons, "referenced by script tag")
	}

	if len(reasons) == 0 {
		return nil, false
	}

	return reasons, true
}

func isProbeSafe(methods []string) bool {
	for _, method := range methods {
		if method != "" && method != "GET" && method != "HEAD" {
			return false
		}
	}

	return true
}

func addMethod(call *model.BackendCall, method string) {
	if method == "" {
		method = "GET"
	}
	addString(&call.Methods, method)
}

func addSource(call *model.BackendCall, source model.Source) {
	for _, existing := range call.Sources {
		if existing == source {
			return
		}
	}

	call.Sources = append(call.Sources, source)
}

func addString(values *[]string, value string) {
	for _, existing := range *values {
		if existing == value {
			return
		}
	}

	*values = append(*values, value)
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}

	return parsed
}
