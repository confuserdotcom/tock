package classify

import (
	"regexp"
	"strings"
)

// siteOf reduces a stored host+path to the site an inbox row stands for: the
// host without "www.". A host whose subdomain carries a digit (CDN nodes and
// viewers such as doc-0o-bs.googleusercontent.com) collapses to its
// registrable domain, returned as domain too; domain is "" otherwise.
func siteOf(u string) (site, domain string) {
	host, _, _ := strings.Cut(u, "/")
	host, _, _ = strings.Cut(host, "?")
	host = strings.TrimPrefix(host, "www.")
	if strings.Contains(host, ":") || ipRe.MatchString(host) {
		return host, ""
	}
	labels := strings.Split(host, ".")
	n := 2
	if len(labels) >= 3 && len(labels[len(labels)-1]) == 2 && secondLevel[labels[len(labels)-2]] {
		n = 3 // example.co.uk, site.com.ma
	}
	if len(labels) <= n {
		return host, ""
	}
	sub := strings.Join(labels[:len(labels)-n], ".")
	if !strings.ContainsAny(sub, "0123456789") {
		return host, ""
	}
	d := strings.Join(labels[len(labels)-n:], ".")
	return d, d
}

var (
	ipRe = regexp.MustCompile(`^[0-9.]+$`)
	// secondLevel are the labels under a country code that are not a
	// registrable name on their own.
	secondLevel = map[string]bool{"co": true, "com": true, "org": true, "net": true, "ac": true, "gov": true, "edu": true, "gouv": true}
)

// pageOf is the first path segment of a stored URL, without a query: the
// unit a site row counts as a page.
func pageOf(u string) string {
	_, path, _ := strings.Cut(u, "/")
	seg, _, _ := strings.Cut(path, "/")
	seg, _, _ = strings.Cut(seg, "?")
	return seg
}

// siteRule is the learned-rule pattern that matches every page of site;
// a collapsed domain also matches all of its subdomains.
func siteRule(site, domain string) string {
	if domain != "" {
		return `^([^/]*\.)?` + regexp.QuoteMeta(domain) + `(/|$)`
	}
	return `^(www\.)?` + regexp.QuoteMeta(site) + `(/|$)`
}

// LegacyKey is the key a view had before inbox rows became whole sites
// (host plus first path segment). Keys ignored back then still hide it.
func LegacyKey(v SpanView) string {
	if v.Private || v.URL == nil || *v.URL == "" {
		return Key(v)
	}
	return v.App + " · " + urlKey(*v.URL)
}
