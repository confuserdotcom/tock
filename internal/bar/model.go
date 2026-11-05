// Package bar is the logic of the tock menu bar app: what its title and
// menu show, which buttons a notification gets and what each click does.
// The Cocoa side in cmd/tockbar only draws and forwards clicks.
package bar

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/recommend"
	"tock/internal/report"
	"tock/internal/span"
	"tock/internal/store"
)

// Menu item ids.
const (
	IDStart = "start:" // + minutes
	IDStop  = "stop"
	// IDNotifySettings opens the Notifications pane of System Settings.
	IDNotifySettings = "notify-settings"
	// IDFeature (+ a config.Features id) turns that feature on or off.
	IDFeature = "feature:"
	// IDHide and IDUnhide (+ "app:<bundle id>" or "site:<host>") add to and
	// remove from the [privacy] hide lists.
	IDHide   = "hide:"
	IDUnhide = "unhide:"
	IDQuit   = "quit"
	// IDSnoozeDistract snoozes the running session's distraction notices
	// for [focus] snooze minutes; IDSnoozeOff resumes them.
	IDSnoozeDistract = "snooze:distract"
	IDSnoozeOff      = "snooze:off"
	// The last session's items carry its id, because the menu is drawn from
	// a snapshot up to reloadEvery old: a click must act on the session the
	// menu showed, not on one that finished since.
	IDRate     = "rate:"     // + "<rating 1..5>:<session id>"
	IDRefile   = "refile:"   // + session id
	IDUnrefile = "unrefile:" // + session id
	// IDJoin (+ the slot's start, unix ms) opens the meeting link of the
	// event the menu offers to join. The id carries the start so a click on
	// a menu drawn from an older snapshot opens nothing new.
	IDJoin = "join:"
	// The suggestion items carry the row id and its send time ("<id>:<at ms>"):
	// the snapshot is up to reloadEvery old and row ids can be reused, so a
	// click must answer the row the menu showed, never a newer one.
	IDRecAccept  = "rec:accept:"  // + "<recommend id>:<sent at, unix ms>"
	IDRecDismiss = "rec:dismiss:" // + "<recommend id>:<sent at, unix ms>"
	// The menu bar app handles these itself; they never reach Menu.
	IDDashboard = "dash"       // opens the web dashboard
	IDInbox     = "dash:inbox" // opens the dashboard on the inbox
	IDSetup     = "setup"      // writes the starter rules
	IDAccess    = "ax"         // opens the Accessibility pane of System Settings
)

const (
	// nextWithinMS is how far ahead a slot shows as the Next row.
	nextWithinMS = 60 * 60_000
	// joinWithinMS is how far ahead an event with a meeting link shows its
	// Join item; a running one shows it too.
	joinWithinMS = 10 * 60_000
)

// lastWithinMS is how long after it ended a session stays in the menu.
const lastWithinMS = 24 * 60 * 60_000

// lastFreshMS is how long a rated, settled session stays in the menu.
const lastFreshMS = 60 * 60_000

// snoozeShowMS is how long after a distraction notice the menu offers to
// snooze the next ones.
const snoozeShowMS = 15 * 60_000

// SettingsURL opens the Notifications pane of System Settings.
const SettingsURL = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"

// AccessibilityURL opens Privacy & Security › Accessibility in System Settings.
const AccessibilityURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

// maxPresets is how many start items the menu offers.
const maxPresets = 3

// Snapshot is what the menu bar shows, read from tock's database.
type Snapshot struct {
	OK           bool  // false when tock could not open (no rules or database yet)
	Why          error // why it could not open, when !OK; nil = unknown
	AXDenied     bool  // trackd reported Accessibility is not granted
	TrackedMS    int64
	FocusMS      int64
	Score        int
	Active       bool // the day has activity, so the score means something
	UnlabelledMS int64
	TrackdUp     bool
	Session      *focus.Session // the running session, or nil
	Live         focus.Live     // its focus level; Level -1 when unknown
	SnoozeUntil  time.Time      // distraction notices are snoozed until then; zero = not snoozed
	SnoozeMin    int            // the minutes the snooze item offers
	Presets      []int
	Access       string         // notification access the app last saw
	Last         *LastSession   // the newest finished session if it ended within 24 hours, else nil
	LastErr      error          // why Last could not be read, for the caller to log; the rest of the snapshot is still good
	Slots        []focus.Slot   // today's plan items and calendar events, start order
	SlotsErr     error          // why Slots could not be read, for the caller to log; the rest of the snapshot is still good
	Loc          *time.Location // zone for the clock times of Slots; nil = local
	Rec          *recommend.Rec // the pending recommendation (sent under 30 min ago, no reply), or nil
	RecErr       error          // why Rec could not be read, for the caller to log; the rest of the snapshot is still good
	BreakUntil   time.Time      // the break the user took ends then; zero = none running
	Strip        []string       // today's time as StripCells kinds, for the colour strip; nil = nothing tracked
	DistractAtMS int64          // the running session's newest distraction notice; 0 = none or no session
	DistractErr  error          // why DistractAtMS could not be read, for the caller to log; the rest of the snapshot is still good
	Features     []FeatureState // the toggles of the Settings submenu, in config.Features order
	Privacy      PrivacyState   // the Privacy submenu
	PrivacyErr   error          // why Privacy.Front or a hidden app's name could not be read, for the caller to log
}

// PrivacyState is what the Privacy submenu offers to hide and shows hidden.
type PrivacyState struct {
	Front *store.Active // the newest app that was not idle; nil = none
	Apps  []HiddenApp   // [privacy] hide_apps, in file order
	Sites []string      // [privacy] hide_sites, in file order
}

// HiddenApp is one hidden app: its bundle id and the name tock saw it under
// (the id when it never ran).
type HiddenApp struct{ Bundle, Name string }

// ownBundles are tock's own apps, never offered for hiding.
var ownBundles = map[string]bool{"com.tock.bar": true, "com.tock.trackd": true, span.IdleBundleID: true}

// StripCells is how many blocks the menu's colour strip has.
const StripCells = 24

// FeatureState is one Settings toggle as the config has it now.
type FeatureState struct {
	ID, Group, Name string
	On              bool
}

// LastSession is the finished session the menu offers to rate and refile.
type LastSession struct {
	Detail report.SessionDetail
	Undo   bool           // the newest change is this session's refile
	Refile bool           // a refile to the top project would change something and can be chosen
	Loc    *time.Location // zone for the time range
}

// NotReady is the snapshot shown while tock cannot open, for the reason why.
func NotReady(why error) Snapshot { return Snapshot{Why: why} }

// Load reads today's snapshot. access is the app's notification access.
func Load(a *app.App, now time.Time, access string) (Snapshot, error) {
	a.Reload() // tockbar is long-running: pick up config and rules edits
	s := Snapshot{OK: true, Access: access, Presets: a.Config().Focus.Presets}
	h, err := a.Health()
	if err != nil {
		return s, err
	}
	s.TrackdUp = h.TrackdUp
	s.AXDenied = h.AXTrusted != nil && !*h.AXTrusted
	d, err := a.Day(now)
	if err != nil {
		return s, err
	}
	s.TrackedMS, s.FocusMS, s.UnlabelledMS = d.TrackedMS, d.Stats.FocusMS, d.ByKind[""]
	s.Score, s.Active = d.Score.Value, d.Stats.ActiveMS > 0
	s.Strip = report.Strip(d.ByKind, StripCells)
	f, err := a.Focus(now)
	if err != nil {
		return s, err
	}
	s.Session, s.Live, s.SnoozeMin = f.Open, f.Live, f.SnoozeMin
	if f.SnoozeUntilMS > 0 {
		s.SnoozeUntil = time.UnixMilli(f.SnoozeUntilMS).In(now.Location())
	}
	if s.Session != nil {
		if s.DistractAtMS, err = a.LastNoticeAt(focus.TopicFocus, s.Session.StartMS); err != nil {
			s.DistractAtMS, s.DistractErr = 0, fmt.Errorf("bar: distraction notice: %w", err)
		}
	}
	cfg := a.Config()
	for _, f := range config.Features {
		s.Features = append(s.Features, FeatureState{ID: f.ID, Group: f.Group, Name: f.Name, On: f.On(cfg)})
	}
	s.Privacy, s.PrivacyErr = loadPrivacy(a, cfg.Privacy)
	s.Last, s.LastErr = loadLast(a, now)
	s.Loc = now.Location()
	if s.Slots, err = a.Slots(now); err != nil {
		s.Slots, s.SlotsErr = nil, fmt.Errorf("bar: slots: %w", err)
	}
	if s.Rec, err = a.PendingRecommend(now); err != nil {
		s.Rec, s.RecErr = nil, fmt.Errorf("bar: pending suggestion: %w", err)
	}
	if until, err := a.BreakUntil(now); err != nil {
		s.RecErr = errors.Join(s.RecErr, fmt.Errorf("bar: break: %w", err))
	} else if until > 0 {
		s.BreakUntil = time.UnixMilli(until).In(now.Location())
	}
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
	Strip     []string // a colour strip row: one kind per block ("" = unlabelled); Title is empty
}

func label(f string, a ...any) Item { return Item{Title: fmt.Sprintf(f, a...)} }

var sep = Item{Separator: true}

// Items is the menu at nowMS.
func (s Snapshot) Items(nowMS int64) []Item {
	if !s.OK {
		first := label("tock could not open · see tockbar.log")
		switch {
		case errors.Is(s.Why, classify.ErrNoRules):
			first = Item{Title: "Set up tock", ID: IDSetup}
		case errors.Is(s.Why, app.ErrNoDB):
			first = label("Waiting for trackd to start")
		}
		return []Item{first, sep, {Title: "Quit tock menu bar", ID: IDQuit}}
	}
	groups := [][]Item{s.header(nowMS), s.context(nowMS), s.action(), s.goItems(), {s.settings()}}
	var out []Item
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, sep)
		}
		out = append(out, g...)
	}
	return out
}

// header is what is going on: today's totals, the strip, the running
// session and anything broken.
func (s Snapshot) header(nowMS int64) []Item {
	line := report.Dur(s.TrackedMS) + " today"
	if s.Active {
		line += fmt.Sprintf(" · %s focus · score %d", report.Dur(s.FocusMS), s.Score)
	}
	out := []Item{{Title: line}}
	if len(s.Strip) > 0 {
		out = append(out, Item{Strip: s.Strip})
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
		if s.snoozed(nowMS) {
			out = append(out, label("Notices snoozed until %s", s.SnoozeUntil.Format("15:04")))
		}
	}
	if !s.TrackdUp {
		out = append(out, label("trackd is not running"))
	}
	if s.AXDenied {
		out = append(out, Item{Title: "Allow Accessibility for trackd…", ID: IDAccess})
	}
	if s.Access == app.NotifyDenied {
		out = append(out, Item{Title: "Notifications are off…", ID: IDNotifySettings})
	}
	return out
}

// snoozed: distraction notices are snoozed at nowMS.
func (s Snapshot) snoozed(nowMS int64) bool {
	return !s.SnoozeUntil.IsZero() && s.SnoozeUntil.UnixMilli() > nowMS
}

// context is what matters right now: the next slot, a break, a suggestion,
// the answer to a distraction notice and the last session.
func (s Snapshot) context(nowMS int64) []Item {
	out := append(s.slotItems(nowMS), s.recItems(nowMS)...)
	if s.Session != nil {
		switch {
		case s.snoozed(nowMS):
			out = append(out, Item{Title: "Resume notices", ID: IDSnoozeOff})
		case s.DistractAtMS > 0 && nowMS-s.DistractAtMS <= snoozeShowMS:
			out = append(out, Item{Title: fmt.Sprintf("Snooze distraction notices (%d min)", s.SnoozeMin), ID: IDSnoozeDistract})
		}
	}
	if l := s.Last; l != nil && l.shown(nowMS) {
		out = append(out, l.item())
	}
	return out
}

// action is the one thing to do: stop the session, or start one with the
// first preset; the other presets wait in More lengths.
func (s Snapshot) action() []Item {
	if s.Session != nil {
		return []Item{{Title: "Stop session", ID: IDStop}}
	}
	ps := s.Presets[:min(len(s.Presets), maxPresets)]
	if len(ps) == 0 {
		return nil
	}
	out := []Item{startItem(ps[0])}
	if len(ps) > 1 {
		more := Item{Title: "More lengths"}
		for _, m := range ps[1:] {
			more.Sub = append(more.Sub, startItem(m))
		}
		out = append(out, more)
	}
	return out
}

func startItem(m int) Item {
	return Item{Title: fmt.Sprintf("Start %d min focus", m), ID: IDStart + strconv.Itoa(m)}
}

// goItems open the dashboard, on the inbox when something is unlabelled.
func (s Snapshot) goItems() []Item {
	var out []Item
	if s.UnlabelledMS > 0 {
		out = append(out, Item{Title: "Inbox · " + report.Dur(s.UnlabelledMS), ID: IDInbox})
	}
	return append(out, Item{Title: "Open dashboard", ID: IDDashboard})
}

// loadPrivacy reads the Privacy submenu's state; on an error the rest of
// it is still filled.
func loadPrivacy(a *app.App, p config.Privacy) (PrivacyState, error) {
	ps := PrivacyState{Sites: p.HideSites}
	var errs []error
	for _, b := range p.HideApps {
		n, err := a.AppName(b)
		if err != nil {
			errs = append(errs, err)
		}
		ps.Apps = append(ps.Apps, HiddenApp{Bundle: b, Name: cmp.Or(n, b)})
	}
	f, err := a.LastActive()
	if err != nil {
		errs = append(errs, err)
	}
	ps.Front = f
	if err := errors.Join(errs...); err != nil {
		return ps, fmt.Errorf("bar: privacy: %w", err)
	}
	return ps, nil
}

// privacy is the Privacy submenu: hide the newest app or its site, and the
// hidden ones, checked; a click on one shows it again.
func (s Snapshot) privacy() Item {
	ps := s.Privacy
	p := config.Privacy{HideSites: ps.Sites}
	for _, h := range ps.Apps {
		p.HideApps = append(p.HideApps, h.Bundle)
	}
	var sub []Item
	if f := ps.Front; f != nil && !ownBundles[f.Bundle] && !p.Hides(f.Bundle, nil) {
		sub = append(sub, Item{Title: "Hide " + f.App, ID: IDHide + config.HideApp + ":" + f.Bundle})
		if h := config.Host(f.URL); h != "" && !p.Hides("", f.URL) {
			sub = append(sub, Item{Title: "Hide " + h, ID: IDHide + config.HideSite + ":" + h})
		}
	}
	if len(sub) > 0 && len(ps.Apps)+len(ps.Sites) > 0 {
		sub = append(sub, sep)
	}
	for _, h := range ps.Apps {
		sub = append(sub, Item{Title: h.Name, ID: IDUnhide + config.HideApp + ":" + h.Bundle, Checked: true})
	}
	for _, h := range ps.Sites {
		sub = append(sub, Item{Title: h, ID: IDUnhide + config.HideSite + ":" + h, Checked: true})
	}
	if len(sub) == 0 {
		sub = append(sub, label("Nothing to hide yet"))
	}
	return Item{Title: "Privacy", Sub: sub}
}

// settings is the Settings submenu: the feature toggles by group, the
// System Settings panes and Quit.
func (s Snapshot) settings() Item {
	var sub []Item
	group := ""
	for _, f := range s.Features {
		if f.Group != group {
			if group != "" {
				sub = append(sub, sep)
			}
			group = f.Group
			sub = append(sub, label("%s", f.Group))
		}
		sub = append(sub, Item{Title: f.Name, ID: IDFeature + f.ID, Checked: f.On})
	}
	if len(sub) > 0 {
		sub = append(sub, sep)
	}
	sub = append(sub,
		s.privacy(),
		Item{Title: "Notifications…", ID: IDNotifySettings},
		Item{Title: "Accessibility…", ID: IDAccess},
		sep,
		Item{Title: "Quit tock menu bar", ID: IDQuit})
	return Item{Title: "Settings", Sub: sub}
}

// slotItems are the Next row and the Join item at nowMS, in that order; none
// when no slot is near.
func (s Snapshot) slotItems(nowMS int64) []Item {
	var out []Item
	loc := s.Loc
	if loc == nil {
		loc = time.Local
	}
	for _, sl := range s.Slots {
		if sl.EndMS <= nowMS || sl.StartMS >= nowMS+nextWithinMS {
			continue
		}
		when := "now"
		if sl.StartMS > nowMS {
			when = fmt.Sprintf("in %d min", max((sl.StartMS-nowMS+59_999)/60_000, 1))
		}
		out = append(out, label("Next: %s %s (%s)", time.UnixMilli(sl.StartMS).In(loc).Format("15:04"),
			focus.SlotName(sl, ProjectCells), when))
		break
	}
	if sl, ok := s.joinSlot(nowMS); ok {
		out = append(out, Item{Title: "Join " + focus.SlotName(sl, ProjectCells), ID: IDJoin + strconv.FormatInt(sl.StartMS, 10)})
	}
	return out
}

// recItems are the rows of the recommendations at nowMS: the Break row while a
// break the user took runs, then the pending suggestion's label, accept item
// and Not now. No suggestion rows while a session runs (a focus session cannot start
// then), without a suggestion, or once it is no longer open: the snapshot can
// be older than the 30 minutes the row stays open.
func (s Snapshot) recItems(nowMS int64) []Item {
	var out []Item
	if !s.BreakUntil.IsZero() && s.BreakUntil.UnixMilli() > nowMS {
		loc := s.Loc
		if loc == nil {
			loc = time.Local
		}
		out = append(out, label("Break · until %s", s.BreakUntil.In(loc).Format("15:04")))
	}
	r := s.Rec
	if s.Session != nil || r == nil || !r.Open(nowMS) {
		return out
	}
	key := strconv.FormatInt(r.ID, 10) + ":" + strconv.FormatInt(r.AtMS, 10)
	return append(out,
		label("Suggested: %s", report.RecText(*r)),
		Item{Title: RecAcceptTitle(r.Kind, int(r.LengthMS/60_000)), ID: IDRecAccept + key},
		Item{Title: "Not now", ID: IDRecDismiss + key},
	)
}

// recKey parses the "<recommend id>:<sent at ms>" of a suggestion item id.
func recKey(v string) (id, atMS int64, ok bool) {
	a, b, found := strings.Cut(v, ":")
	if !found {
		return 0, 0, false
	}
	id, ok1 := sessionID(a)
	atMS, ok2 := sessionID(b)
	return id, atMS, ok1 && ok2
}

// joinSlot is the event the menu offers to join at nowMS: the first one with
// an accepted meeting link that has not ended and starts within joinWithinMS.
func (s Snapshot) joinSlot(nowMS int64) (focus.Slot, bool) {
	for _, sl := range s.Slots {
		if sl.Kind == focus.SlotEvent && classify.IsMeetingLink(sl.JoinURL) && sl.EndMS > nowMS && sl.StartMS < nowMS+joinWithinMS {
			return sl, true
		}
	}
	return focus.Slot{}, false
}

// JoinURL is the link the Join item with id opens at nowMS: only when that
// item is on the menu now. The link is checked again here. ok is false for
// any other id, for an event no longer offered, and for a link that is not
// an accepted meeting link.
func (s Snapshot) JoinURL(id string, nowMS int64) (string, bool) {
	sl, ok := s.joinSlot(nowMS)
	if !ok || !strings.HasPrefix(id, IDJoin) || id != IDJoin+strconv.FormatInt(sl.StartMS, 10) {
		return "", false
	}
	return sl.JoinURL, true
}

// shown: the last session stays in the menu while there is something to do
// with it: it is unrated, it just ended, or a refile or undo is offered.
func (l *LastSession) shown(nowMS int64) bool {
	return l.Detail.Sess.Rating == 0 || nowMS-l.Detail.Sess.EndMS <= lastFreshMS || l.Refile || l.Undo
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
	if line := d.UrgeLine(); line != "" {
		sub = append(sub, label("%s", line))
	}
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
