package site

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

func SameSiteHost(left, right string) bool {
	leftHost := normalizeHost(left)
	rightHost := normalizeHost(right)

	if leftHost == "" || rightHost == "" {
		return false
	}

	if leftHost == rightHost {
		return true
	}

	leftRoot, leftErr := publicsuffix.EffectiveTLDPlusOne(leftHost)
	rightRoot, rightErr := publicsuffix.EffectiveTLDPlusOne(rightHost)
	if leftErr == nil && rightErr == nil {
		return leftRoot == rightRoot
	}

	return false
}

func normalizeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse("https://" + raw)
	if err != nil {
		return strings.ToLower(raw)
	}

	host := parsed.Hostname()
	if host == "" {
		return strings.ToLower(raw)
	}

	return strings.ToLower(host)
}
