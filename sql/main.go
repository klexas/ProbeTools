package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		configPath   string
		reportPath   string
		timeout      int
		delayMS      int
		showHelpOnly bool
	)

	flag.StringVar(&configPath, "config", "", "Path to the JSON configuration file")
	flag.StringVar(&reportPath, "report", "", "Optional output path for the generated report (.md or .json)")
	flag.IntVar(&timeout, "timeout", 0, "Optional per-request timeout override in seconds")
	flag.IntVar(&delayMS, "delay-threshold-ms", 0, "Optional time-based detection threshold override in milliseconds")
	flag.BoolVar(&showHelpOnly, "help", false, "Show usage information")
	flag.Parse()

	if showHelpOnly {
		usage()
		return
	}

	if strings.TrimSpace(configPath) == "" {
		usage()
		fmt.Fprintln(os.Stderr, "\nerror: -config is required")
		os.Exit(2)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if timeout > 0 {
		cfg.TimeoutSeconds = timeout
	}
	if delayMS > 0 {
		cfg.DelayThresholdMS = delayMS
	}
	if strings.TrimSpace(reportPath) != "" {
		cfg.ReportPath = reportPath
	}

	report, err := RunScan(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	outputPath, err := WriteReport(report, cfg.ReportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Scan complete: %d findings across %d endpoints. Report: %s\n",
		report.Summary.Findings,
		report.Summary.EndpointsScanned,
		outputPath,
	)

	if len(report.Suggestions) > 0 {
		fmt.Println("Top fixes:")
		for _, suggestion := range report.Suggestions {
			fmt.Printf("- %s\n", suggestion)
		}
	}
}

func usage() {
	fmt.Println(`API SQLi Scanner

Usage:
  go run . -config targets.example.json
  go run . -config targets.example.json -report results.md

The tool sends baseline and mutated requests to public API endpoints, looks for
SQL injection indicators, and writes a markdown or JSON report depending on the
report file extension.`)
}

func defaultReportPath() string {
	return fmt.Sprintf("sqlscan-report-%s.md", time.Now().Format("20060102-150405"))
}
