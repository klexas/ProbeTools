package extract

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/klexas/ProbeTools/scanner/internal/model"
	"golang.org/x/net/html"
)

var inlineCandidatePattern = regexp.MustCompile(`(?i)(https?://[^\s"'<>]+|/(?:api|graphql|ajax|rest|rpc|services?|auth|session|token|login|logout|v[0-9]+)[^\s"'<>]*)`)

type Reference struct {
	URL    *url.URL
	Source model.Source
	Method string
}

type Extracted struct {
	Links      []Reference
	Candidates []Reference
}

type HTMLExtractor struct{}

func NewHTMLExtractor() *HTMLExtractor {
	return &HTMLExtractor{}
}

func (e *HTMLExtractor) Extract(base *url.URL, body []byte) (Extracted, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Extracted{}, fmt.Errorf("parse html: %w", err)
	}

	extracted := Extracted{}

	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "a":
				if resolved := resolveURL(base, attr(node, "href")); resolved != nil {
					ref := Reference{URL: resolved, Source: model.SourceLink, Method: "GET"}
					extracted.Links = append(extracted.Links, ref)
					extracted.Candidates = append(extracted.Candidates, ref)
				}
			case "form":
				action := attr(node, "action")
				if action == "" {
					action = base.String()
				}
				if resolved := resolveURL(base, action); resolved != nil {
					method := strings.ToUpper(strings.TrimSpace(attr(node, "method")))
					if method == "" {
						method = "GET"
					}
					extracted.Candidates = append(extracted.Candidates, Reference{
						URL:    resolved,
						Source: model.SourceForm,
						Method: method,
					})
				}
			case "script":
				if resolved := resolveURL(base, attr(node, "src")); resolved != nil {
					extracted.Candidates = append(extracted.Candidates, Reference{
						URL:    resolved,
						Source: model.SourceScriptRef,
						Method: "GET",
					})
				}
			case "img", "iframe":
				if resolved := resolveURL(base, attr(node, "src")); resolved != nil {
					extracted.Candidates = append(extracted.Candidates, Reference{
						URL:    resolved,
						Source: model.SourceResourceRef,
						Method: "GET",
					})
				}
			case "link":
				if resolved := resolveURL(base, attr(node, "href")); resolved != nil {
					extracted.Candidates = append(extracted.Candidates, Reference{
						URL:    resolved,
						Source: model.SourceResourceRef,
						Method: "GET",
					})
				}
			}
		}

		if node.Type == html.TextNode && node.Parent != nil && strings.EqualFold(node.Parent.Data, "script") {
			for _, match := range inlineCandidatePattern.FindAllString(node.Data, -1) {
				if resolved := resolveURL(base, match); resolved != nil {
					extracted.Candidates = append(extracted.Candidates, Reference{
						URL:    resolved,
						Source: model.SourceInlineScript,
						Method: "GET",
					})
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}

	visit(doc)

	extracted.Links = uniqueReferences(extracted.Links)
	extracted.Candidates = uniqueReferences(extracted.Candidates)

	return extracted, nil
}

func attr(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return strings.TrimSpace(attribute.Val)
		}
	}

	return ""
}

func resolveURL(base *url.URL, raw string) *url.URL {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(strings.ToLower(raw), "javascript:") || strings.HasPrefix(strings.ToLower(raw), "mailto:") {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return nil
	}

	resolved.Fragment = ""
	return resolved
}

func uniqueReferences(values []Reference) []Reference {
	seen := make(map[string]Reference, len(values))
	for _, value := range values {
		if value.URL == nil {
			continue
		}

		key := value.Method + "|" + string(value.Source) + "|" + value.URL.String()
		if _, exists := seen[key]; !exists {
			seen[key] = value
		}
	}

	result := make([]Reference, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].URL.String() == result[j].URL.String() {
			if result[i].Source == result[j].Source {
				return result[i].Method < result[j].Method
			}
			return result[i].Source < result[j].Source
		}
		return result[i].URL.String() < result[j].URL.String()
	})

	return result
}
