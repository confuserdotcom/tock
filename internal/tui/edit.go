package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// rangeDlg asks for a time range on the shown day, then opens the label
// picker for an edit over it: an added entry, or a label over part of the
// timeline (a split).
type rangeDlg struct {
	kind  string // classify.EditAdd or classify.EditLabel
	title string
	text  string // "HH:MM-HH:MM"
	day   time.Time
}

func clockText(ms int64, loc *time.Location) string {
	return time.UnixMilli(ms).In(loc).Format("15:04")
}

// newRangeDlg starts with [from, to) filled in.
func newRangeDlg(kind, title string, from, to int64, day time.Time) *rangeDlg {
	loc := day.Location()
	return &rangeDlg{kind: kind, title: title, text: clockText(from, loc) + "-" + clockText(to, loc), day: day}
}

// parse reads the text as a range on the dialog's day.
func (d *rangeDlg) parse(now time.Time) (int64, int64, error) {
	a, b, ok := strings.Cut(d.text, "-")
	if !ok {
		return 0, 0, errors.New("want HH:MM-HH:MM")
	}
	clock := func(s string) (int64, error) {
		t, err := time.Parse("15:04", strings.TrimSpace(s))
		if err != nil {
			return 0, fmt.Errorf("%q is not HH:MM", s)
		}
		y, m, dd := d.day.Date()
		return time.Date(y, m, dd, t.Hour(), t.Minute(), 0, 0, d.day.Location()).UnixMilli(), nil
	}
	from, err := clock(a)
	if err != nil {
		return 0, 0, err
	}
	to, err := clock(b)
	if err != nil {
		return 0, 0, err
	}
	if to <= from {
		return 0, 0, errors.New("the range ends before it starts")
	}
	if to > now.UnixMilli() {
		return 0, 0, errors.New("the range reaches into the future")
	}
	return from, to, nil
}

func (m Model) rangeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := m.edit
	switch k := msg.String(); k {
	case "esc":
		m.edit = nil
	case "backspace":
		if n := len(d.text); n > 0 {
			d.text = d.text[:n-1]
		}
	case "ctrl+u":
		d.text = ""
	case "enter":
		from, to, err := d.parse(m.now())
		if err != nil {
			m.setFlash(err.Error(), true)
			return m, nil
		}
		m.edit = nil
		what := clockText(from, d.day.Location()) + "–" + clockText(to, d.day.Location()) + " (" + report.Dur(to-from) + ")"
		p := newPicker(m.src, pickEdit, nil, "", what)
		p.editKind, p.editFrom, p.editTo = d.kind, from, to
		m.picker = p
	default:
		if len(k) == 1 && strings.ContainsAny(k, "0123456789:-") && len(d.text) < 11 {
			d.text += k
		}
	}
	return m, nil
}

func (d *rangeDlg) view(w int) []string {
	cell := min(w-8, 34)
	return center2(card(d.title, []string{
		sDim.Render("from-to, on " + d.day.Format("Mon 2 Jan")),
		sBrand.Render("› ") + sBright.Render(d.text) + sBrand.Render("▏"),
	}, cell+4), w)
}

// editDone runs the picked edit.
func (m Model) editDone(p *picker) tea.Cmd {
	kind, from, to, want, what := p.editKind, p.editFrom, p.editTo, p.want, p.what
	label := report.LabelText(want.Category, want.Project)
	verb := "labelled "
	if kind == classify.EditAdd {
		verb = "added "
	}
	return m.act(func(a *app.App) (string, error) {
		_, err := a.AddEdit(kind, from, to, want, "")
		return verb + what + " → " + label + " (u undo)", err
	})
}

// deleteRow cuts the selected row's time out of the timeline.
func (m Model) deleteRow(r *report.Row) tea.Cmd {
	from, to := r.Start, r.End
	what := clockText(from, m.now().Location()) + "–" + clockText(to, m.now().Location()) + " (" + report.Dur(to-from) + ")"
	return m.act(func(a *app.App) (string, error) {
		_, err := a.AddEdit(classify.EditCut, from, to, classify.Want{}, "")
		return "deleted " + what + " (u undo)", err
	})
}

// undo reverses the newest change.
func (m Model) undo() tea.Cmd {
	return m.act(func(a *app.App) (string, error) {
		what, err := a.Undo()
		if errors.Is(err, app.ErrNothingToUndo) {
			return "nothing to undo", nil
		}
		return "undid: " + report.Safe(what), err
	})
}
