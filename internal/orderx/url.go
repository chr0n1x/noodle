package orderx

import (
	"net/url"
	"strings"
)

// CleanOrderURL returns raw trimmed when it is an absolute http or https URL
// with a host, and "" otherwise, so an order without a usable link keeps a
// plain title.
func CleanOrderURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return raw
}
