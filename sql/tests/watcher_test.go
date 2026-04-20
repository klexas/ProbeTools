package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apisqlscan/sqlscan"
)

func TestProxyRecorderCapturesHTTPRequests(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "watched-config.json")
	recorder, err := sqlscan.NewProxyRecorder(sqlscan.WatcherOptions{
		Domain:     targetURL.Hostname(),
		OutputPath: outputPath,
	})
	if err != nil {
		t.Fatalf("NewProxyRecorder error: %v", err)
	}

	proxy := httptest.NewServer(recorder)
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest(http.MethodPost, target.URL+"/users/123?q=alice", strings.NewReader(`{"email":"alice@example.com"}`))
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "demo")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do error: %v", err)
	}
	resp.Body.Close()

	if err := recorder.FlushConfig(); err != nil {
		t.Fatalf("FlushConfig error: %v", err)
	}

	cfg := readConfigFile(t, outputPath)
	if cfg.BaseURL == "" {
		t.Fatalf("expected base_url to be set")
	}
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}

	endpoint := cfg.Endpoints[0]
	if endpoint.Path != "/users/{param1}" {
		t.Fatalf("expected templated path, got %q", endpoint.Path)
	}
	if endpoint.Query["q"] != "alice" {
		t.Fatalf("expected query q to be recorded")
	}
	if endpoint.PathParams["param1"] != "123" {
		t.Fatalf("expected path param sample to be recorded")
	}
	if endpoint.JSONBody["email"] != "alice@example.com" {
		t.Fatalf("expected json body to be captured")
	}
	if endpoint.Headers["X-Tenant-Id"] != "demo" {
		t.Fatalf("expected custom header to be captured")
	}
}

func TestProxyRecorderIgnoresOtherDomains(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	outputPath := filepath.Join(t.TempDir(), "watched-config.json")
	recorder, err := sqlscan.NewProxyRecorder(sqlscan.WatcherOptions{
		Domain:     "api.example.test",
		OutputPath: outputPath,
	})
	if err != nil {
		t.Fatalf("NewProxyRecorder error: %v", err)
	}

	proxy := httptest.NewServer(recorder)
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(target.URL + "/health")
	if err != nil {
		t.Fatalf("client.Get error: %v", err)
	}
	resp.Body.Close()

	cfg := readConfigFile(t, outputPath)
	if len(cfg.Endpoints) != 0 {
		t.Fatalf("expected no recorded endpoints, got %d", len(cfg.Endpoints))
	}
}

func readConfigFile(t *testing.T, path string) sqlscan.Config {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	var cfg sqlscan.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	return cfg
}
