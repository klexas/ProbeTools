package sqlscan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WriteReport(report Report, configuredPath string) (string, error) {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		path = defaultReportPath()
	}

	var (
		content []byte
		err     error
	)

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		content, err = json.MarshalIndent(report, "", "  ")
	default:
		content = []byte(generateMarkdown(report))
	}
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

func generateMarkdown(report Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# API SQL Injection Report\n\n")
	if report.Name != "" {
		fmt.Fprintf(&b, "**Target:** %s  \n", report.Name)
	}
	fmt.Fprintf(&b, "**Base URL:** %s  \n", report.BaseURL)
	fmt.Fprintf(&b, "**Generated:** %s\n\n", report.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| Endpoints | Requests | Findings | High | Medium | Low |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %d |\n\n",
		report.Summary.EndpointsScanned,
		report.Summary.RequestsSent,
		report.Summary.Findings,
		report.Summary.High,
		report.Summary.Medium,
		report.Summary.Low,
	)

	fmt.Fprintf(&b, "## Endpoint Baselines\n\n")
	fmt.Fprintf(&b, "| Endpoint | Method | Path | Status | Duration (ms) | Notes |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | ---: | ---: | --- |\n")
	for _, endpoint := range report.Endpoints {
		notes := endpoint.Baseline.BodySnippet
		if endpoint.SkippedReason != "" {
			notes = "Skipped: " + endpoint.SkippedReason
		}
		fmt.Fprintf(&b, "| %s | %s | `%s` | %d | %d | %s |\n",
			endpoint.Name,
			endpoint.Method,
			endpoint.Path,
			endpoint.Baseline.Status,
			endpoint.Baseline.DurationMS,
			escapePipes(notes),
		)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Findings\n\n")
	if len(report.Findings) == 0 {
		b.WriteString("No clear SQL injection indicators were detected with the current payload set.\n\n")
	} else {
		for _, finding := range report.Findings {
			fmt.Fprintf(&b, "### %s / %s (%s)\n\n", finding.Endpoint, finding.Target, finding.Category)
			fmt.Fprintf(&b, "- **Severity:** %s\n", finding.Severity)
			fmt.Fprintf(&b, "- **Confidence:** %s\n", finding.Confidence)
			fmt.Fprintf(&b, "- **Vector:** %s `%s`\n", finding.Location, finding.Target)
			fmt.Fprintf(&b, "- **Payload:** `%s`\n", finding.Payload)
			fmt.Fprintf(&b, "- **Status change:** %d -> %d\n", finding.BaselineStatus, finding.TestStatus)
			fmt.Fprintf(&b, "- **Latency change:** %dms -> %dms\n", finding.BaselineLatencyMS, finding.TestLatencyMS)
			if len(finding.Evidence) > 0 {
				b.WriteString("- **Evidence:**\n")
				for _, item := range finding.Evidence {
					fmt.Fprintf(&b, "  - %s\n", item)
				}
			}
			if len(finding.Recommendations) > 0 {
				b.WriteString("- **Suggested fixes:**\n")
				for _, item := range finding.Recommendations {
					fmt.Fprintf(&b, "  - %s\n", item)
				}
			}
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(&b, "## Hardening Suggestions\n\n")
	for _, suggestion := range report.Suggestions {
		fmt.Fprintf(&b, "- %s\n", suggestion)
	}
	b.WriteString("\n")

	return b.String()
}

func escapePipes(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func defaultReportPath() string {
	return fmt.Sprintf("sqlscan-report-%s.md", time.Now().Format("20060102-150405"))
}
