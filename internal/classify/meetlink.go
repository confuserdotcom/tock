package classify

import (
	"net/url"
	"strings"
)

// MeetingLinkMax is the longest meeting link, in bytes, tock keeps or opens.
const MeetingLinkMax = 2048

// MeetingLink returns the first Meet, Zoom or Teams link found in texts,
// looked at in order (first text first, left to right inside a text), or ""
// when none passes IsMeetingLink. Texts come from calendar invites, so they
// are third-party input: only https links on the hosts listed at
// IsMeetingLink are ever returned, and those are the only hosts tock opens.
func MeetingLink(texts ...string) string {
	for _, text := range texts {
		for i := 0; i+len(httpsPrefix) <= len(text); i++ {
			if !strings.EqualFold(text[i:i+len(httpsPrefix)], httpsPrefix) {
				continue
			}
			cand := text[i:]
			if end := strings.IndexFunc(cand, func(r rune) bool {
				return r <= ' ' || r == 0x7f || strings.ContainsRune(`<>"'`, r)
			}); end >= 0 {
				cand = cand[:end]
			}
			cand = strings.TrimRight(cand, ".,;:!?)]}")
			if u := meetingURL(cand); u != nil {
				return u.String()
			}
		}
	}
	return ""
}

const httpsPrefix = "https://"

// IsMeetingLink reports whether s, taken whole, is a link tock may open: at
// most MeetingLinkMax bytes without whitespace or control characters,
// scheme https, no userinfo, no explicit port, and the host meet.google.com,
// zoom.us or a subdomain of it, teams.microsoft.com or teams.live.com. This
// is the only set of hosts tock ever opens.
func IsMeetingLink(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] == 0x7f {
			return false
		}
	}
	return meetingURL(s) != nil
}

// meetingURL parses s and returns it when it passes the host rules.
func meetingURL(s string) *url.URL {
	if s == "" || len(s) > MeetingLinkMax {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Opaque != "" {
		return nil
	}
	if !isMeetingHost(strings.ToLower(u.Hostname())) {
		return nil
	}
	return u
}

// isMeetingHost reports whether the lowercase host h is a meeting host:
// meet.google.com, zoom.us or a subdomain, teams.microsoft.com or
// teams.live.com.
func isMeetingHost(h string) bool {
	switch {
	case h == "meet.google.com", h == "zoom.us", h == "teams.microsoft.com", h == "teams.live.com":
		return true
	}
	return strings.HasSuffix(h, ".zoom.us") && len(h) > len(".zoom.us")
}
