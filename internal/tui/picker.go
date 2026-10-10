package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// pickAction is what happens once a label is picked.
type pickAction int

const (
	pickTag     pickAction = iota // tag ids
	pickProject                   // set only the project of ids
	pickOnce                      // tag every span of key, no rule
	pickLearn                     // propose a rule for key
	pickAway                      // label the absence away
	pickEdit                      // a timeline edit over editFrom–editTo
)

const noProject = "(no project)"

// option is one picker row. A quick option carries a whole label (want) and
// finishes the pick at once.
type option struct {
	name, note string
	want       *classify.Want
}

// picker chooses a category, then a project, by prefix filter.
type picker struct {
	action           pickAction
	ids              []int64
	key              string
	what             string
	cats             []option
	projects         []option
	quick            []option // whole labels listed first: suggested, then recent
	stage            int      // 0 category, 1 project
	query            string
	cursor           int
	want             classify.Want
	away             classify.Interval // pickAway
	editKind         string            // pickEdit: classify.EditAdd or EditLabel
	editFrom, editTo int64             // pickEdit
}

// newAwayPicker labels the absence iv.
func newAwayPicker(src *source, iv classify.Interval, what string) *picker {
	p := newPicker(src, pickAway, nil, "", "away "+what)
	p.away = iv
	return p
}

func newPicker(src *source, action pickAction, ids []int64, key, what string) *picker {
	p := &picker{action: action, ids: ids, key: key, what: what}
	src.do(func(a *app.App) {
		rs := a.Rules()
		for _, c := range rs.Categories {
			p.cats = append(p.cats, option{name: c.Name, note: c.Kind})
		}
		if action != pickProject {
			p.projects = append(p.projects, option{name: noProject})
		}
		for _, pr := range rs.Projects {
			if !pr.Archived {
				p.projects = append(p.projects, option{name: pr.Name})
			}
		}
	})
	if action == pickProject {
		p.stage = 1
	}
	return p
}

// withQuick adds the quick options: the suggestion for key, and for time
// away the recent away labels.
func (p *picker) withQuick(m *Model, key string) *picker {
	if w, ok := m.inbox.sugg[key]; ok && key != "" {
		w := w
		p.quick = append(p.quick, option{name: report.LabelText(w.Category, w.Project), note: "★ suggested", want: &w})
	}
	if p.action == pickAway {
		var recent []classify.Want
		m.src.do(func(a *app.App) { recent, _ = a.RecentAwayLabels(3) })
		for _, w := range recent {
			w := w
			p.quick = append(p.quick, option{name: report.LabelText(w.Category, w.Project), note: "↺ recent", want: &w})
		}
	}
	return p
}

// options is the current stage's list after the prefix filter; quick
// options lead the category stage while nothing is typed.
func (p *picker) options() []option {
	list := p.cats
	if p.stage == 1 {
		list = p.projects
	}
	q := strings.ToLower(p.query)
	var out []option
	if p.stage == 0 && q == "" {
		out = append(out, p.quick...)
	}
	for _, o := range list {
		if q == "" || (o.name != noProject && strings.HasPrefix(strings.ToLower(o.name), q)) {
			out = append(out, o)
		}
	}
	return out
}

func (m Model) pickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.picker
	opts := p.options()
	switch msg.String() {
	case "esc":
		m.picker = nil
		return m, nil
	case "up", "ctrl+p":
		p.cursor = max(p.cursor-1, 0)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = min(p.cursor+1, max(len(opts)-1, 0))
		return m, nil
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query, p.cursor = string(r[:len(r)-1]), 0
		}
		return m, nil
	case "enter":
		if len(opts) == 0 {
			return m, nil
		}
		opt := opts[min(p.cursor, len(opts)-1)]
		if opt.want != nil {
			p.want = *opt.want
			m.picker = nil
			return m, m.finishPick(p)
		}
		choice := opt.name
		if p.stage == 0 {
			p.want.Category = choice
			p.stage, p.query, p.cursor = 1, "", 0
			return m, nil
		}
		if choice != noProject {
			p.want.Project = choice
		}
		m.picker = nil
		return m, m.finishPick(p)
	}
	if msg.Text != "" {
		p.query += msg.Text
		p.cursor = 0
	}
	return m, nil
}

// finishPick runs the picker's action with the chosen label.
func (m Model) finishPick(p *picker) tea.Cmd {
	want, label := p.want, report.LabelText(p.want.Category, p.want.Project)
	switch p.action {
	case pickEdit:
		return m.editDone(p)
	case pickLearn:
		return m.propose(p.key, want)
	case pickAway:
		iv, what := p.away, p.what
		return m.act(func(a *app.App) (string, error) {
			return "labelled " + what + " → " + label, a.Answer(iv, want)
		})
	case pickOnce:
		key := p.key
		return m.act(func(a *app.App) (string, error) {
			ids, ms, err := a.KeySpans(key)
			if err != nil {
				return "", err
			}
			return "tagged " + spansWhat(len(ids), ms) + " → " + label, a.Tag(ids, want)
		})
	}
	ids, what := p.ids, p.what
	return m.act(func(a *app.App) (string, error) {
		return "tagged " + what + " → " + label, a.Tag(ids, want)
	})
}

// window is the range of options shown in a body of h lines.
func (p *picker) window(h, n int) (first, end int) {
	room := max(h-8, 3)
	first = max(min(p.cursor-room+1, n-room), 0)
	return first, min(first+room, n)
}

// pickerHead is the box's lines above the options: border, title, query,
// blank.
const pickerHead = 4

// rowAt is the option shown on body line line, if any.
func (p *picker) rowAt(line, w, h int) (int, bool) {
	body := p.view(w, h)
	top := -1
	for i, l := range body {
		if strings.Contains(l, "╭") {
			top = i
			break
		}
	}
	first, end := p.window(h, len(p.options()))
	i := first + line - top - pickerHead
	if top < 0 || line < top+pickerHead || i >= end {
		return 0, false
	}
	return i, true
}

func (p *picker) view(w, h int) []string {
	title := "Category"
	if p.stage == 1 {
		title = "Project"
		if p.want.Category != "" {
			title = "Project for " + p.want.Category
		}
	}
	subject := p.what
	if subject == "" {
		subject = report.Safe(p.key)
	}
	const cw = 44
	lines := []string{sDim.Render(subject), sBrand.Render("› ") + sBright.Render(p.query) + sBrand.Render("▏"), ""}
	opts := p.options()
	first, end := p.window(h, len(opts))
	for i := first; i < end; i++ {
		o := opts[i]
		name := sText.Render(fit(o.name, 24))
		note := sDim.Render(fit(o.note, 12))
		if o.want != nil {
			name = sUnl.Render(fit(o.name, 24))
			note = sBrand.Render(fit(o.note, 12))
		} else if p.stage == 0 {
			name = dot(o.note) + " " + kindStyle(o.note).Render(fit(o.name, 22))
		}
		line := name + " " + note
		if i == p.cursor {
			line = selRow(sBrand.Bold(true).Render("▸ ")+line, cw-4)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(opts) == 0 {
		lines = append(lines, sDim.Render("  no match · backspace"))
	}
	box := strings.Join(card(title, lines, cw), "\n")
	return strings.Split(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box), "\n")
}
