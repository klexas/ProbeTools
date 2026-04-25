package extract

import (
	"net/url"
	"testing"

	"github.com/klexas/ProbeTools/scanner/internal/model"
)

func TestHTMLExtractorExtractsLinksFormsAndInlineCandidates(t *testing.T) {
	base, err := url.Parse("https://example.com/start")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	body := []byte(`
		<html>
			<body>
				<a href="/about">About</a>
				<form method="post" action="/login"></form>
				<script>
					fetch("/api/users");
				</script>
			</body>
		</html>
	`)

	extractor := NewHTMLExtractor()
	extracted, err := extractor.Extract(base, body)
	if err != nil {
		t.Fatalf("extract html: %v", err)
	}

	if len(extracted.Links) != 1 {
		t.Fatalf("expected 1 crawled link, got %d", len(extracted.Links))
	}

	if len(extracted.ScriptRefs) != 0 {
		t.Fatalf("expected no script refs, got %d", len(extracted.ScriptRefs))
	}

	if extracted.Links[0].URL.String() != "https://example.com/about" {
		t.Fatalf("unexpected link url: %s", extracted.Links[0].URL.String())
	}

	var foundForm bool
	var foundInline bool
	for _, candidate := range extracted.Candidates {
		switch {
		case candidate.Source == model.SourceForm && candidate.URL.String() == "https://example.com/login" && candidate.Method == "POST":
			foundForm = true
		case candidate.Source == model.SourceInlineScript && candidate.URL.String() == "https://example.com/api/users":
			foundInline = true
		}
	}

	if !foundForm {
		t.Fatal("expected to find POST form action candidate")
	}

	if !foundInline {
		t.Fatal("expected to find inline script candidate")
	}
}

func TestHTMLExtractorCollectsScriptRefs(t *testing.T) {
	base, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	body := []byte(`
		<html>
			<head>
				<script src="/assets/app.js"></script>
				<link rel="modulepreload" href="/assets/chunk.js" />
			</head>
		</html>
	`)

	extractor := NewHTMLExtractor()
	extracted, err := extractor.Extract(base, body)
	if err != nil {
		t.Fatalf("extract html: %v", err)
	}

	if len(extracted.ScriptRefs) != 2 {
		t.Fatalf("expected 2 script refs, got %d", len(extracted.ScriptRefs))
	}
}
