package suggest

import (
	"regexp"
	"strings"

	"tock/internal/classify"
	"tock/internal/config"
)

// Filter decides which inbox items may leave the machine.
type Filter struct {
	minMS  int64
	apps   map[string]bool
	hosts  []*regexp.Regexp
	titles []*regexp.Regexp
}

// NewFilter compiles the [llm] deny lists.
func NewFilter(c config.LLM) (*Filter, error) {
	f := &Filter{minMS: c.MinMS, apps: map[string]bool{}}
	for _, b := range c.DenyApps {
		f.apps[b] = true
	}
	for _, p := range c.DenyHosts {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		f.hosts = append(f.hosts, re)
	}
	for _, p := range c.DenyTitles {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, err
		}
		f.titles = append(f.titles, re)
	}
	return f, nil
}

// rest is the part of the key after "<app> · ": a host and segment, a
// normalised title, or "" for an app-only key.
func rest(it classify.Item) string {
	r, ok := strings.CutPrefix(it.Key, it.App+" · ")
	if !ok {
		return ""
	}
	return r
}

// Allow reports whether it may be sent. Host patterns run against the first
// part of every key's text and title patterns against all of it, whatever
// the key kind: a pattern can only make the filter stricter.
func (f *Filter) Allow(it classify.Item) bool {
	if it.Private || it.TotalMS < f.minMS {
		return false
	}
	for _, b := range it.Bundles {
		if f.apps[b] {
			return false
		}
	}
	r := rest(it)
	if host, _, _ := strings.Cut(r, "/"); host != "" {
		for _, re := range f.hosts {
			if re.MatchString(host) {
				return false
			}
		}
	}
	for _, re := range f.titles {
		if re.MatchString(r) {
			return false
		}
	}
	return true
}
