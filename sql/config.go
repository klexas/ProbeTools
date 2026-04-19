package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Name              string            `json:"name"`
	BaseURL           string            `json:"base_url"`
	DefaultHeaders    map[string]string `json:"default_headers"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
	DelayThresholdMS  int               `json:"delay_threshold_ms"`
	ReportPath        string            `json:"report_path"`
	Endpoints         []EndpointConfig  `json:"endpoints"`
}

type EndpointConfig struct {
	Name           string                 `json:"name"`
	Method         string                 `json:"method"`
	Path           string                 `json:"path"`
	Query          map[string]string      `json:"query"`
	Headers        map[string]string      `json:"headers"`
	PathParams     map[string]string      `json:"path_params"`
	JSONBody       map[string]any         `json:"json_body"`
	Targets        []TargetConfig         `json:"targets"`
	ExpectedStatus int                    `json:"expected_status"`
}

type TargetConfig struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if err := cfg.normalize(path); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) normalize(sourcePath string) error {
	c.Name = strings.TrimSpace(c.Name)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")

	if c.BaseURL == "" {
		return fmt.Errorf("base_url is required")
	}
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 10
	}
	if c.DelayThresholdMS <= 0 {
		c.DelayThresholdMS = 900
	}
	if strings.TrimSpace(c.ReportPath) == "" {
		c.ReportPath = defaultReportPath()
	}
	if !filepath.IsAbs(c.ReportPath) {
		c.ReportPath = filepath.Clean(c.ReportPath)
	}
	if c.DefaultHeaders == nil {
		c.DefaultHeaders = map[string]string{}
	}

	for i := range c.Endpoints {
		if err := c.Endpoints[i].normalize(); err != nil {
			return fmt.Errorf("endpoint %d: %w", i+1, err)
		}
	}

	return nil
}

func (e *EndpointConfig) normalize() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
	e.Path = strings.TrimSpace(e.Path)

	if e.Method == "" {
		e.Method = "GET"
	}
	if e.Name == "" {
		e.Name = fmt.Sprintf("%s %s", e.Method, e.Path)
	}
	if e.Path == "" {
		return fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(e.Path, "/") {
		e.Path = "/" + e.Path
	}
	if e.Query == nil {
		e.Query = map[string]string{}
	}
	if e.Headers == nil {
		e.Headers = map[string]string{}
	}
	if e.PathParams == nil {
		e.PathParams = map[string]string{}
	}
	if e.JSONBody == nil {
		e.JSONBody = map[string]any{}
	}
	if len(e.Targets) == 0 {
		e.Targets = e.autoTargets()
	}
	for i := range e.Targets {
		e.Targets[i].Location = strings.ToLower(strings.TrimSpace(e.Targets[i].Location))
		e.Targets[i].Name = strings.TrimSpace(e.Targets[i].Name)
		if e.Targets[i].Name == "" || e.Targets[i].Location == "" {
			return fmt.Errorf("targets require name and location")
		}
	}
	return nil
}

func (e EndpointConfig) autoTargets() []TargetConfig {
	targets := make([]TargetConfig, 0, len(e.Query)+len(e.PathParams)+len(e.JSONBody))

	for name := range e.Query {
		targets = append(targets, TargetConfig{Name: name, Location: "query"})
	}
	for name := range e.PathParams {
		targets = append(targets, TargetConfig{Name: name, Location: "path"})
	}
	for name := range e.JSONBody {
		targets = append(targets, TargetConfig{Name: name, Location: "json"})
	}

	return targets
}
