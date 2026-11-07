package main

import (
	"path/filepath"

	"tock/internal/config"
	"tock/internal/span"
)

// hideEvery is how often the hide lists are read again.
const hideEvery int64 = 60_000

// hidePausedEvery is how often a config.toml that did not load is read
// again, so capture resumes soon after the file is fixed.
const hidePausedEvery int64 = 5_000

// hider tracks samples of the [privacy] hidden apps and sites as time only.
// It reads config.toml again once every ms of sample time.
//
// A config.toml that does not load (broken, or keys this trackd does not
// know, as a newer tock writes) pauses capture: the pipe records nothing
// until it loads. There is no fallback to the defaults or to the last good
// lists, so a hidden app's title or URL is never stored on account of a
// broken file.
type hider struct {
	load  func() (config.Privacy, error)
	every int64
	at    int64 // sample time of the last read; 0 = never
	p     config.Privacy
	err   error // the last read's error; non-nil = paused
}

// newHider reads the hide lists from the config.toml in configDir.
func newHider(configDir string) *hider {
	path := filepath.Join(configDir, config.File)
	return &hider{every: hideEvery, load: func() (config.Privacy, error) {
		c, err := config.Load(path)
		return c.Privacy, err
	}}
}

// check reads config.toml again when a read is due at ts, and reports
// whether that paused or resumed capture. A nil hider never pauses.
func (h *hider) check(ts int64) (changed bool) {
	if h == nil {
		return false
	}
	every := h.every
	if h.err != nil {
		every = hidePausedEvery
	}
	if h.at != 0 && ts-h.at < every && ts >= h.at {
		return false
	}
	h.at = ts
	was := h.err != nil
	p, err := h.load()
	if err != nil {
		h.p, h.err = config.Privacy{}, err
	} else {
		h.p, h.err = p, nil
	}
	return was != (h.err != nil)
}

// paused reports whether capture is paused because config.toml did not load.
func (h *hider) paused() bool { return h != nil && h.err != nil }

// reason is the config error that paused capture, or "".
func (h *hider) reason() string {
	if !h.paused() {
		return ""
	}
	return h.err.Error()
}

// apply makes s private, with no title or URL, when its app or site is
// hidden. It reads the lists first when due. A nil hider changes nothing;
// the pipe drops samples while paused, before apply.
func (h *hider) apply(s *span.Sample) {
	if h == nil || s == nil {
		return
	}
	h.check(s.TS)
	if h.p.Hides(s.BundleID, s.URL) {
		s.Private, s.Title, s.URL = true, nil, nil
	}
}
