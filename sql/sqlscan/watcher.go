package sqlscan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type WatcherOptions struct {
	Domain     string
	ListenAddr string
	OutputPath string
}

type ProxyRecorder struct {
	opts      WatcherOptions
	transport *http.Transport

	mu         sync.Mutex
	config     Config
	endpoints  map[string]*EndpointConfig
	flushQueue chan struct{}
}

func NewProxyRecorder(opts WatcherOptions) (*ProxyRecorder, error) {
	opts.Domain = normalizeHost(opts.Domain)
	opts.ListenAddr = strings.TrimSpace(opts.ListenAddr)
	opts.OutputPath = strings.TrimSpace(opts.OutputPath)

	if opts.Domain == "" {
		return nil, fmt.Errorf("watch domain is required")
	}
	if opts.ListenAddr == "" {
		opts.ListenAddr = "127.0.0.1:8088"
	}
	if opts.OutputPath == "" {
		opts.OutputPath = "watched-config.json"
	}
	if !filepath.IsAbs(opts.OutputPath) {
		opts.OutputPath = filepath.Clean(opts.OutputPath)
	}

	recorder := &ProxyRecorder{
		opts: opts,
		transport: &http.Transport{
			Proxy:               nil,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		config: Config{
			Name:             opts.Domain + "-observed-api",
			BaseURL:          "https://" + opts.Domain,
			TimeoutSeconds:   10,
			DelayThresholdMS: 900,
			ReportPath:       "sqlscan-report.md",
			Endpoints:        []EndpointConfig{},
		},
		endpoints:  map[string]*EndpointConfig{},
		flushQueue: make(chan struct{}, 1),
	}

	return recorder, recorder.flushConfig()
}

func (p *ProxyRecorder) Run(ctx context.Context) error {
	go p.flushLoop(ctx)

	server := &http.Server{
		Addr:    p.opts.ListenAddr,
		Handler: p,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return p.flushConfig()
}

func (p *ProxyRecorder) FlushConfig() error {
	return p.flushConfig()
}

func (p *ProxyRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *ProxyRecorder) handleHTTP(w http.ResponseWriter, r *http.Request) {
	targetURL := cloneURL(r.URL)
	if !targetURL.IsAbs() {
		targetURL.Scheme = "http"
		targetURL.Host = r.Host
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	outbound := r.Clone(r.Context())
	outbound.URL = targetURL
	outbound.RequestURI = ""
	outbound.Host = targetURL.Host
	outbound.Body = io.NopCloser(bytes.NewReader(body))
	outbound.ContentLength = int64(len(body))
	removeHopByHopHeaders(outbound.Header)

	resp, err := p.transport.RoundTrip(outbound)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if hostMatches(targetURL.Hostname(), p.opts.Domain) {
		p.recordHTTP(targetURL, outbound, body, resp.StatusCode)
	}

	copyHeader(w.Header(), resp.Header)
	removeHopByHopHeaders(w.Header())
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *ProxyRecorder) handleConnect(w http.ResponseWriter, r *http.Request) {
	targetConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "proxy does not support hijacking", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if hostMatches(hostFromAddress(r.Host), p.opts.Domain) {
		p.noteTunnel(r.Host)
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	go tunnel(targetConn, clientConn)
	go tunnel(clientConn, targetConn)
}

func (p *ProxyRecorder) recordHTTP(targetURL *url.URL, req *http.Request, body []byte, status int) {
	path, pathParams := normalizeObservedPath(targetURL.Path)
	key := req.Method + " " + path
	queryValues := firstQueryValues(targetURL.Query())
	headers := captureHeaders(req.Header)
	jsonBody := captureJSONBody(req.Header.Get("Content-Type"), body)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.updateBaseURL(targetURL)

	endpoint, exists := p.endpoints[key]
	if !exists {
		endpoint = &EndpointConfig{
			Name:           strings.ToLower(strings.ReplaceAll(req.Method, " ", "-")) + "-" + endpointSlug(path),
			Method:         req.Method,
			Path:           path,
			Query:          map[string]string{},
			Headers:        map[string]string{},
			PathParams:     map[string]string{},
			JSONBody:       map[string]any{},
			ExpectedStatus: status,
		}
		p.endpoints[key] = endpoint
	}

	if status > 0 {
		endpoint.ExpectedStatus = status
	}
	mergeStringMap(endpoint.Query, queryValues)
	mergeStringMap(endpoint.Headers, headers)
	mergeStringMap(endpoint.PathParams, pathParams)
	mergeAnyMap(endpoint.JSONBody, jsonBody)
	endpoint.Targets = buildTargets(*endpoint)

	select {
	case p.flushQueue <- struct{}{}:
	default:
	}
}

func (p *ProxyRecorder) noteTunnel(address string) {
	host := hostFromAddress(address)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.config.BaseURL == "" || strings.HasPrefix(p.config.BaseURL, "http://") {
		p.config.BaseURL = "https://" + host
	}

	select {
	case p.flushQueue <- struct{}{}:
	default:
	}
}

func (p *ProxyRecorder) updateBaseURL(targetURL *url.URL) {
	if targetURL == nil || targetURL.Hostname() == "" {
		return
	}
	if p.config.BaseURL == "" || strings.HasPrefix(p.config.BaseURL, "http://") && targetURL.Scheme == "https" {
		p.config.BaseURL = targetURL.Scheme + "://" + targetURL.Hostname()
	}
}

func (p *ProxyRecorder) flushLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.flushQueue:
			_ = p.flushConfig()
		}
	}
}

func (p *ProxyRecorder) flushConfig() error {
	cfg := p.snapshot()
	return WriteConfig(p.opts.OutputPath, cfg)
}

func (p *ProxyRecorder) snapshot() Config {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := p.config
	cfg.DefaultHeaders = cloneStringMap(p.config.DefaultHeaders)
	cfg.Endpoints = make([]EndpointConfig, 0, len(p.endpoints))

	keys := make([]string, 0, len(p.endpoints))
	for key := range p.endpoints {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		endpoint := p.endpoints[key]
		cfg.Endpoints = append(cfg.Endpoints, cloneEndpoint(*endpoint))
	}

	return cfg
}

func normalizeObservedPath(path string) (string, map[string]string) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) == 1 && segments[0] == "" {
		return "/", map[string]string{}
	}

	pathParams := map[string]string{}
	dynamicIndex := 1
	for i, segment := range segments {
		if !looksDynamicPathSegment(segment) {
			continue
		}
		paramName := "param" + fmt.Sprint(dynamicIndex)
		dynamicIndex++
		pathParams[paramName] = segment
		segments[i] = "{" + paramName + "}"
	}

	return "/" + strings.Join(segments, "/"), pathParams
}

func looksDynamicPathSegment(segment string) bool {
	if segment == "" {
		return false
	}
	if allDigits(segment) {
		return true
	}
	if len(segment) == 36 && strings.Count(segment, "-") == 4 {
		return true
	}
	return false
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func firstQueryValues(values url.Values) map[string]string {
	out := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) == 0 {
			continue
		}
		out[key] = items[0]
	}
	return out
}

func captureHeaders(header http.Header) map[string]string {
	out := map[string]string{}
	for key, values := range header {
		if !shouldCaptureHeader(key) || len(values) == 0 {
			continue
		}
		out[key] = values[0]
	}
	return out
}

func shouldCaptureHeader(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, blocked := range []string{
		"authorization",
		"proxy-authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"x-auth-token",
	} {
		if key == blocked {
			return false
		}
	}
	return strings.HasPrefix(key, "x-") || key == "content-type" || key == "accept"
}

func captureJSONBody(contentType string, body []byte) map[string]any {
	if !strings.Contains(strings.ToLower(contentType), "application/json") || len(body) == 0 {
		return map[string]any{}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return map[string]any{}
	}

	out := map[string]any{}
	for key, value := range payload {
		switch typed := value.(type) {
		case string, bool, float64, int, int64:
			out[key] = typed
		default:
			out[key] = fmt.Sprint(typed)
		}
	}
	return out
}

func buildTargets(endpoint EndpointConfig) []TargetConfig {
	targets := endpoint.autoTargets()
	for key := range endpoint.Headers {
		targets = append(targets, TargetConfig{Name: key, Location: "header"})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Location == targets[j].Location {
			return targets[i].Name < targets[j].Name
		}
		return targets[i].Location < targets[j].Location
	})
	return targets
}

func mergeStringMap(dst, src map[string]string) {
	for key, value := range src {
		if _, exists := dst[key]; exists {
			continue
		}
		dst[key] = value
	}
}

func mergeAnyMap(dst, src map[string]any) {
	for key, value := range src {
		if _, exists := dst[key]; exists {
			continue
		}
		dst[key] = value
	}
}

func endpointSlug(path string) string {
	slug := strings.Trim(path, "/")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, "{", "")
	slug = strings.ReplaceAll(slug, "}", "")
	if slug == "" {
		return "root"
	}
	return slug
}

func hostMatches(host, domain string) bool {
	host = normalizeHost(host)
	domain = normalizeHost(domain)
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil {
			host = parsed.Hostname()
		}
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.TrimSpace(host)
}

func hostFromAddress(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return normalizeHost(address)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeHopByHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}

func tunnel(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	_, _ = io.Copy(dst, src)
}

func cloneURL(in *url.URL) *url.URL {
	if in == nil {
		return &url.URL{}
	}
	out := *in
	return &out
}
