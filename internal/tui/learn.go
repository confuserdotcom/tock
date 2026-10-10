package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// proposedMsg carries the rule candidates for a key.
type proposedMsg struct {
	key   string
	want  classify.Want
	cands []classify.Candidate // safe ones only
	ids   []int64
	ms    int64
	err   error
}

// learnDlg offers learned rules for a key, most general first.
type learnDlg struct {
	key   string
	want  classify.Want
	cands []classify.Candidate
	i     int
	ids   []int64
	ms    int64
}

func (m Model) propose(key string, want classify.Want) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		msg := proposedMsg{key: key, want: want}
		src.do(func(a *app.App) {
			var all []classify.Candidate
			if all, msg.err = a.Propose(key, want); msg.err != nil {
				return
			}
			for _, c := range all {
				if c.Safe {
					msg.cands = append(msg.cands, c)
				}
			}
			msg.ids, msg.ms, msg.err = a.KeySpans(key)
		})
		return msg
	}
}

func (m Model) openLearn(msg proposedMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.setFlash(msg.err.Error(), true)
	case len(msg.ids) == 0:
		m.setFlash("no spans with this key any more", true)
	default:
		m.learn = &learnDlg{key: msg.key, want: msg.want, cands: msg.cands, ids: msg.ids, ms: msg.ms}
	}
	return m, nil
}

func (m Model) learnKey(k string) (tea.Model, tea.Cmd) {
	d := m.learn
	label := report.LabelText(d.want.Category, d.want.Project)
	switch {
	case k == "y" && len(d.cands) > 0:
		c, key, want := d.cands[d.i], d.key, d.want
		m.learn = nil
		return m, m.act(func(a *app.App) (string, error) {
			return fmt.Sprintf("learned: %s now covers %d spans (%s)", label, c.Matches, report.Dur(c.MatchMS)),
				a.Learn(key, c, want)
		})
	case k == "s" && d.i+1 < len(d.cands):
		d.i++
	case k == "o" && len(d.cands) == 0:
		ids, what, want := d.ids, spansWhat(len(d.ids), d.ms), d.want
		m.learn = nil
		return m, m.act(func(a *app.App) (string, error) {
			return "tagged " + what + " → " + label, a.Tag(ids, want)
		})
	case k == "n" || k == "esc":
		m.learn = nil
		m.setFlash("cancelled", false)
	}
	return m, nil
}

func (d *learnDlg) hint() string {
	switch {
	case len(d.cands) == 0:
		return "o tag these spans only · n cancel"
	case d.i+1 < len(d.cands):
		return "y accept · s more specific · n cancel"
	}
	return "y accept · n cancel"
}

func (d *learnDlg) view(w int) []string {
	label := report.LabelText(d.want.Category, d.want.Project)
	lines := []string{sHead.Render("Learn a rule") + sDim.Render("  "+report.Safe(d.key)), ""}
	if len(d.cands) == 0 {
		lines = append(lines,
			"no safe rule: every candidate would relabel spans other rules already label.",
			"",
			fmt.Sprintf("[o] tag these %s only   [n] cancel", spansWhat(len(d.ids), d.ms)))
	} else {
		c := d.cands[d.i]
		lines = append(lines,
			sDim.Render("rule     ")+report.Safe(c.Rule.Describe())+"  ->  "+label,
			sDim.Render("matches  ")+fmt.Sprintf("%d spans (%s): %d this key, %d other unlabelled (%s), %d relabelled",
				c.Matches, report.Dur(c.MatchMS), c.ThisKey, c.New, report.Dur(c.NewMS), c.Relabeled),
			"")
		opts := "[y] accept   [n] cancel"
		if d.i+1 < len(d.cands) {
			opts = "[y] accept   [s] more specific   [n] cancel"
		}
		lines = append(lines, opts)
	}
	for i := range lines {
		lines[i] = fit(lines[i], max(w-6, 10))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colFaint).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	return append([]string{""}, strings.Split(box, "\n")...)
}
