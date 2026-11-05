package main

import (
	"cmp"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/bar"
	"tock/internal/classify"
	"tock/internal/focus"
)

const (
	// beatEvery is how often the app checks its notification access and
	// tells trackd it is alive (well inside app.BarFreshMS).
	beatEvery = 5 * time.Second
	// reloadEvery is how often the day summary is read again.
	reloadEvery = 30 * time.Second
	// reopenEvery is how often a failed open is retried.
	reopenEvery = 30 * time.Second
	// keepCategories is how many notification categories stay registered.
	keepCategories = 8
)

// defaultAction is the action of a click on a notification itself.
const defaultAction = "com.apple.UNNotificationDefaultActionIdentifier"

// ui is the Cocoa side, faked in tests.
type ui interface {
	SetTitle(title string)
	SetMenu(spec string)
	Post(id int64, title, body, category, categories string)
	// Curtain shows the break screen of notice id over every screen until a
	// button (answered through evResponse) or Esc closes it: fromMS is when
	// the break began, untilMS its planned end, buttons "id\ttitle…".
	Curtain(id int64, title, body, word string, fromMS, untilMS int64, buttons string)
	CheckAuth(prompt bool)
	OpenURL(url string)
	Quit()
}

type eventKind int

const (
	evReady eventKind = iota
	evMenu
	evResponse
	evAuth
	evPostFailed
)

// event is a click or a callback from the Cocoa side.
type event struct {
	kind   eventKind
	id     string // menu id, notification action, access or error text
	notice int64
}

// runner owns the app session and drives the UI from one goroutine.
type runner struct {
	open   func() (*app.App, error)
	now    func() time.Time
	log    *slog.Logger
	prompt bool
	ui     ui
	rules  string // rules.toml, which Set up tock writes
	dash   *dash

	a        *app.App
	openedAt time.Time // last open attempt
	snap     bar.Snapshot
	snapAt   time.Time
	dirty    bool
	access   string
	beatAt   time.Time
	asked    bool
	title    string
	menu     string
	cats     []string // registered category lines, newest last
	lastErr  string
}

// loop runs until events closes.
func (r *runner) loop(events <-chan event) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	ready := false
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				r.close()
				return
			}
			if ev.kind == evReady {
				ready = true
			}
			if ready {
				r.handle(ev)
			}
		case <-t.C:
			if ready {
				r.tick()
			}
		}
	}
}

func (r *runner) close() {
	r.dash.Close()
	if r.a != nil {
		r.a.Close()
		r.a = nil
	}
}

func (r *runner) warn(msg string, err error) {
	if s := msg + ": " + err.Error(); s != r.lastErr {
		r.lastErr = s
		r.log.Warn(msg, "err", err)
	}
}

// tick runs once a second: heartbeat, notices, then the title and menu.
func (r *runner) tick() {
	now := r.now()
	if r.a == nil && (r.openedAt.IsZero() || now.Sub(r.openedAt) >= reopenEvery) {
		r.openedAt = now
		a, err := r.open()
		if err != nil {
			r.warn("tock not ready", err)
			r.snap = bar.NotReady(err)
		} else {
			r.a, r.dirty, r.lastErr = a, true, ""
			r.log.Info("tock opened")
		}
	}
	if r.beatAt.IsZero() || now.Sub(r.beatAt) >= beatEvery {
		r.beatAt = now
		r.ui.CheckAuth(r.prompt && !r.asked)
		r.asked = true
		if r.a != nil {
			access := r.access
			if access == "" {
				access = app.NotifyUnknown
			}
			if err := r.a.BarBeat(access); err != nil {
				r.warn("heartbeat failed", err)
			}
		}
	}
	if r.a != nil {
		r.post()
		if r.dirty || now.Sub(r.snapAt) >= reloadEvery {
			s, err := bar.Load(r.a, now, r.access)
			if err != nil {
				r.warn("reading today failed", err)
			} else {
				r.snap, r.snapAt, r.dirty = s, now, false
				if s.LastErr != nil {
					r.warn("reading the last session failed", s.LastErr)
				}
				if s.SlotsErr != nil {
					r.warn("reading the plan and calendar failed", s.SlotsErr)
				}
				if s.RecErr != nil {
					r.warn("reading the pending suggestion failed", s.RecErr)
				}
				if s.DistractErr != nil {
					r.warn("reading the last distraction notice failed", s.DistractErr)
				}
				if s.PrivacyErr != nil {
					r.warn("reading the privacy menu failed", s.PrivacyErr)
				}
			}
		}
	}
	r.snap.Access = r.access
	r.render(now)
}

// post shows the queued notices.
func (r *runner) post() {
	ns, err := r.a.TakeNotices()
	if err != nil {
		r.warn("reading notices failed", err)
		return
	}
	for _, n := range ns {
		_, d, err := r.a.Notice(n.ID)
		if err != nil {
			r.warn("reading a notice failed", err)
			continue
		}
		if (n.Topic == focus.TopicBreakScreen || n.Topic == focus.TopicWake) && d.From > 0 {
			var bs []string
			for _, b := range bar.Buttons(n, d) {
				bs = append(bs, b.ID, clean(b.Title))
			}
			word, until, what := "away", d.To, "break screen"
			if n.Topic == focus.TopicWake {
				word, until, what = "off task", d.From, "wake screen"
			}
			r.ui.Curtain(n.ID, clean(n.Title), clean(n.Body), word, d.From, until, strings.Join(bs, "\t"))
			r.log.Info(what + " shown")
			continue
		}
		cat := bar.Category(n, d)
		if cat != "" {
			line := []string{cat}
			for _, b := range bar.Buttons(n, d) {
				line = append(line, b.ID, clean(b.Title))
			}
			r.cats = append(r.cats, strings.Join(line, "\t"))
			if len(r.cats) > keepCategories {
				r.cats = r.cats[len(r.cats)-keepCategories:]
			}
		}
		r.ui.Post(n.ID, n.Title, n.Body, cat, strings.Join(r.cats, "\n"))
		r.log.Info("notification shown", "title", n.Title)
		// TakeNotices hands each notice out once, so this opens once per notice.
		// An invite the owner has not accepted offers Join but never opens by itself.
		if link := bar.JoinURL(n, d); link != "" && n.Topic == focus.TopicStarting && d.Mine && r.a.Config().Calendar.AutoOpen {
			r.ui.OpenURL(link)
			r.log.Info("meeting link opened", "notice", n.ID)
		}
	}
}

func (r *runner) render(now time.Time) {
	nowMS := now.UnixMilli()
	if t := r.snap.Title(nowMS); t != r.title {
		r.title = t
		r.ui.SetTitle(t)
	}
	if m := strings.Join(specLines(r.snap.Items(nowMS), 0, nil), "\n"); m != r.menu {
		r.menu = m
		r.ui.SetMenu(m)
	}
}

// specLines flattens items into menu spec lines: one ">" per depth, then "-"
// for a separator, else "id\ttitle" with a third field "c" for a check mark;
// a strip row is "\t\t\ts:k1,k2,…" (an unlabelled cell is "none").
// A submenu parent is written first, with no id, then its rows one deeper.
func specLines(items []bar.Item, depth int, lines []string) []string {
	prefix := strings.Repeat(">", depth)
	for _, it := range items {
		if it.Separator {
			lines = append(lines, prefix+"-")
			continue
		}
		if len(it.Strip) > 0 {
			ks := make([]string, len(it.Strip))
			for i, k := range it.Strip {
				ks[i] = cmp.Or(clean(k), "none")
			}
			lines = append(lines, prefix+"\t\t\ts:"+strings.Join(ks, ","))
			continue
		}
		line := prefix + it.ID + "\t" + clean(it.Title)
		if it.Checked {
			line += "\tc"
		}
		lines = append(lines, line)
		if len(it.Sub) > 0 {
			lines = specLines(it.Sub, depth+1, lines)
		}
	}
	return lines
}

// clean keeps tabs and newlines out of a UI spec field.
func clean(s string) string { return strings.NewReplacer("\t", " ", "\n", " ").Replace(s) }

func (r *runner) handle(ev event) {
	switch ev.kind {
	case evReady:
		r.log.Info("menu bar ready")
		r.tick()
	case evAuth:
		if ev.id != r.access {
			r.log.Info("notification access", "access", ev.id)
		}
		r.access = ev.id
	case evPostFailed:
		r.log.Warn("notification failed", "notice", ev.notice, "err", ev.id)
	case evMenu:
		switch ev.id {
		case bar.IDQuit:
			r.log.Info("quit from the menu")
			r.ui.Quit()
			return
		case bar.IDNotifySettings:
			r.ui.OpenURL(bar.SettingsURL)
			return
		case bar.IDAccess:
			r.ui.OpenURL(bar.AccessibilityURL)
			return
		case bar.IDDashboard, bar.IDInbox:
			view := ""
			if ev.id == bar.IDInbox {
				view = "inbox"
			}
			link, err := r.dash.URL(view)
			if err == nil {
				r.ui.OpenURL(link)
			}
			r.done("menu "+ev.id, "opened the dashboard", err)
			return
		case bar.IDSetup:
			err := classify.WriteStarter(r.rules)
			if errors.Is(err, os.ErrExist) {
				err = nil // set up meanwhile: just open it
			}
			r.openedAt = time.Time{} // reopen now, not in reopenEvery
			r.done("menu setup", "wrote the starter rules", err)
			return
		}
		if strings.HasPrefix(ev.id, bar.IDJoin) {
			// The runner opens the link; it never reaches bar.Menu.
			msg := "that meeting is no longer offered"
			if link, ok := r.snap.JoinURL(ev.id, r.now().UnixMilli()); ok {
				r.ui.OpenURL(link)
				msg = "opened the meeting link"
			}
			r.done("menu join", msg, nil)
			return
		}
		if r.a == nil {
			return
		}
		msg, err := bar.Menu(r.a, ev.id)
		r.done("menu "+ev.id, msg, err)
	case evResponse:
		if r.a == nil {
			return
		}
		if ev.id == defaultAction {
			// A click on the notice itself: the recap opens Today.
			if n, _, err := r.a.Notice(ev.notice); err == nil && n.Topic == focus.TopicRecap {
				link, err := r.dash.URL("")
				if err == nil {
					r.ui.OpenURL(link)
				}
				r.done("recap", "opened the dashboard", err)
			}
			return
		}
		if ev.id == bar.ActJoin {
			n, d, err := r.a.Notice(ev.notice)
			if err != nil {
				r.warn("reading a notice failed", err)
			} else if link := bar.JoinURL(n, d); link != "" {
				r.ui.OpenURL(link)
			}
		}
		msg, err := bar.Respond(r.a, ev.notice, ev.id)
		r.done("notification "+strconv.FormatInt(ev.notice, 10)+" "+ev.id, msg, err)
	}
}

// done logs an action's result and refreshes the menu.
func (r *runner) done(what, msg string, err error) {
	switch {
	case err != nil && !errors.Is(err, bar.ErrUnknown):
		r.log.Warn("action failed", "action", what, "err", err)
	case err != nil:
		r.log.Warn("unknown action", "action", what)
	case msg != "":
		r.log.Info("action", "action", what, "result", msg)
	}
	r.dirty = true
	r.tick()
}
