package main

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/bar"
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

// ui is the Cocoa side, faked in tests.
type ui interface {
	SetTitle(title string)
	SetMenu(spec string)
	Post(id int64, title, body, category, categories string)
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
			r.snap = bar.Snapshot{}
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
	}
}

func (r *runner) render(now time.Time) {
	nowMS := now.UnixMilli()
	if t := r.snap.Title(nowMS); t != r.title {
		r.title = t
		r.ui.SetTitle(t)
	}
	var lines []string
	for _, it := range r.snap.Items(nowMS) {
		if it.Separator {
			lines = append(lines, "-")
			continue
		}
		lines = append(lines, it.ID+"\t"+clean(it.Title))
	}
	if m := strings.Join(lines, "\n"); m != r.menu {
		r.menu = m
		r.ui.SetMenu(m)
	}
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
		case bar.IDSettings:
			r.ui.OpenURL(bar.SettingsURL)
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
