package model

import "time"

type Source string

const (
	SourceLink         Source = "link"
	SourceForm         Source = "form"
	SourceScriptRef    Source = "script_ref"
	SourceResourceRef  Source = "resource_ref"
	SourceInlineScript Source = "inline_script"
)

type ProbeResult struct {
	Attempted  bool   `json:"attempted"`
	Method     string `json:"method,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	FinalURL   string `json:"final_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Request struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	Depth       int    `json:"depth"`
	StatusCode  int    `json:"status_code,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Referrer    string `json:"referrer,omitempty"`
	Error       string `json:"error,omitempty"`
}

type BackendCall struct {
	URL           string      `json:"url"`
	Host          string      `json:"host"`
	Path          string      `json:"path"`
	Methods       []string    `json:"methods"`
	Sources       []Source    `json:"sources"`
	EvidencePages []string    `json:"evidence_pages"`
	Reasons       []string    `json:"reasons"`
	Probe         ProbeResult `json:"probe"`
}

type Page struct {
	URL                 string `json:"url"`
	Depth               int    `json:"depth"`
	StatusCode          int    `json:"status_code"`
	ContentType         string `json:"content_type"`
	DiscoveredCallCount int    `json:"discovered_call_count"`
}

type ReportConfig struct {
	MaxPages  int    `json:"max_pages"`
	MaxDepth  int    `json:"max_depth"`
	Timeout   string `json:"timeout"`
	UserAgent string `json:"user_agent"`
	Probe     bool   `json:"probe"`
}

type Summary struct {
	PagesCrawled    int      `json:"pages_crawled"`
	GetRequests     int      `json:"get_requests"`
	BackendCalls    int      `json:"backend_calls"`
	MaxDepthReached int      `json:"max_depth_reached"`
	Warnings        []string `json:"warnings,omitempty"`
}

type Report struct {
	Target       string        `json:"target"`
	GeneratedAt  time.Time     `json:"generated_at"`
	Config       ReportConfig  `json:"config"`
	Summary      Summary       `json:"summary"`
	Requests     []Request     `json:"requests"`
	Pages        []Page        `json:"pages"`
	BackendCalls []BackendCall `json:"backend_calls"`
}
