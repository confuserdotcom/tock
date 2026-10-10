package span

import (
	"net/url"
	"strings"
)

// maxURLLen bounds the raw URL accepted from a browser. Longer input is
// rejected rather than parsed.
const maxURLLen = 8192

// URLKey reduces a browser URL to lowercase host (with port) plus path
// without a trailing slash. Scheme, userinfo, query and fragment are dropped
// so tokens and credentials never reach storage (T-1-urltokens). Only http and
// https URLs with a host are accepted; anything else returns ok=false.
func URLKey(raw string) (string, bool) {
	if raw == "" || len(raw) > maxURLLen {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if u.Host == "" {
		return "", false
	}
	return strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/"), true
}
