package app

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/klexas/ProbeTools/scanner/internal/config"
	"github.com/klexas/ProbeTools/scanner/internal/crawl"
	"github.com/klexas/ProbeTools/scanner/internal/discovery"
	"github.com/klexas/ProbeTools/scanner/internal/extract"
	"github.com/klexas/ProbeTools/scanner/internal/fetch"
	"github.com/klexas/ProbeTools/scanner/internal/model"
	"github.com/klexas/ProbeTools/scanner/internal/probe"
	"github.com/klexas/ProbeTools/scanner/internal/report"
)

func Run(cfg config.Config) error {
	target, err := url.Parse(cfg.Target)
	if err != nil {
		return fmt.Errorf("parse target url: %w", err)
	}

	fetcher := fetch.NewHTTPClient(cfg.Timeout, cfg.UserAgent)
	extractor := extract.NewHTMLExtractor()

	var endpointProber probe.Prober
	if cfg.Probe {
		endpointProber = probe.NewHTTPProber(cfg.Timeout, cfg.UserAgent)
	}

	analyzer := discovery.NewAnalyzer(target, cfg.Probe, endpointProber)
	crawler := crawl.NewCrawler(target, cfg.MaxPages, fetcher, extractor, analyzer)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.MaxPages+5)*cfg.Timeout)
	defer cancel()

	pages, backendCalls, warnings, err := crawler.Run(ctx)
	if err != nil {
		return err
	}

	scanReport := model.Report{
		Target:      target.String(),
		GeneratedAt: time.Now().UTC(),
		Config: model.ReportConfig{
			MaxPages:  cfg.MaxPages,
			Timeout:   cfg.Timeout.String(),
			UserAgent: cfg.UserAgent,
			Probe:     cfg.Probe,
		},
		Summary: model.Summary{
			PagesCrawled: len(pages),
			BackendCalls: len(backendCalls),
			Warnings:     warnings,
		},
		Pages:        pages,
		BackendCalls: backendCalls,
	}

	if err := report.NewJSONWriter().Write(cfg.Output, scanReport); err != nil {
		return err
	}

	return nil
}
