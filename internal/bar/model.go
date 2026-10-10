// Package bar is the logic of the tock menu bar app: what its title and
// menu show, which buttons a notification gets and what each click does.
// The Cocoa side in cmd/tockbar only draws and forwards clicks.
package bar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/report"
)

// Menu item ids.
const (
	IDStart    = "start:" // + minutes
	IDStop     = "stop"
	IDSettings = "settings"
	IDQuit     = "quit"
)

// SettingsURL opens the Notifications pane of System Settings.
const SettingsURL = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"

// maxPresets is how many start items the menu offers.
const maxPresets = 3

// Snapshot is what the menu bar shows, read from tock's database.
type Snapshot struct {
	OK           bool // false when tock could not open (no rules or database yet)
	TrackedMS    int64
	FocusMS      int64
	Score        int
	Active       bool // the day has activity, so the score means something
	UnlabelledMS int64
	TrackdUp     bool
	Session      *focus.Session // the running session, or nil
	Presets      []int
	Access       string // notification access the app last saw
}

// Load reads today's snapshot. access is the app's notification access.
func Load(a *app.App, now time.Time, access string) (Snapshot, error) {
	s := Snapshot{OK: true, Access: access, Presets: a.Config().Focus.Presets}
	h, err := a.Health()
	if err != nil {
		return s, err
	}
	s.TrackdUp = h.TrackdUp
	d, err := a.Day(now)
	if err != nil {
		return s, err
	}
	s.TrackedMS, s.FocusMS, s.UnlabelledMS = d.TrackedMS, d.Stats.FocusMS, d.ByKind[""]
	s.Score, s.Active = d.Score.Value, d.Stats.ActiveMS > 0
	f, err := a.Focus(now)
	if err != nil {
		return s, err
	}
	s.Session = f.Open
	return s, nil
}

// Title is the menu bar text at nowMS: the countdown while a session runs,
// else today's tracked time, with a ! while trackd is down.
func (s Snapshot) Title(nowMS int64) string {
	switch {
	case !s.OK:
		return "◷ tock"
	case s.Session != nil:
		return "◉ " + report.Countdown(s.Session.PlannedMS-nowMS)
	case !s.TrackdUp:
		return "◷ " + report.Dur(s.TrackedMS) + " !"
	}
	return "◷ " + report.Dur(s.TrackedMS)
}

// Item is one menu row. A row with no ID is a disabled label; Separator
// rows have nothing else.
type Item struct {
	Title     string
	ID        string
	Separator bool
}

func label(f string, a ...any) Item { return Item{Title: fmt.Sprintf(f, a...)} }

var sep = Item{Separator: true}

// Items is the menu at nowMS.
func (s Snapshot) Items(nowMS int64) []Item {
	if !s.OK {
		return []Item{label("tock is not set up: run tock init"), sep, {Title: "Quit tock menu bar", ID: IDQuit}}
	}
	out := []Item{label("Today · %s tracked", report.Dur(s.TrackedMS))}
	if s.Active {
		out = append(out, label("Focus %s · score %d", report.Dur(s.FocusMS), s.Score))
	}
	if ss := s.Session; ss != nil {
		word := "interruptions"
		if ss.Interruptions == 1 {
			word = "interruption"
		}
		out = append(out, label("Session · %s left · %d %s", report.Countdown(ss.PlannedMS-nowMS), ss.Interruptions, word))
	}
	if s.UnlabelledMS > 0 {
		out = append(out, label("%s unlabelled · open tock to label", report.Dur(s.UnlabelledMS)))
	}
	if !s.TrackdUp {
		out = append(out, label("trackd is not running"))
	}
	out = append(out, sep)
	if s.Session != nil {
		out = append(out, Item{Title: "Stop session", ID: IDStop})
	} else {
		for _, m := range s.Presets[:min(len(s.Presets), maxPresets)] {
			out = append(out, Item{Title: fmt.Sprintf("Start %d min focus", m), ID: IDStart + strconv.Itoa(m)})
		}
	}
	out = append(out, sep)
	if s.Access == app.NotifyDenied {
		out = append(out, Item{Title: "Notifications are off…", ID: IDSettings})
	}
	return append(out, Item{Title: "Quit tock menu bar", ID: IDQuit})
}

// startMinutes parses a start item id.
func startMinutes(id string) (int, bool) {
	v, ok := strings.CutPrefix(id, IDStart)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
