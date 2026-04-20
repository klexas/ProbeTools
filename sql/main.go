package main

import (
	"apisqlscan/sqlscan"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	var (
		configPath   string
		reportPath   string
		timeout      int
		delayMS      int
		watchDomain  string
		watchListen  string
		watchOutput  string
		showHelpOnly bool
	)

	flag.StringVar(&configPath, "config", "", "Path to the JSON configuration file")
	flag.StringVar(&reportPath, "report", "", "Optional output path for the generated report (.md or .json)")
	flag.IntVar(&timeout, "timeout", 0, "Optional per-request timeout override in seconds")
	flag.IntVar(&delayMS, "delay-threshold-ms", 0, "Optional time-based detection threshold override in milliseconds")
	flag.StringVar(&watchDomain, "watch-domain", "", "Run as a local proxy recorder for a specific domain")
	flag.StringVar(&watchListen, "watch-listen", "127.0.0.1:8088", "Listen address for proxy recorder mode")
	flag.StringVar(&watchOutput, "watch-output", "watched-config.json", "Output path for generated config in watcher mode")
	flag.BoolVar(&showHelpOnly, "help", false, "Show usage information")
	flag.Parse()

	if showHelpOnly {
		usage()
		return
	}

	if strings.TrimSpace(watchDomain) != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		recorder, err := sqlscan.NewProxyRecorder(sqlscan.WatcherOptions{
			Domain:     watchDomain,
			ListenAddr: watchListen,
			OutputPath: watchOutput,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create watcher: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Proxy recorder listening on %s for domain %s. Output: %s\n", watchListen, watchDomain, watchOutput)
		fmt.Println("Configure your client to use this proxy. Press Ctrl+C to stop.")

		if err := recorder.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "watcher failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if strings.TrimSpace(configPath) == "" {
		usage()
		fmt.Fprintln(os.Stderr, "\nerror: -config is required")
		os.Exit(2)
	}

	cfg, err := sqlscan.LoadConfig(configPath)
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

	report, err := sqlscan.RunScan(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	outputPath, err := sqlscan.WriteReport(report, cfg.ReportPath)
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
  go run . -watch-domain api.example.com -watch-listen 127.0.0.1:8088

The tool sends baseline and mutated requests to public API endpoints, looks for
SQL injection indicators, and writes a markdown or JSON report depending on the
report file extension.

Watcher mode runs a local explicit proxy that records requests for one domain and
keeps writing a starter config JSON you can feed back into the scanner.`)
}
