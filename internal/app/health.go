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
	TrackdUp   bool
	LastSeenMS int64 // 0 when trackd never ran
	// Recorded says the store holds any activity at all; false on the
	// first run, before trackd's first span.
	Recorded      bool
	AXTrusted     *bool    // nil when unknown
	AutomationOff []string // browsers whose Automation permission is denied, sorted
	Version       string
	// CalendarStatus is trackd's calendar access ("" before trackd
	// reports); CalendarSyncMS the last sync, 0 when none.
	CalendarStatus string
	CalendarSyncMS int64
	// OCRPerm is the Screen Recording permission trackd's last screen read
	// saw: granted, missing, or "" before any.
	OCRPerm string
	// BarUp says the menu bar app's heartbeat is fresh; BarNotify is its
	// notification access ("" before it reports).
	BarUp     bool
	BarNotify string
	RulesErr  error
	ConfigErr error
	// BackupMS is trackd's last good daily snapshot (0 = none); BackupErr
	// its last error. IntegrityMS is the last check, IntegrityOK "ok", a
	// problem, or "" before any check.
	BackupMS    int64
	BackupErr   string
	IntegrityMS int64
	IntegrityOK string
	// CapturePaused is the config.toml error that paused trackd's
	// recording; "" while it records (see Paused).
	CapturePaused string
	// TrackdStopped is why trackd stopped and stays down
	// (store.StoppedSchemaNewer), "" otherwise. See TooOld.
	TrackdStopped string
}

// TooOldText says what TooOld means, for every surface.
const TooOldText = "trackd is older than the database"

// TooOld says trackd is down because a newer tock raised the database's
// compat level: restarting the same trackd would not help, replacing it
// with the newer one does.
func (h Health) TooOld() bool { return !h.TrackdUp && h.TrackdStopped == store.StoppedSchemaNewer }

// Paused says trackd is running but records nothing, because its
// config.toml does not load. A pause left behind by a trackd that is down
// now does not count: down says more.
func (h Health) Paused() bool { return h.TrackdUp && h.CapturePaused != "" }

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
	if _, h.Recorded, err = st.FirstMS(); err != nil {
		return h, err
	}
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
	h.OCRPerm = states[store.KeyOCRPerm]
	h.CalendarSyncMS, _ = strconv.ParseInt(states[store.KeyCalendarSync], 10, 64)
	beat, _ := strconv.ParseInt(states[store.KeyBarBeat], 10, 64)
	h.BarUp = beat > 0 && nowMS-beat <= BarFreshMS && beat <= nowMS+BarFreshMS
	h.BarNotify = states[store.KeyBarNotify]
	h.BackupMS, _ = strconv.ParseInt(states[store.KeyBackupLast], 10, 64)
	h.BackupErr = states[store.KeyBackupErr]
	h.IntegrityMS, _ = strconv.ParseInt(states[store.KeyIntegrityAt], 10, 64)
	h.IntegrityOK = states[store.KeyIntegrityOK]
	h.CapturePaused = states[store.KeyCapturePaused]
	h.TrackdStopped = states[store.KeyTrackdStopped]
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

// Damaged says the last check found a problem.
func (h Health) Damaged() bool { return h.IntegrityOK != "" && h.IntegrityOK != "ok" }

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
	line := report.Status(in)
	if h.Damaged() {
		line = "db! " + line
	}
	if h.Paused() {
		line = "paused! " + line
	}
	if h.TooOld() {
		line = "old! " + line
	}
	return line, nil
}
