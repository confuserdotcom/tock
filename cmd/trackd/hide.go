package main

import (
	"path/filepath"

	"tock/internal/config"
	"tock/internal/span"
)

// hideEvery is how often the hide lists are read again.
const hideEvery int64 = 60_000

// hider tracks samples of the [privacy] hidden apps and sites as time only.
// It reads the lists again once every ms of sample time; a failed read keeps
// the last good lists.
type hider struct {
	load  func() (config.Privacy, error)
	every int64
	at    int64 // sample time of the last read; 0 = never
	p     config.Privacy
}

// newHider reads the hide lists from the config.toml in configDir.
func newHider(configDir string) *hider {
	path := filepath.Join(configDir, config.File)
	return &hider{every: hideEvery, load: func() (config.Privacy, error) {
		c, err := config.Load(path)
		return c.Privacy, err
	}}
}

// apply makes s private, with no title or URL, when its app or site is
// hidden. A nil hider changes nothing.
func (h *hider) apply(s *span.Sample) {
	if h == nil || s == nil {
		return
	}
	if h.at == 0 || s.TS-h.at >= h.every || s.TS < h.at {
		h.at = s.TS
		if p, err := h.load(); err == nil {
			h.p = p
		}
	}
	if h.p.Hides(s.BundleID, s.URL) {
		s.Private, s.Title, s.URL = true, nil, nil
	}
}
