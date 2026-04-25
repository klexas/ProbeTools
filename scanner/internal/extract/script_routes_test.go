package extract

import (
	"net/url"
	"testing"
)

func TestExtractRoutesFromScriptFindsNavigableRoutes(t *testing.T) {
	base, err := url.Parse("https://example.com/assets/app.js")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	body := []byte(`const routes = ["/creator/new", "/login", "/assets/logo.svg", "/api/trips"];`)
	routes := ExtractRoutesFromScript(base, body)

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	if routes[0].URL.String() != "https://example.com/creator/new" {
		t.Fatalf("unexpected first route: %s", routes[0].URL.String())
	}

	if routes[1].URL.String() != "https://example.com/login" {
		t.Fatalf("unexpected second route: %s", routes[1].URL.String())
	}
}
