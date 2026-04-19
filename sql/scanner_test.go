package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunScanDetectsSQLSignals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("q")
		switch {
		case strings.Contains(value, "SLEEP") || strings.Contains(value, "pg_sleep") || strings.Contains(value, "WAITFOR"):
			time.Sleep(1100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"delayed":true}`))
		case strings.Contains(value, "' OR '1'='1'--"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"rows":["admin","user"]}`))
		case strings.Contains(value, "' OR '1'='2'--"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"rows":[]}`))
		case strings.Contains(value, "'"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`sql syntax error near "'"`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"rows":["user"]}`))
		}
	}))
	defer server.Close()

	cfg := Config{
		Name:             "test-api",
		BaseURL:          server.URL,
		TimeoutSeconds:   3,
		DelayThresholdMS: 800,
		ReportPath:       "test-report.md",
		Endpoints: []EndpointConfig{
			{
				Name:   "search",
				Method: "GET",
				Path:   "/users",
				Query: map[string]string{
					"q": "user",
				},
				Targets: []TargetConfig{
					{Name: "q", Location: "query"},
				},
			},
		},
	}

	report, err := RunScan(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunScan returned error: %v", err)
	}

	if report.Summary.Findings == 0 {
		t.Fatalf("expected findings, got none")
	}

	assertHasCategory(t, report.Findings, "error-based")
	assertHasCategory(t, report.Findings, "boolean-based")
	assertHasCategory(t, report.Findings, "time-based")
}

func TestGenerateMarkdownIncludesSuggestions(t *testing.T) {
	report := Report{
		GeneratedAt: time.Unix(0, 0).UTC(),
		BaseURL:     "https://api.example.test",
		Summary: Summary{
			EndpointsScanned: 1,
			RequestsSent:     5,
			Findings:         1,
			High:             1,
		},
		Endpoints: []EndpointReport{
			{
				Name:   "login",
				Method: "POST",
				Path:   "/login",
				Baseline: ResponseSummary{
					Status:     200,
					DurationMS: 42,
				},
			},
		},
		Findings: []Finding{
			{
				Endpoint:        "login",
				Target:          "email",
				Category:        "error-based",
				Payload:         "quote-break",
				Severity:        "high",
				Confidence:      "high",
				BaselineStatus:  200,
				TestStatus:      500,
				Recommendations: []string{"Use parameterized queries."},
			},
		},
		Suggestions: []string{"Use parameterized queries."},
	}

	markdown := generateMarkdown(report)
	if !strings.Contains(markdown, "## Hardening Suggestions") {
		t.Fatalf("expected hardening section in markdown")
	}
	if !strings.Contains(markdown, "Use parameterized queries.") {
		t.Fatalf("expected suggestion in markdown")
	}
}

func assertHasCategory(t *testing.T, findings []Finding, category string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Category == category {
			return
		}
	}
	t.Fatalf("expected category %q in findings", category)
}
