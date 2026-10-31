// Package bar is the logic of the tock menu bar app: what its title and
// menu show, which buttons a notification gets and what each click does.
// The Cocoa side in cmd/tockbar only draws and forwards clicks.
package bar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// Menu item ids.
const (
	IDStart    = "start:" // + minutes
	IDStop     = "stop"
	IDSettings = "settings"
	IDQuit     = "quit"
	// The last session's items carry its id, because the menu is drawn from
	// a snapshot up to reloadEvery old: a click must act on the session the
	// menu showed, not on one that finished since.
	IDRate     = "rate:"     // + "<rating 1..5>:<session id>"
	IDRefile   = "refile:"   // + session id
	IDUnrefile = "unrefile:" // + session id
)

// lastWithinMS is how long after it ended a session stays in the menu.
const lastWithinMS = 24 * 60 * 60_000

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
	Live         focus.Live     // its focus level; Level -1 when unknown
	Presets      []int
	Access       string       // notification access the app last saw
	Last         *LastSession // the newest finished session if it ended within 24 hours, else nil
	LastErr      error        // why Last could not be read, for the caller to log; the rest of the snapshot is still good
}

// LastSession is the finished session the menu offers to rate and refile.
type LastSession struct {
	Detail report.SessionDetail
	Undo   bool           // the newest change is this session's refile
	Refile bool           // a refile to the top project would change something and can be chosen
	Loc    *time.Location // zone for the time range
}

// Load reads today's snapshot. access is the app's notification access.
func Load(a *app.App, now time.Time, access string) (Snapshot, error) {
	a.Reload() // tockbar is long-running: pick up config and rules edits
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
	s.Session, s.Live = f.Open, f.Live
	s.Last, s.LastErr = loadLast(a, now)
	return s, nil
}

// loadLast reads the last-session row. A session deleted meanwhile is no
// row and no error; any other failure is returned for the caller to log, so
// it never costs the rest of the snapshot.
func loadLast(a *app.App, now time.Time) (*LastSession, error) {
	last, err := a.LastSession()
	if err != nil {
		return nil, fmt.Errorf("bar: last session: %w", err)
	}
	if last == nil || now.UnixMilli()-last.EndMS > lastWithinMS {
		return nil, nil
	}
	detail, err := a.SessionDetail(last.ID)
	if errors.Is(err, store.ErrSessionGone) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bar: last session detail: %w", err)
	}
	undo, err := a.RefileUndoable(last.ID)
	if err != nil {
		return nil, fmt.Errorf("bar: last session undo: %w", err)
	}
	_, unresolved := a.Rules().ResolveProject(detail.Top.Project)
	return &LastSession{Detail: detail, Undo: undo, Refile: detail.Top.Refile() && unresolved == nil, Loc: now.Location()}, nil
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
// rows have nothing else. A row with Sub is a submenu parent: it has no ID
// and Sub are its rows. Checked draws a check mark.
type Item struct {
	Title     string
	ID        string
	Separator bool
	Checked   bool
	Sub       []Item
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
		lv := ""
		if s.Live.Level >= 0 {
			lv = report.LevelText(s.Live) + " · "
		}
		out = append(out, label("Session · %s left · %s%d %s", report.Countdown(ss.PlannedMS-nowMS), lv, ss.Interruptions, word))
	}
	if s.Last != nil {
		out = append(out, s.Last.item())
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

// item is the "Last session" row with its rate, refile and undo submenu.
func (l *LastSession) item() Item {
	d, id := l.Detail, strconv.FormatInt(l.Detail.Sess.ID, 10)
	title := fmt.Sprintf("Last session · %s · Q%d", report.Dur(d.Sess.EndMS-d.Sess.StartMS), d.Q.Score)
	if d.Sess.Kind == focus.KindAuto {
		title += " · auto"
	}
	if d.Sess.Rating > 0 {
		title += fmt.Sprintf(" · ★%d", d.Sess.Rating)
	}
	sub := []Item{label("%s", d.Summary(l.Loc)), label("%s", d.Q.Line())}
	if d.Sess.Note != "" {
		sub = append(sub, label("Note: %s", report.Cut(d.Sess.Note, noteCells)))
	}
	sub = append(sub, sep)
	for r := 5; r >= 1; r-- {
		sub = append(sub, Item{Title: fmt.Sprintf("Rate ★%d", r), ID: IDRate + strconv.Itoa(r) + ":" + id, Checked: r == d.Sess.Rating})
	}
	var acts []Item
	if l.Refile {
		acts = append(acts, Item{Title: "Refile to " + report.Cut(d.Top.Project, ProjectCells), ID: IDRefile + id})
	}
	if l.Undo {
		acts = append(acts, Item{Title: "Undo refile", ID: IDUnrefile + id})
	}
	if len(acts) > 0 {
		sub = append(append(sub, sep), acts...)
	}
	return Item{Title: title, Sub: sub}
}

// noteCells is how wide a note may be in the menu.
const noteCells = 60

// startMinutes parses a start item id.
func startMinutes(id string) (int, bool) {
	v, ok := strings.CutPrefix(id, IDStart)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
