package main

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/bar"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
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
	// missedKeepMS is how long a session end or recap that folded unseen
	// waits in the notch.
	missedKeepMS = 3 * 60 * 60_000
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
	// Prompt shows the start prompt: "start\ttitle\tdetail\t<min>,<min>…"
	// or "stop\ttitle\tdetail"; the answer comes back as a menu click.
	Prompt(spec string)
	SetHotkey(on bool) // ⌥⌘F opens the start prompt
	// Island draws the notch from bar.Island JSON; "" hides it.
	Island(spec string)
	// NotchOptions sets how the notch panel behaves: seen by screen sharing
	// and recordings, drawn on every display, opened by resting the pointer.
	NotchOptions(sharing, allDisplays, openOnHover bool)
	Quit()
}

type eventKind int

const (
	evReady eventKind = iota
	evMenu
	evResponse
	evAuth
	evPostFailed
	evNotch // the Cocoa side made its notch panel ("1") or cannot ("0")
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
	hotkey   *bool // what SetHotkey was last told; nil = never

	notchUI bool        // the Cocoa side has a notch panel
	moment  *bar.Island // the open notch moment, nil = none
	labels  []bar.Entry // the labelling round the moment shows
	labelAt int
	island  string // what Island was last told
	options string // what NotchOptions was last told; "" = never
	// missed is a session end or recap that folded unseen; it waits in the
	// quiet notch until opened, replaced or missedKeepMS old.
	missed   *bar.Island
	missedMS int64
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
				if on := featureOn(s, "hotkey"); r.hotkey == nil || *r.hotkey != on {
					r.hotkey = &on
					r.ui.SetHotkey(on)
				}
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
		if r.notchOn() && bar.NotchTopic(n.Topic) && r.toNotch(n, d) {
			r.autoOpen(n, d)
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
		r.autoOpen(n, d)
	}
}

// autoOpen opens the meeting link of a starting event when [calendar]
// auto_open asks. TakeNotices hands each notice out once, so this opens once
// per notice. An invite the owner has not accepted offers Join but never
// opens by itself.
func (r *runner) autoOpen(n store.Notice, d app.NoticeData) {
	if link := bar.JoinURL(n, d); link != "" && n.Topic == focus.TopicStarting && d.Mine && r.a.Config().Calendar.AutoOpen {
		r.ui.OpenURL(link)
		r.log.Info("meeting link opened", "notice", n.ID)
	}
}

// notchOn reports whether notices and the running state go to the notch.
func (r *runner) notchOn() bool { return r.notchUI && r.a != nil && r.a.Config().Notch.Enabled }

// toNotch opens the moment of notice n. false means it still needs a banner:
// a moment that ranks higher is open, or the moment could not be read.
func (r *runner) toNotch(n store.Notice, d app.NoticeData) bool {
	i, ok, err := bar.Moment(r.a, n, d, r.now(), r.snap.UnlabelledMS)
	if err != nil {
		r.warn("reading a notch moment failed", err)
	}
	if !ok {
		return err == nil // an up-next notice: the compact island shows it
	}
	if r.moment != nil && bar.Rank(r.moment.State) > bar.Rank(i.State) {
		return false
	}
	r.moment, r.labels, r.missed = &i, nil, nil
	r.log.Info("notch moment", "state", i.State, "notice", n.ID)
	return true
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
	r.renderNotch(nowMS)
}

// renderNotch draws the open moment, else the running state, in the notch.
func (r *runner) renderNotch(nowMS int64) {
	if !r.notchUI {
		return
	}
	spec := ""
	if r.notchOn() && r.snap.OK {
		c := r.a.Config().Notch
		if o := fmt.Sprint(c.ShowInRecordings, c.AllDisplays, c.OpenOnHover); o != r.options {
			r.options = o
			r.ui.NotchOptions(c.ShowInRecordings, c.AllDisplays, c.OpenOnHover)
		}
		if r.missed != nil && nowMS-r.missedMS > missedKeepMS {
			r.missed = nil
		}
		i := r.snap.IslandBase(nowMS)
		switch {
		case r.moment != nil:
			i = *r.moment
			r.moment.Haptic = "" // played once
		case r.missed != nil:
			i = bar.Missed(i, *r.missed)
		}
		spec = i.JSON()
	} else {
		r.moment, r.labels, r.missed = nil, nil, nil
	}
	if spec != r.island {
		r.island = spec
		r.ui.Island(spec)
	}
}

// notch handles a click in the notch panel.
func (r *runner) notch(id string) {
	if r.a == nil {
		return
	}
	switch {
	case id == bar.IDNotchClose:
		r.moment, r.labels = nil, nil
		r.done("notch close", "", nil)
	case strings.HasPrefix(id, bar.IDNotchExpire):
		if m := r.moment; m != nil && strconv.FormatInt(m.Notice, 10) == strings.TrimPrefix(id, bar.IDNotchExpire) {
			if m.State == bar.IslandDone || m.State == bar.IslandRecap {
				r.missed, r.missedMS = m, r.now().UnixMilli()
			}
			r.moment, r.labels = nil, nil
		}
		r.done("notch expire", "", nil)
	case id == bar.IDNotchSeen:
		r.missed = nil
		r.done("notch seen", "", nil)
	case id == bar.IDNotchLabel:
		es, err := bar.LoadLabels(r.a, r.now())
		if err == nil {
			r.missed = nil
			r.labels, r.labelAt = es, 0
			i := bar.LabelIsland(es, 0, "")
			r.moment = &i
		}
		r.done("notch label", fmt.Sprintf("%d to label", len(es)), err)
	case strings.HasPrefix(id, bar.IDNotchPick), strings.HasPrefix(id, bar.IDNotchSkip):
		r.labelStep(id)
	case strings.HasPrefix(id, bar.IDNotchAct):
		v := strings.TrimPrefix(id, bar.IDNotchAct)
		sid, action, ok := strings.Cut(v, ":")
		nid, err := strconv.ParseInt(sid, 10, 64)
		if !ok || err != nil {
			r.log.Warn("unknown notch action", "id", id)
			return
		}
		r.moment, r.labels, r.missed = nil, nil, nil
		r.handle(event{kind: evResponse, notice: nid, id: action})
	case strings.HasPrefix(id, bar.IDNotchMenu):
		r.moment, r.labels, r.missed = nil, nil, nil
		r.handle(event{kind: evMenu, id: strings.TrimPrefix(id, bar.IDNotchMenu)})
	default:
		r.log.Warn("unknown notch action", "id", id)
	}
}

// labelStep picks or skips the shown entry. The id names the entry, so a
// click on a panel drawn before the last step does nothing.
func (r *runner) labelStep(id string) {
	pick := strings.HasPrefix(id, bar.IDNotchPick)
	v := strings.TrimPrefix(strings.TrimPrefix(id, bar.IDNotchPick), bar.IDNotchSkip)
	se, so, _ := strings.Cut(v, ":")
	i, err := strconv.Atoi(se)
	if r.moment == nil || r.moment.State != bar.IslandLabel || err != nil || i != r.labelAt || i >= len(r.labels) {
		r.log.Warn("stale label click", "id", id)
		return
	}
	haptic, msg := "", "skipped"
	if pick {
		j, err := strconv.Atoi(so)
		e := &r.labels[i]
		if err != nil || j < 0 || j >= len(e.Opts) {
			r.log.Warn("unknown label option", "id", id)
			return
		}
		saved, err := bar.Pick(r.a, *e, e.Opts[j])
		if err != nil {
			r.done("notch pick", "", err)
			e.Saved = "Not saved: " + err.Error()
		} else {
			e.Saved, e.Picked, haptic, msg = saved, j, bar.HapticAlignment, saved
		}
	}
	r.labelAt++
	m := bar.LabelIsland(r.labels, r.labelAt, haptic)
	r.moment = &m
	r.done("notch label", msg, nil)
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
	case evNotch:
		r.notchUI = ev.id == "1"
		r.log.Info("notch panel", "ready", r.notchUI)
		r.island = "-" // draw it again
		r.tick()
	case evMenu:
		if u, ok := strings.CutPrefix(ev.id, "url:"); ok {
			id, ok := bar.URLAction(u)
			if !ok {
				r.log.Warn("unknown link", "url", u)
				return
			}
			ev.id = id
		}
		if strings.HasPrefix(ev.id, bar.IDNotch) {
			r.notch(ev.id)
			return
		}
		if ev.id == bar.IDHotkey {
			if !r.snap.OK {
				r.log.Warn("start prompt: tock is not ready")
				return
			}
			r.ui.Prompt(promptSpec(r.snap.Prompt(r.now().UnixMilli())))
			return
		}
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

// featureOn reports whether the Settings toggle id is on in s.
func featureOn(s bar.Snapshot, id string) bool {
	for _, f := range s.Features {
		if f.ID == id {
			return f.On
		}
	}
	return false
}

// promptSpec is p as the Cocoa side reads it.
func promptSpec(p bar.Prompt) string {
	if p.Stop {
		return "stop\t" + clean(p.Title) + "\t" + clean(p.Detail)
	}
	m := make([]string, len(p.Presets))
	for i, n := range p.Presets {
		m[i] = strconv.Itoa(n)
	}
	return "start\t" + clean(p.Title) + "\t" + clean(p.Detail) + "\t" + strings.Join(m, ",")
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
