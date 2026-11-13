package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
	"tock/internal/report"
	"tock/internal/service"
)

// headerGap separates the header's names.
const headerGap = "   "

// tabNames are the header's tab names, in tab order. The inbox carries its
// count as a superscript badge.
func (m Model) tabNames() []string {
	names := append([]string{}, tabTitles[:]...)
	if n := m.inbox.count(); n > 0 {
		names[tabInbox] = "inbox " + superscript(n)
	}
	if m.noFocus {
		names[tabFocus] = "" // not shown
	}
	return names
}

// superscript writes n in superscript digits: 12 → ¹².
func superscript(n int) string {
	const sup = "⁰¹²³⁴⁵⁶⁷⁸⁹"
	digits := []rune(sup)
	var b strings.Builder
	for _, c := range fmt.Sprint(n) {
		b.WriteRune(digits[c-'0'])
	}
	return b.String()
}

func (m Model) header() string {
	names := m.tabNames()
	parts := []string{sBrand.Bold(true).Render("tock")}
	for i, n := range names {
		if n == "" {
			continue
		}
		id := "tab:" + fmt.Sprint(i)
		switch {
		case i == m.tab:
			parts = append(parts, z(id, pill(n, sBrand)))
		case i == tabInbox && m.inbox.count() > 0:
			name, badge, _ := strings.Cut(n, " ")
			parts = append(parts, z(id, sDim.Render(name+" ")+sUnl.Render(badge)))
		default:
			parts = append(parts, z(id, sDim.Render(n)))
		}
	}
	left := strings.Join(parts, headerGap)
	var session string
	if s := m.focus.data.Open; s != nil {
		session = z("tab:"+fmt.Sprint(tabFocus), pill("◷ "+report.Countdown(s.PlannedMS-m.now().UnixMilli()), sBrand)) + "  "
	}
	// The widest right side that fits: date, clock and trackd; then the
	// clock and trackd's dot; then the dot.
	for _, right := range []string{
		session + sDim.Render(m.now().Format("Mon 2 Jan  15:04")+"  ") + m.trackdText(),
		session + sDim.Render(m.now().Format("15:04")+" ") + m.trackdDot(),
		session + m.trackdDot(),
	} {
		if ansi.StringWidth(left)+ansi.StringWidth(right)+1 <= m.width {
			return spread(left, right, m.width)
		}
	}
	return fit(left, m.width)
}

// trackdDot is trackd's state as one dot: green up, coral down.
func (m Model) trackdDot() string {
	if m.health.TrackdUp {
		return sGood.Render("●")
	}
	return sAccent.Render("●")
}

func (m Model) trackdText() string {
	if m.health.TrackdUp {
		return sGood.Render("●") + sDim.Render(" trackd")
	}
	return sAccent.Render("● trackd down")
}

// reinstallHint is how to replace trackd with the newer one, for how tock
// was installed. Tests pin it.
var reinstallHint = func() string { return service.Installed().Reinstall(app.HostOS == "linux") }

// banner is the most important problem, or "".
func (m Model) banner() string {
	h := m.health
	switch {
	case m.healthErr != nil:
		return "cannot read trackd state: " + m.healthErr.Error()
	case h.TooOld():
		return app.TooOldText + "; update it: " + reinstallHint()
	case !h.TrackdUp:
		return "trackd is not running — showing history. Restart: launchctl kickstart -k gui/$UID/com.tock.trackd"
	case h.Paused():
		return "not recording: config.toml does not load (" + h.CapturePaused + "); fix it and recording resumes within seconds"
	case h.RulesErr != nil:
		return "rule error, using the last good rules: " + h.RulesErr.Error()
	case h.ConfigErr != nil:
		return "config error, using the last good settings: " + h.ConfigErr.Error()
	case h.AXTrusted != nil && !*h.AXTrusted:
		return "no window titles: allow trackd in System Settings › Privacy & Security › Accessibility"
	case len(h.AutomationOff) > 0:
		return "no URLs from " + strings.Join(h.AutomationOff, ", ") + ": allow trackd in System Settings › Privacy & Security › Automation"
	}
	return ""
}

func (m Model) footer() string {
	if m.flash != "" {
		if m.flashErr {
			return sAccent.Render(fit("✗ "+m.flash, m.width))
		}
		if text, ok := strings.CutSuffix(m.flash, " (u undo)"); ok {
			return fit(sGood.Render("✓ ")+sText.Render(text)+"   "+keycap("u", "undo"), m.width)
		}
		return sGood.Render("✓ ") + sText.Render(fit(m.flash, m.width-2))
	}
	if m.zones != nil && m.menu == nil {
		if t := m.zones.tips[m.zones.hover]; t != "" {
			return sBrand.Render("◆ ") + sText.Render(fit(t, m.width-2))
		}
	}
	if m.baseView() {
		return fit(styledHint(m.footerActions(m.width)), m.width)
	}
	return sDim.Render(fit(m.hint(), m.width))
}

// baseView reports a tab with no dialog, page or filter over it: the
// footer then comes from the action table.
func (m Model) baseView() bool {
	return m.palette == nil && !m.overlay() && !m.filtering()
}

// center2 centres every line of a block in w cells, with one blank line
// above it.
func center2(block []string, w int) []string {
	out := []string{""}
	if len(block) == 0 {
		return out
	}
	pad := max((w-ansiWidth(block[0]))/2, 0)
	for _, l := range block {
		out = append(out, strings.Repeat(" ", pad)+l)
	}
	return out
}

// hint is the footer's key list for the current view.
func (m Model) hint() string {
	var hint string
	switch {
	case m.palette != nil:
		hint = "type to search · ↑/↓ move · enter run · esc close"
	case m.help:
		hint = "esc close · : commands"
	case m.score:
		hint = "esc close · ? keys"
	case m.picker != nil:
		hint = "type to filter · ↑/↓ move · enter pick · esc cancel"
	case m.learn != nil:
		hint = m.learn.hint()
	case m.start != nil:
		hint = "↑/↓ move · digits custom length · enter start · esc cancel"
	case m.edit != nil:
		hint = "type HH:MM-HH:MM · enter pick a label · esc cancel"
	case m.ask != nil:
		hint = "type · enter ok · esc cancel"
	case m.choose != nil:
		hint = "↑/↓ move · enter pick · esc cancel"
	case m.filtering():
		hint = m.today.hint()
	case m.tab == tabPlan && m.plan.adding:
		hint = "type the task · enter add · esc cancel"
	default:
		hint = hintOf(m.footerActions(m.width))
	}
	return hint
}
