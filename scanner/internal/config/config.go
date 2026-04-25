package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const defaultUserAgent = "ProbeTools-Scanner/0.1"

type Config struct {
	Target    string
	Output    string
	MaxPages  int
	MaxDepth  int
	Timeout   time.Duration
	UserAgent string
	Probe     bool
}

func ParseArgs(args []string) (Config, error) {
	fs := flag.NewFlagSet("scanner", flag.ContinueOnError)

	cfg := Config{}
	fs.StringVar(&cfg.Target, "url", "", "Target website URL to scan.")
	fs.StringVar(&cfg.Output, "output", "report.json", "Path to the JSON report file.")
	fs.IntVar(&cfg.MaxPages, "max-pages", 50, "Maximum number of HTML pages to crawl.")
	fs.IntVar(&cfg.MaxDepth, "max-depth", 2, "Maximum link depth to crawl from the target page.")
	fs.DurationVar(&cfg.Timeout, "timeout", 10*time.Second, "Timeout for HTTP requests.")
	fs.StringVar(&cfg.UserAgent, "user-agent", defaultUserAgent, "User-Agent to send on HTTP requests.")
	fs.BoolVar(&cfg.Probe, "probe", true, "Issue safe HEAD probes for discovered GET-like endpoints.")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Target) == "" {
		return errors.New("missing required -url flag")
	}

	parsed, err := url.Parse(c.Target)
	if err != nil {
		return fmt.Errorf("parse target url: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("target URL must use http or https")
	}

	if parsed.Host == "" {
		return errors.New("target URL must include a host")
	}

	if c.MaxPages <= 0 {
		return errors.New("max-pages must be greater than zero")
	}

	if c.MaxDepth < 0 {
		return errors.New("max-depth must be zero or greater")
	}

	if c.Timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}

	if strings.TrimSpace(c.Output) == "" {
		return errors.New("output path must not be empty")
	}

	if strings.TrimSpace(c.UserAgent) == "" {
		return errors.New("user-agent must not be empty")
	}

	return nil
}
