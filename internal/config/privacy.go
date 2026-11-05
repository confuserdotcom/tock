package config

import (
	"fmt"
	"regexp"
	"strings"
)

// siteRE is a hidden site: a lower-case host, no scheme, path or port.
var siteRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

// decodePrivacy reads the hide lists and adds them to the [llm] deny lists,
// which OCR reads too.
func decodePrivacy(f privacyTOML, c *Config) error {
	if f.HideApps != nil {
		for _, b := range *f.HideApps {
			if b == "" || strings.ContainsAny(b, " \t/") {
				return fmt.Errorf("[privacy] hide_apps has %q, want a bundle id like com.apple.Safari", b)
			}
		}
		c.Privacy.HideApps = append([]string{}, *f.HideApps...)
	}
	if f.HideSites != nil {
		for _, h := range *f.HideSites {
			if !siteRE.MatchString(h) {
				return fmt.Errorf("[privacy] hide_sites has %q, want a lower-case host like bank.example.com", h)
			}
		}
		c.Privacy.HideSites = append([]string{}, *f.HideSites...)
	}
	c.LLM.DenyApps = append(append([]string{}, c.LLM.DenyApps...), c.Privacy.HideApps...)
	for _, h := range c.Privacy.HideSites {
		c.LLM.DenyHosts = append(c.LLM.DenyHosts, SitePattern(h))
	}
	return nil
}

// SitePattern is the RE2 pattern that matches host and its subdomains.
func SitePattern(host string) string { return `(^|\.)` + regexp.QuoteMeta(host) + `$` }

// Hides reports whether a sample of bundle at url (host+path, nil = none)
// is hidden.
func (p Privacy) Hides(bundle string, url *string) bool {
	for _, b := range p.HideApps {
		if b == bundle {
			return true
		}
	}
	host := rawHost(url)
	if host == "" {
		return false
	}
	for _, h := range p.HideSites {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// rawHost is the lower-case host of url (host+path, nil = none), without
// its port.
func rawHost(url *string) string {
	if url == nil {
		return ""
	}
	host, _, _ := strings.Cut(*url, "/")
	host, _, _ = strings.Cut(host, ":")
	return strings.ToLower(host)
}

// Host is the site to offer hiding for url: its host without the port and
// a leading www., or "" when there is none or it is not a valid site.
func Host(url *string) string {
	h := strings.TrimPrefix(rawHost(url), "www.")
	if !siteRE.MatchString(h) {
		return ""
	}
	return h
}
