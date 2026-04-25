# ProbeTools

## Scanner

`scanner` is a Go CLI that crawls a target website over plain HTTP, discovers likely backend/API endpoints, and writes a JSON report for later analysis.

### Current scope

- Accepts a target URL via CLI flags.
- Crawls same-origin HTML pages only.
- Follows discovered links breadth-first up to a configurable crawl depth.
- Treats canonical redirects such as apex-to-`www` as the same site for continued crawling.
- Mines same-site JavaScript bundles for route-like paths when pages are client-rendered and HTML contains no direct links.
- Extracts backend-call candidates from links, form actions, resource references, and inline script bodies.
- Applies heuristics to keep likely backend endpoints and can issue safe `HEAD` probes for GET-like candidates.
- Writes a structured JSON report with every crawled GET request, page coverage, page depth, discovered calls, and warnings.

### Structure

- `cmd/scanner`: CLI entrypoint
- `internal/config`: flag parsing and validation
- `internal/crawl`: crawl orchestration and queue management
- `internal/fetch`: HTTP fetching
- `internal/extract`: HTML parsing and reference extraction
- `internal/discovery`: backend-call classification and aggregation
- `internal/probe`: safe endpoint probing
- `internal/report`: JSON report writing

### Usage

```bash
go run ./cmd/scanner -url https://example.com -output report.json
```

Useful flags:

- `-max-pages`: limit how many HTML pages are crawled
- `-max-depth`: limit how deep link traversal can go from the starting page
- `-timeout`: per-request timeout, for example `15s`
- `-probe=false`: disable `HEAD` probes for discovered endpoints
- `-user-agent`: override the default scanner user agent

The generated report now includes a `requests` section listing each GET request the crawler attempted, including depth, final URL, status, content type, and referrer when applicable.

### Known limitations

- No JavaScript execution yet, so XHR/fetch calls created only at runtime will not be observed.
- The first version stays same-origin for crawling and only uses heuristics to classify backend calls.
- POST form endpoints are recorded but not executed.
