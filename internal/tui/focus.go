package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/report"
)

// clockEvery is how often the countdown redraws while a session runs. The
// header shows it on every tab, so the redraw does not depend on the tab.
const clockEvery = time.Second

// focusTab shows the running session and this week's sessions.
type focusTab struct {
	data   report.Focus
	loaded bool
	cursor int                   // the selected row of rows()
	detail *report.SessionDetail // the card of the selected row, nil until loaded
}

// rows are the finished sessions the cursor moves over: today's, then the
// rest of the week's, newest first. The running session is never a row, so
// the card and its keys never act on it.
func (t focusTab) rows() []focus.Session {
	return append(slices.Clone(t.data.Today), t.data.Week...)
}

// shown is the session the card shows, nil when nothing has finished.
func (t focusTab) shown() *focus.Session {
	rows := t.rows()
	if t.cursor < 0 || t.cursor >= len(rows) {
		return nil
	}
	return &rows[t.cursor]
}

func sessionIDs(ss []focus.Session) []int64 {
	ids := make([]int64, len(ss))
	for i, s := range ss {
		ids[i] = s.ID
	}
	return ids
}

// set takes a reload. A list of other sessions puts the cursor back on the
// newest and drops the card; the same list keeps both, the card refreshing
// when its detail arrives.
func (t *focusTab) set(f report.Focus) {
	before := sessionIDs(t.rows())
	t.data, t.loaded = f, true
	rows := t.rows()
	if !slices.Equal(before, sessionIDs(rows)) {
		t.cursor, t.detail = 0, nil
	}
	t.cursor = max(min(t.cursor, len(rows)-1), 0)
}

// detailMsg carries the card of session id.
type detailMsg struct {
	id  int64
	d   report.SessionDetail
	err error
}

// loadDetail reads the card of the selected row. It is asked per selection,
// never for the whole week (the card needs the labelled window of one
// session).
func (m Model) loadDetail() tea.Cmd {
	s := m.focus.shown()
	if s == nil {
		return nil
	}
	id, src := s.ID, m.src
	return func() tea.Msg {
		msg := detailMsg{id: id}
		src.do(func(a *app.App) { msg.d, msg.err = a.SessionDetail(id) })
		return msg
	}
}

// gotDetail keeps the card when it still belongs to the selected row.
func (m Model) gotDetail(msg detailMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setFlash("could not load the session: "+msg.err.Error(), true)
		return m, nil
	}
	if s := m.focus.shown(); s != nil && s.ID == msg.id {
		d := msg.d
		m.focus.detail = &d
	}
	return m, nil
}

// clockMsg redraws the countdown.
type clockMsg struct{}

// maybeClock starts the one-second redraw when the focus tab shows a
// running session and it is not already ticking.
func (m *Model) maybeClock() tea.Cmd {
	if m.clockOn || m.clockTick == nil || m.focus.data.Open == nil {
		return nil
	}
	m.clockOn = true
	return m.clockTick()
}

func (m Model) focusKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "j", "down", "k", "up":
		n := len(m.focus.rows())
		to := m.focus.cursor + 1
		if k == "k" || k == "up" {
			to = m.focus.cursor - 1
		}
		if to < 0 || to >= n {
			return m, nil
		}
		m.focus.cursor, m.focus.detail = to, nil
		return m, m.loadDetail()
	case "R", "n", "f":
		s := m.focus.shown()
		if s == nil {
			return m.noFinished()
		}
		switch k {
		case "R":
			return m.openRate(*s), nil
		case "n":
			return m.openNote(*s), nil
		}
		return m.openRefile(*s), nil
	case "s":
		if m.focus.data.Open != nil {
			m.setFlash("a session is already running · x stops it", true)
			return m, nil
		}
		var presets []int
		m.src.do(func(a *app.App) { presets = append([]int{}, a.Config().Focus.Presets...) })
		m.start = &startDlg{presets: presets}
	case "x":
		if m.focus.data.Open == nil {
			m.setFlash("no session running · s starts one", true)
			return m, nil
		}
		return m, m.stopSession()
	}
	return m, nil
}

// noFinished is the answer to a session key when nothing has finished.
func (m Model) noFinished() (tea.Model, tea.Cmd) {
	m.setFlash("no finished session yet · s starts one", true)
	return m, nil
}

// sessionClock is the "HH:MM" a session started at, in the card's zone.
func (m Model) sessionClock(s focus.Session) string {
	return time.UnixMilli(s.StartMS).In(m.now().Location()).Format("15:04")
}

// ratingWords are the advisory words of the rate dialog, ★5 first. They are
// not stored.
var ratingWords = [5]string{"deep, nothing pulled", "good", "ok", "scattered", "lost it"}

// openRate opens the rate dialog for s. Its option values are the digits, so
// 1 to 5 pick at once (dialog keys come before the tab keys).
func (m Model) openRate(s focus.Session) Model {
	opts := make([]choice, 5)
	for i := range opts {
		n := 5 - i
		opts[i] = choice{label: report.Stars(n) + "  " + strconv.Itoa(n), value: strconv.Itoa(n), note: ratingWords[i]}
	}
	cursor := 0
	if s.Rating >= 1 && s.Rating <= 5 {
		cursor = 5 - s.Rating
	}
	m.choose = &chooseDlg{title: "Rate this session", opts: opts, cursor: cursor, hotkeys: true,
		done: func(m Model, c choice) (tea.Model, tea.Cmd) {
			n, _ := strconv.Atoi(c.value)
			return m, m.rate(s, n)
		}}
	return m
}

// rate gives session s the rating n (the last rating wins).
func (m Model) rate(s focus.Session, n int) tea.Cmd {
	id, clock := s.ID, m.sessionClock(s)
	return m.act(func(a *app.App) (string, error) {
		_, prev, err := a.RateSession(id, n, nil)
		if err != nil {
			return "", fmt.Errorf("could not save the rating: %w", err)
		}
		if prev > 0 {
			return fmt.Sprintf("rating changed to ★%d", n), nil
		}
		return fmt.Sprintf("rated %s session ★%d", clock, n), nil
	})
}

// openNote opens the one-line note dialog for s, prefilled with its note.
func (m Model) openNote(s focus.Session) Model {
	id := s.ID
	m.ask = &askDlg{title: "Note for " + m.sessionClock(s) + " session", prompt: "One line, up to 200 characters.",
		text: s.Note, max: report.MaxNoteRunes,
		done: func(m Model, text string) (tea.Model, tea.Cmd) {
			return m, m.act(func(a *app.App) (string, error) {
				if _, err := a.SetNote(id, text); err != nil {
					return "", fmt.Errorf("could not save the note: %w", err)
				}
				if text == "" {
					return "note cleared", nil
				}
				return "note saved", nil
			})
		}}
	return m
}

// openRefile opens the project picker to refile s, the project holding most
// of its deep time first.
func (m Model) openRefile(s focus.Session) Model {
	loc := m.now().Location()
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	what := fmt.Sprintf("Refile session %s–%s (%s) to", clock(s.StartMS), clock(s.EndMS), report.Dur(s.EndMS-s.StartMS))
	p := newPicker(m.src, pickRefile, nil, "", what)
	p.refileID = s.ID
	var top string
	if d := m.focus.detail; d != nil && d.Sess.ID == s.ID {
		top = d.Top.Project
	} else {
		m.src.do(func(a *app.App) {
			if d, err := a.SessionDetail(s.ID); err == nil {
				top = d.Top.Project
			}
		})
	}
	if top != "" {
		p.quick = append(p.quick, option{name: top, note: "top project", want: &classify.Want{Project: top}})
	}
	m.picker = p
	return m
}

func (m Model) startSession(n int) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		var msg doneMsg
		src.do(func(a *app.App) {
			if _, err := a.StartSession(n); err != nil {
				msg.err = err
				return
			}
			msg.text = fmt.Sprintf("session started: %d min", n)
			if h, err := a.Health(); err == nil && !h.TrackdUp {
				msg.text += " · trackd down, no alerts"
			}
		})
		return msg
	}
}

func (m Model) stopSession() tea.Cmd {
	src, loc := m.src, m.now().Location()
	return func() tea.Msg {
		var msg doneMsg
		src.do(func(a *app.App) {
			s, err := a.StopSession()
			if err != nil {
				msg.err = err
				return
			}
			msg.text = "stopped: " + report.SessionLine(s, loc, false)
		})
		return msg
	}
}

// startDlg picks a session length: the presets, then a custom row.
type startDlg struct {
	presets []int
	cursor  int    // len(presets) is the custom row
	custom  string // digits typed on the custom row
}

func (m Model) startKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := m.start
	switch k := msg.String(); k {
	case "esc":
		m.start = nil
	case "up", "k":
		d.cursor = max(d.cursor-1, 0)
	case "down", "j":
		d.cursor = min(d.cursor+1, len(d.presets))
	case "backspace":
		if n := len(d.custom); n > 0 {
			d.custom = d.custom[:n-1]
		}
	case "enter":
		n := 0
		if d.cursor < len(d.presets) {
			n = d.presets[d.cursor]
		} else if v, err := strconv.Atoi(d.custom); err == nil && v >= 1 && v <= config.MaxSessionMin {
			n = v
		} else {
			m.setFlash(fmt.Sprintf("length 1 to %d minutes", config.MaxSessionMin), true)
			return m, nil
		}
		m.start = nil
		return m, m.startSession(n)
	default:
		if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && len(d.custom) < 3 {
			d.cursor = len(d.presets)
			d.custom += k
		}
	}
	return m, nil
}

// startHead is the dialog's lines above the length rows.
const startHead = 2

// rowAt is the length row (len(presets) is custom) on body line line, if any.
func (d *startDlg) rowAt(line int) (int, bool) {
	i := line - startHead
	if i < 0 || i > len(d.presets) {
		return 0, false
	}
	return i, true
}

func (d *startDlg) view(w int) []string {
	var out []string
	cell := min(w-8, 30)
	for i := 0; i <= len(d.presets); i++ {
		label := "custom: " + d.custom + "▏ min"
		if i < len(d.presets) {
			label = fmt.Sprintf("%d min", d.presets[i])
		}
		if i == d.cursor {
			out = append(out, selRow(sBrand.Bold(true).Render("▸ ")+sBright.Render(label), cell))
		} else {
			out = append(out, sText.Render(fit("  "+label, cell)))
		}
	}
	return center2(card("Start a focus session", out, cell+4), w)
}

func (t focusTab) view(w, h int, now time.Time) []string {
	if !t.loaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	f, loc := t.data, now.Location()
	nowMS := now.UnixMilli()
	cw := min(w-2, 72)
	pad := strings.Repeat(" ", max((w-cw)/2, 0))
	var body []string
	if s := f.Open; s != nil {
		total, left := s.PlannedMS-s.StartMS, s.PlannedMS-nowMS
		inner := cw - 4
		body = append(body, "")
		for _, r := range bigDigits(report.Countdown(left)) {
			body = append(body, center(sBrand.Bold(true).Render(r), inner))
		}
		body = append(body, center(sDim.Render("left of "+report.Dur(total)), inner), "",
			center(meter(total-left, total, min(inner-4, 48), sBrand), inner),
			center(sText.Render(report.LiveStats(*s, f.Live, nowMS)), inner), "")
		body = card("Session", body, cw)
	} else {
		body = card("Session", []string{"",
			sText.Render("No session running."),
			sDim.Render("A session nudges you when you drift and records how deep it went."),
			"",
			keycap("s", "choose a length") + "    " + keycap(":", "then \"50 min\""),
			""}, cw)
	}
	out := []string{""}
	for _, l := range body {
		out = append(out, pad+l)
	}
	out = append(out, "")
	for _, l := range t.card(cw, h, loc) {
		out = append(out, pad+l)
	}
	n := len(f.Today)
	out = append(out, "", pad+" "+sHead.Render("Today"))
	out = append(out, prefix(t.sessionRows(f.Today, 0, loc, false, cw), pad)...)
	out = append(out, "", pad+" "+sHead.Render("Earlier this week"))
	return append(out, prefix(t.sessionRows(f.Week, n, loc, true, cw), pad)...)
}

// cardLabel is a card row's 12-cell label.
func cardLabel(s string) string { return sDim.Render(fit(s, 12)) }

// qualityLine draws the breakdown "share 82 · longest 70 · ..." with the
// numbers in the text colour and the words dim.
func qualityLine(s string) string {
	var out, run strings.Builder
	digits := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		st := sDim
		if digits {
			st = sText
		}
		out.WriteString(st.Render(run.String()))
		run.Reset()
	}
	for _, r := range s {
		if d := unicode.IsDigit(r); d != digits {
			flush()
			digits = d
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}

// starsLine draws the rating: filled stars bright, the rest and "unrated"
// faint.
func starsLine(rating int) string {
	if rating < 1 {
		return sFaint.Render(report.Stars(0))
	}
	rating = min(rating, 5)
	return sBright.Render(strings.Repeat("★", rating)) + sFaint.Render(strings.Repeat("☆", 5-rating))
}

// card is the last-session card of the selected row (UI-SPEC Surface 1),
// shortened to fit a body of h lines: below 34 the blank lines go, then the
// apps row at 31, then the categories row.
func (t focusTab) card(cw, h int, loc *time.Location) []string {
	title := "Last session"
	s := t.shown()
	if s == nil {
		return card(title, []string{
			sText.Render("No finished session yet."),
			sDim.Render("A finished session gets a quality score and a card here.")}, cw)
	}
	if t.cursor > 0 {
		title = "Session · " + time.UnixMilli(s.StartMS).In(loc).Format("Mon 15:04")
	}
	d := t.detail
	if d == nil || d.Sess.ID != s.ID {
		return card(title, []string{sDim.Render("loading…")}, cw)
	}
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	length := d.Sess.EndMS - d.Sess.StartMS
	share := func(xs []focus.Share) string {
		if txt := report.ShareText(xs); txt != "" {
			return sText.Render(report.Safe(txt))
		}
		return sDim.Render("none")
	}
	blank := h >= 34
	var lines []string
	lines = append(lines,
		spread(sText.Render(clock(d.Sess.StartMS)+"–"+clock(d.Sess.EndMS)+"  "+report.Dur(length)+"  "+d.Sess.Kind),
			sBrand.Bold(true).Render("quality "+strconv.Itoa(d.Q.Score)), cw-4),
		meter(d.Sess.DeepMS, length, 40, sDeep)+"  "+sText.Render(strconv.Itoa(d.DeepPct())+"% deep"),
		qualityLine(d.Q.Line()))
	if blank {
		lines = append(lines, "")
	}
	if h >= 31 {
		lines = append(lines, cardLabel("categories")+share(d.Top.Categories))
	}
	if h >= 32 {
		lines = append(lines, cardLabel("apps")+share(d.Top.Apps))
	}
	lines = append(lines, cardLabel("")+sText.Render(d.RunLine()),
		cardLabel("rating")+starsLine(d.Sess.Rating))
	if d.Sess.Note != "" {
		lines = append(lines, cardLabel("note")+sText.Render(report.Safe(d.Sess.Note)))
	}
	if blank {
		lines = append(lines, "")
	}
	keys := keycap("R", "rate") + "   " + keycap("n", "note")
	if d.Top.Refile() {
		keys += "   " + keycap("f", "refile → "+report.Cut(report.Safe(d.Top.Project), 24))
	}
	keys += "   " + keycap("j/k", "other session")
	lines = append(lines, keys)
	for i := range lines {
		lines[i] = fit(lines[i], cw-4)
	}
	return card(title, lines, cw)
}

// sessionRows draws ss, the rows from index first of the cursor's list, each
// ending in its quality and rating (UI-SPEC "List rows").
func (t focusTab) sessionRows(ss []focus.Session, first int, loc *time.Location, withDay bool, w int) []string {
	if len(ss) == 0 {
		return []string{"   " + sDim.Render("none")}
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		star := sFaint.Render(fitRight("·", 3))
		if s.Rating > 0 {
			star = sBright.Render(fitRight("★"+strconv.Itoa(s.Rating), 3))
		}
		q := sBrand.Render(fitRight("Q"+strconv.Itoa(focus.Quality(s, t.data.Weights).Score), 4))
		row := sText.Render(fit(report.SessionLine(s, loc, withDay), w-3-9)) + " " + q + " " + star
		if first+i == t.cursor {
			out[i] = selRow(sBrand.Bold(true).Render("▸")+"  "+row, w)
			continue
		}
		out[i] = "   " + row
	}
	return out
}

// progress draws done out of total as a bar of cells blocks.
func progress(done, total int64, cells int) string {
	if total <= 0 || cells <= 0 {
		return ""
	}
	n := int(min(max(done, 0), total) * int64(cells) / total)
	return sBright.Render(strings.Repeat("█", n)) + sFaint.Render(strings.Repeat("░", cells-n))
}
