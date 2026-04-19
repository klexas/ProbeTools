package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Report struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Name        string           `json:"name"`
	BaseURL     string           `json:"base_url"`
	Summary     Summary          `json:"summary"`
	Endpoints   []EndpointReport `json:"endpoints"`
	Findings    []Finding        `json:"findings"`
	Suggestions []string         `json:"suggestions"`
}

type Summary struct {
	EndpointsScanned int `json:"endpoints_scanned"`
	RequestsSent     int `json:"requests_sent"`
	Findings         int `json:"findings"`
	High             int `json:"high"`
	Medium           int `json:"medium"`
	Low              int `json:"low"`
}

type EndpointReport struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	ExpectedStatus int             `json:"expected_status"`
	Baseline       ResponseSummary `json:"baseline"`
	SkippedReason  string          `json:"skipped_reason,omitempty"`
}

type ResponseSummary struct {
	Status       int    `json:"status"`
	DurationMS   int64  `json:"duration_ms"`
	BodySnippet  string `json:"body_snippet"`
	ContentBytes int    `json:"content_bytes"`
}

type Finding struct {
	ID                string   `json:"id"`
	Endpoint          string   `json:"endpoint"`
	Method            string   `json:"method"`
	Path              string   `json:"path"`
	Location          string   `json:"location"`
	Target            string   `json:"target"`
	Category          string   `json:"category"`
	Payload           string   `json:"payload"`
	Severity          string   `json:"severity"`
	Confidence        string   `json:"confidence"`
	Evidence          []string `json:"evidence"`
	Recommendations   []string `json:"recommendations"`
	BaselineStatus    int      `json:"baseline_status"`
	TestStatus        int      `json:"test_status"`
	BaselineLatencyMS int64    `json:"baseline_latency_ms"`
	TestLatencyMS     int64    `json:"test_latency_ms"`
}

type scanResult struct {
	Status      int
	Duration    time.Duration
	Body        string
	ContentSize int
}

type Scanner struct {
	cfg      Config
	client   *http.Client
	payloads []Payload
}

func RunScan(ctx context.Context, cfg Config) (Report, error) {
	scanner := Scanner{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		payloads: defaultPayloads(),
	}

	report := Report{
		GeneratedAt: time.Now().UTC(),
		Name:        cfg.Name,
		BaseURL:     cfg.BaseURL,
		Endpoints:   make([]EndpointReport, 0, len(cfg.Endpoints)),
	}

	for _, endpoint := range cfg.Endpoints {
		endpointReport, findings, requests, err := scanner.scanEndpoint(ctx, endpoint)
		report.Endpoints = append(report.Endpoints, endpointReport)
		report.Findings = append(report.Findings, findings...)
		report.Summary.RequestsSent += requests
		if err != nil {
			return Report{}, err
		}
	}

	report.Summary.EndpointsScanned = len(report.Endpoints)
	report.Summary.Findings = len(report.Findings)
	for _, finding := range report.Findings {
		switch finding.Severity {
		case "high":
			report.Summary.High++
		case "medium":
			report.Summary.Medium++
		default:
			report.Summary.Low++
		}
	}
	report.Suggestions = collectSuggestions(report.Findings)

	sort.Slice(report.Findings, func(i, j int) bool {
		return report.Findings[i].ID < report.Findings[j].ID
	})

	return report, nil
}

func (s Scanner) scanEndpoint(ctx context.Context, endpoint EndpointConfig) (EndpointReport, []Finding, int, error) {
	requests := 0
	baseline, err := s.send(ctx, endpoint)
	requests++
	endpointReport := EndpointReport{
		Name:           endpoint.Name,
		Method:         endpoint.Method,
		Path:           endpoint.Path,
		ExpectedStatus: endpoint.ExpectedStatus,
	}

	if err != nil {
		endpointReport.SkippedReason = err.Error()
		return endpointReport, nil, requests, nil
	}

	endpointReport.Baseline = summarizeResult(baseline)

	findings := make([]Finding, 0)

	for _, target := range endpoint.Targets {
		pairResults := map[string]map[string]scanResult{}

		for _, payload := range s.payloads {
			mutated := cloneEndpoint(endpoint)
			if err := injectPayload(&mutated, target, payload.Value); err != nil {
				findings = append(findings, newUnsupportedTargetFinding(endpoint, target, err.Error()))
				continue
			}

			result, err := s.send(ctx, mutated)
			requests++
			if err != nil {
				findings = append(findings, Finding{
					ID:          findingID(endpoint, target, payload.Name, "transport-error"),
					Endpoint:    endpoint.Name,
					Method:      endpoint.Method,
					Path:        endpoint.Path,
					Location:    target.Location,
					Target:      target.Name,
					Category:    "request-failure",
					Payload:     payload.Name,
					Severity:    "low",
					Confidence:  "low",
					Evidence:    []string{fmt.Sprintf("request failed after payload %q: %v", payload.Name, err)},
					Recommendations: []string{
						"Review input validation and upstream error handling for this parameter.",
					},
					BaselineStatus:    baseline.Status,
					TestStatus:        0,
					BaselineLatencyMS: baseline.Duration.Milliseconds(),
					TestLatencyMS:     0,
				})
				continue
			}

			findings = append(findings, evaluateDirectFindings(endpoint, target, payload, baseline, result, s.cfg.DelayThresholdMS)...)

			if payload.BooleanPair != "" {
				if pairResults[payload.BooleanPair] == nil {
					pairResults[payload.BooleanPair] = map[string]scanResult{}
				}
				pairResults[payload.BooleanPair][payload.BooleanMode] = result
			}
		}

		for pairName, variants := range pairResults {
			trueResult, okTrue := variants["true"]
			falseResult, okFalse := variants["false"]
			if !okTrue || !okFalse {
				continue
			}
			if finding := evaluateBooleanPair(endpoint, target, pairName, baseline, trueResult, falseResult); finding != nil {
				findings = append(findings, *finding)
			}
		}
	}

	return endpointReport, dedupeFindings(findings), requests, nil
}

func (s Scanner) send(ctx context.Context, endpoint EndpointConfig) (scanResult, error) {
	requestURL, body, headers, err := s.buildRequest(endpoint)
	if err != nil {
		return scanResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, endpoint.Method, requestURL, body)
	if err != nil {
		return scanResult{}, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		return scanResult{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return scanResult{}, err
	}

	return scanResult{
		Status:      resp.StatusCode,
		Duration:    time.Since(start),
		Body:        string(responseBody),
		ContentSize: len(responseBody),
	}, nil
}

func (s Scanner) buildRequest(endpoint EndpointConfig) (string, io.Reader, map[string]string, error) {
	renderedPath := endpoint.Path
	for key, value := range endpoint.PathParams {
		renderedPath = strings.ReplaceAll(renderedPath, "{"+key+"}", url.PathEscape(value))
	}

	u, err := url.Parse(s.cfg.BaseURL + renderedPath)
	if err != nil {
		return "", nil, nil, err
	}

	query := u.Query()
	for key, value := range endpoint.Query {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()

	headers := map[string]string{}
	for key, value := range s.cfg.DefaultHeaders {
		headers[key] = value
	}
	for key, value := range endpoint.Headers {
		headers[key] = value
	}

	var body io.Reader
	if len(endpoint.JSONBody) > 0 {
		encoded, err := json.Marshal(endpoint.JSONBody)
		if err != nil {
			return "", nil, nil, err
		}
		body = bytes.NewReader(encoded)
		if _, exists := headers["Content-Type"]; !exists {
			headers["Content-Type"] = "application/json"
		}
	}

	return u.String(), body, headers, nil
}

func cloneEndpoint(endpoint EndpointConfig) EndpointConfig {
	clone := endpoint
	clone.Query = cloneStringMap(endpoint.Query)
	clone.Headers = cloneStringMap(endpoint.Headers)
	clone.PathParams = cloneStringMap(endpoint.PathParams)
	clone.JSONBody = cloneAnyMap(endpoint.JSONBody)
	clone.Targets = append([]TargetConfig(nil), endpoint.Targets...)
	return clone
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func injectPayload(endpoint *EndpointConfig, target TargetConfig, payload string) error {
	switch target.Location {
	case "query":
		base, ok := endpoint.Query[target.Name]
		if !ok {
			return fmt.Errorf("query parameter %q not found", target.Name)
		}
		endpoint.Query[target.Name] = base + payload
	case "path":
		base, ok := endpoint.PathParams[target.Name]
		if !ok {
			return fmt.Errorf("path parameter %q not found", target.Name)
		}
		endpoint.PathParams[target.Name] = base + payload
	case "json":
		base, ok := endpoint.JSONBody[target.Name]
		if !ok {
			return fmt.Errorf("json field %q not found", target.Name)
		}
		endpoint.JSONBody[target.Name] = fmt.Sprint(base) + payload
	case "header":
		base, ok := endpoint.Headers[target.Name]
		if !ok {
			base = ""
		}
		endpoint.Headers[target.Name] = base + payload
	default:
		return fmt.Errorf("unsupported target location %q", target.Location)
	}

	return nil
}

func evaluateDirectFindings(endpoint EndpointConfig, target TargetConfig, payload Payload, baseline, result scanResult, delayThresholdMS int) []Finding {
	findings := make([]Finding, 0, 3)
	bodyLower := strings.ToLower(result.Body)

	if marker := matchingSQLMarker(bodyLower); marker != "" {
		findings = append(findings, Finding{
			ID:          findingID(endpoint, target, payload.Name, "sql-error"),
			Endpoint:    endpoint.Name,
			Method:      endpoint.Method,
			Path:        endpoint.Path,
			Location:    target.Location,
			Target:      target.Name,
			Category:    payload.Category,
			Payload:     payload.Name,
			Severity:    "high",
			Confidence:  "high",
			Evidence:    []string{fmt.Sprintf("response contains SQL error marker %q", marker), statusEvidence(baseline, result)},
			Recommendations: recommendationsFor(payload.Category, true),
			BaselineStatus:    baseline.Status,
			TestStatus:        result.Status,
			BaselineLatencyMS: baseline.Duration.Milliseconds(),
			TestLatencyMS:     result.Duration.Milliseconds(),
		})
	}

	if statusShiftLooksSuspicious(baseline.Status, result.Status) {
		findings = append(findings, Finding{
			ID:          findingID(endpoint, target, payload.Name, "status-shift"),
			Endpoint:    endpoint.Name,
			Method:      endpoint.Method,
			Path:        endpoint.Path,
			Location:    target.Location,
			Target:      target.Name,
			Category:    payload.Category,
			Payload:     payload.Name,
			Severity:    "medium",
			Confidence:  "medium",
			Evidence:    []string{statusEvidence(baseline, result), excerptEvidence(result.Body)},
			Recommendations: recommendationsFor(payload.Category, false),
			BaselineStatus:    baseline.Status,
			TestStatus:        result.Status,
			BaselineLatencyMS: baseline.Duration.Milliseconds(),
			TestLatencyMS:     result.Duration.Milliseconds(),
		})
	}

	if payload.ExpectedDelay > 0 {
		delta := result.Duration - baseline.Duration
		if delta >= maxDuration(time.Duration(delayThresholdMS)*time.Millisecond, payload.ExpectedDelay-150*time.Millisecond) {
			findings = append(findings, Finding{
				ID:          findingID(endpoint, target, payload.Name, "time-delay"),
				Endpoint:    endpoint.Name,
				Method:      endpoint.Method,
				Path:        endpoint.Path,
				Location:    target.Location,
				Target:      target.Name,
				Category:    payload.Category,
				Payload:     payload.Name,
				Severity:    "high",
				Confidence:  "medium",
				Evidence: []string{
					fmt.Sprintf("baseline %dms vs injected %dms", baseline.Duration.Milliseconds(), result.Duration.Milliseconds()),
				},
				Recommendations: recommendationsFor(payload.Category, false),
				BaselineStatus:    baseline.Status,
				TestStatus:        result.Status,
				BaselineLatencyMS: baseline.Duration.Milliseconds(),
				TestLatencyMS:     result.Duration.Milliseconds(),
			})
		}
	}

	return findings
}

func evaluateBooleanPair(endpoint EndpointConfig, target TargetConfig, pairName string, baseline, trueResult, falseResult scanResult) *Finding {
	contentDelta := abs(trueResult.ContentSize - falseResult.ContentSize)
	statusDiff := trueResult.Status != falseResult.Status
	bodyDiff := normalizedBody(trueResult.Body) != normalizedBody(falseResult.Body)

	if !statusDiff && !bodyDiff && contentDelta < 24 {
		return nil
	}

	return &Finding{
		ID:          findingID(endpoint, target, pairName, "boolean-delta"),
		Endpoint:    endpoint.Name,
		Method:      endpoint.Method,
		Path:        endpoint.Path,
		Location:    target.Location,
		Target:      target.Name,
		Category:    "boolean-based",
		Payload:     pairName,
		Severity:    "high",
		Confidence:  "medium",
		Evidence: []string{
			fmt.Sprintf("boolean probes produced different responses: true=%d (%d bytes), false=%d (%d bytes), baseline=%d (%d bytes)",
				trueResult.Status, trueResult.ContentSize, falseResult.Status, falseResult.ContentSize, baseline.Status, baseline.ContentSize),
		},
		Recommendations: recommendationsFor("boolean-based", false),
		BaselineStatus:    baseline.Status,
		TestStatus:        trueResult.Status,
		BaselineLatencyMS: baseline.Duration.Milliseconds(),
		TestLatencyMS:     trueResult.Duration.Milliseconds(),
	}
}

func dedupeFindings(findings []Finding) []Finding {
	seen := map[string]bool{}
	out := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		if seen[finding.ID] {
			continue
		}
		seen[finding.ID] = true
		out = append(out, finding)
	}
	return out
}

func summarizeResult(result scanResult) ResponseSummary {
	return ResponseSummary{
		Status:       result.Status,
		DurationMS:   result.Duration.Milliseconds(),
		BodySnippet:  trimSnippet(result.Body),
		ContentBytes: result.ContentSize,
	}
}

func matchingSQLMarker(body string) string {
	for _, marker := range []string{
		"sql syntax",
		"sqlstate",
		"mysql",
		"postgresql",
		"pg_query",
		"sqlite",
		"odbc",
		"unclosed quotation mark",
		"quoted string not properly terminated",
		"near \"select\"",
		"syntax error at or near",
		"warning: sqlite",
	} {
		if strings.Contains(body, marker) {
			return marker
		}
	}
	return ""
}

func statusShiftLooksSuspicious(baselineStatus, testStatus int) bool {
	if baselineStatus == 0 || testStatus == 0 {
		return false
	}
	if baselineStatus >= 500 || testStatus >= 500 {
		return baselineStatus != testStatus
	}
	if baselineStatus < 400 && testStatus >= 500 {
		return true
	}
	return baselineStatus/100 != testStatus/100 && abs(baselineStatus-testStatus) >= 100
}

func recommendationsFor(category string, errorLeak bool) []string {
	recommendations := []string{
		"Use parameterized queries or prepared statements for every database call that uses client input.",
		"Enforce strict server-side validation and type constraints on user-controlled fields before they reach query builders.",
	}

	switch category {
	case "time-based", "boolean-based", "union-based":
		recommendations = append(recommendations,
			"Audit any dynamic WHERE, ORDER BY, and search-filter construction for string concatenation.",
			"Run database queries with least-privilege credentials and disable stacked queries where possible.",
		)
	case "error-based":
		recommendations = append(recommendations,
			"Return generic API errors to clients and log detailed SQL diagnostics only on the server side.",
		)
	}

	if errorLeak {
		recommendations = append(recommendations,
			"Remove database product names, stack traces, and raw query errors from public responses.",
		)
	}

	return uniqueStrings(recommendations)
}

func collectSuggestions(findings []Finding) []string {
	all := make([]string, 0)
	for _, finding := range findings {
		all = append(all, finding.Recommendations...)
	}
	if len(all) == 0 {
		all = append(all,
			"Keep using parameterized queries, strict input validation, and generic error messages for database-backed endpoints.",
		)
	}
	return uniqueStrings(all)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func newUnsupportedTargetFinding(endpoint EndpointConfig, target TargetConfig, detail string) Finding {
	return Finding{
		ID:          findingID(endpoint, target, "target", "unsupported"),
		Endpoint:    endpoint.Name,
		Method:      endpoint.Method,
		Path:        endpoint.Path,
		Location:    target.Location,
		Target:      target.Name,
		Category:    "configuration",
		Payload:     "n/a",
		Severity:    "low",
		Confidence:  "high",
		Evidence:    []string{detail},
		Recommendations: []string{
			"Adjust the endpoint target configuration so each target points to an existing query, path, header, or JSON field.",
		},
	}
}

func findingID(endpoint EndpointConfig, target TargetConfig, payloadName, suffix string) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		endpoint.Name,
		endpoint.Method,
		endpoint.Path,
		target.Location,
		target.Name,
		payloadName,
		suffix,
	}, "|")))
	return hex.EncodeToString(sum[:8])
}

func trimSnippet(body string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
	if len(cleaned) > 180 {
		return cleaned[:180] + "..."
	}
	return cleaned
}

func excerptEvidence(body string) string {
	snippet := trimSnippet(body)
	if snippet == "" {
		return "response body was empty"
	}
	return fmt.Sprintf("response excerpt: %q", snippet)
}

func normalizedBody(body string) string {
	body = strings.ToLower(body)
	body = strings.Join(strings.Fields(body), " ")
	if len(body) > 250 {
		return body[:250]
	}
	return body
}

func statusEvidence(baseline, result scanResult) string {
	return fmt.Sprintf("baseline status %d vs injected status %d", baseline.Status, result.Status)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
