package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
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
}

func (t *focusTab) set(f report.Focus) { t.data, t.loaded = f, true }

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
	out := []string{" " + sHead.Render("Start a focus session"), ""}
	cell := min(w-4, 30)
	for i := 0; i <= len(d.presets); i++ {
		label := "custom: " + d.custom + "▏ min"
		if i < len(d.presets) {
			label = fmt.Sprintf("%d min", d.presets[i])
		}
		if i == d.cursor {
			out = append(out, "  "+sSel.Render(fit(sBrand.Bold(true).Render("▸ ")+label, cell)))
		} else {
			out = append(out, "  "+sText.Render(fit("  "+label, cell)))
		}
	}
	return out
}

func (t focusTab) view(w int, now time.Time) []string {
	if !t.loaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	f, nowMS, loc := t.data, now.UnixMilli(), now.Location()
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
			center(sText.Render(report.SessionStats(*s, nowMS)), inner), "")
		body = card("Session", body, cw)
	} else {
		body = card("Session", []string{"",
			sText.Render("No session running."),
			sDim.Render("A session silences nothing; it nudges you when you drift."),
			"",
			keycap("s", "choose a length") + "    " + keycap(":", "then \"50 min\""),
			""}, cw)
	}
	out := []string{""}
	for _, l := range body {
		out = append(out, pad+l)
	}
	out = append(out, "", pad+" "+sHead.Render("Today"))
	out = append(out, prefix(sessionLines(f.Today, loc, false, cw), pad)...)
	out = append(out, "", pad+" "+sHead.Render("Earlier this week"))
	return append(out, prefix(sessionLines(f.Week, loc, true, cw), pad)...)
}

func sessionLines(ss []focus.Session, loc *time.Location, withDay bool, w int) []string {
	if len(ss) == 0 {
		return []string{"   " + sDim.Render("none")}
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = "   " + sText.Render(fit(report.SessionLine(s, loc, withDay), w-3))
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
