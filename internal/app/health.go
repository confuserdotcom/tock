package app

import (
	"sort"
	"strconv"
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
	// CalendarStatus is trackd's calendar access ("" before trackd
	// reports); CalendarSyncMS the last sync, 0 when none.
	CalendarStatus string
	CalendarSyncMS int64
	RulesErr       error
	ConfigErr      error
}

// Health reads trackd's liveness and permission state.
func (a *App) Health() (Health, error) {
	h, err := ReadHealth(a.st, a.nowMS())
	h.RulesErr, h.ConfigErr = a.rulesErr, a.configErr
	return h, err
}

// ReadHealth reads trackd's liveness and permission state from the store,
// for callers without rules (tock doctor). RulesErr and ConfigErr stay nil.
func ReadHealth(st *store.Store, nowMS int64) (Health, error) {
	var h Health
	last, err := st.LastSeen()
	if err != nil {
		return h, err
	}
	h.LastSeenMS = last
	h.TrackdUp = last > 0 && nowMS-last <= DownAfterMS
	states, err := st.States()
	if err != nil {
		return h, err
	}
	if v, ok := states[store.KeyAXTrusted]; ok {
		b := v == "1"
		h.AXTrusted = &b
	}
	h.Version = states[store.KeyVersion]
	h.CalendarStatus = states[store.KeyCalendarStatus]
	h.CalendarSyncMS, _ = strconv.ParseInt(states[store.KeyCalendarSync], 10, 64)
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
		Active: d.Stats.ActiveMS > 0, TrackdUp: h.TrackdUp, Goals: d.Goals}
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
