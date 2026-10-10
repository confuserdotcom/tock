package app

import (
	"sort"
	"strings"

	"tock/internal/report"
	"tock/internal/store"
)

// DownAfterMS is how old trackd's last sign of life may be before it counts
// as down: three missed 10 s heartbeats.
const DownAfterMS = 30_000

// browsers names the browsers trackd reads URLs from.
var browsers = map[string]string{
	"com.apple.Safari":           "Safari",
	"com.google.Chrome":          "Chrome",
	"company.thebrowser.Browser": "Arc",
	"com.brave.Browser":          "Brave",
}

// Health is what the TUI banner and tock status need to know.
type Health struct {
	TrackdUp      bool
	LastSeenMS    int64    // 0 when trackd never ran
	AXTrusted     *bool    // nil when unknown
	AutomationOff []string // browsers whose Automation permission is denied, sorted
	Version       string
	RulesErr      error
	ConfigErr     error
}

// Health reads trackd's liveness and permission state.
func (a *App) Health() (Health, error) {
	h := Health{RulesErr: a.rulesErr, ConfigErr: a.configErr}
	last, err := a.st.LastSeen()
	if err != nil {
		return h, err
	}
	h.LastSeenMS = last
	h.TrackdUp = last > 0 && a.nowMS()-last <= DownAfterMS
	states, err := a.st.States()
	if err != nil {
		return h, err
	}
	if v, ok := states[store.KeyAXTrusted]; ok {
		b := v == "1"
		h.AXTrusted = &b
	}
	h.Version = states[store.KeyVersion]
	for k, v := range states {
		id, ok := strings.CutPrefix(k, store.AutomationKey(""))
		if !ok || v != "denied" {
			continue
		}
		name := browsers[id]
		if name == "" {
			name = id
		}
		h.AutomationOff = append(h.AutomationOff, name)
	}
	sort.Strings(h.AutomationOff)
	return h, nil
}

// Status is the line `tock status` prints.
func (a *App) Status() (string, error) {
	d, err := a.Day(a.now())
	if err != nil {
		return "", err
	}
	h, err := a.Health()
	if err != nil {
		return "", err
	}
	in := report.StatusInput{TrackedMS: d.TrackedMS, FocusMS: d.Stats.FocusMS, Score: d.Score.Value,
		Active: d.Stats.ActiveMS > 0, TrackdUp: h.TrackdUp}
	open, err := a.st.OpenSession()
	if err != nil {
		return "", err
	}
	if open != nil {
		in.InSession, in.SessionLeftMS = true, open.PlannedMS-a.nowMS()
	}
	if n := len(d.Timeline); n > 0 && d.Timeline[n-1].Open {
		in.Open = &d.Timeline[n-1]
	}
	return report.Status(in), nil
}
