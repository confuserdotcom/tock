package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// inboxHeader is the rows above the list: title and a blank line.
const inboxHeader = 2

// inboxTab lists unlabelled keys.
type inboxTab struct {
	items   []classify.Item
	loaded  bool
	cursor  int
	top     int
	height  int                      // list rows that fit; set by Model.Update
	ignored []string                 // keys ignored this session, for u
	sugg    map[string]classify.Want // valid suggestions by key
}

func (t *inboxTab) count() int { return len(t.items) }

// suggested counts the listed keys that have a suggestion.
func (t *inboxTab) suggested() int {
	n := 0
	for _, it := range t.items {
		if _, ok := t.sugg[it.Key]; ok {
			n++
		}
	}
	return n
}

func (t *inboxTab) set(items []classify.Item) {
	var key string
	if t.cursor < len(t.items) {
		key = t.items[t.cursor].Key
	}
	t.items, t.loaded = items, true
	for i, it := range items {
		if it.Key == key {
			t.cursor = i
			return
		}
	}
	t.cursor = min(t.cursor, max(len(items)-1, 0))
}

func (t *inboxTab) clampScroll() {
	h := max(t.height, 1)
	t.cursor = min(max(t.cursor, 0), max(len(t.items)-1, 0))
	if t.cursor < t.top {
		t.top = t.cursor
	}
	if t.cursor >= t.top+h {
		t.top = t.cursor - h + 1
	}
	t.top = max(min(t.top, len(t.items)-h), 0)
}

func (t *inboxTab) selected() *classify.Item {
	if t.cursor < len(t.items) {
		return &t.items[t.cursor]
	}
	return nil
}

func (m Model) inboxKey(k string) (tea.Model, tea.Cmd) {
	t := &m.inbox
	switch k {
	case "j", "down":
		t.cursor++
	case "k", "up":
		t.cursor--
	case "g", "home":
		t.cursor = 0
	case "G", "end":
		t.cursor = len(t.items) - 1
	case "enter", "T":
		if it := t.selected(); it != nil {
			m.picker = newPicker(m.src, pickLearn, nil, it.Key, "")
		}
	case "a":
		if it := t.selected(); it != nil {
			w, ok := t.sugg[it.Key]
			if !ok {
				m.setFlash("no suggestion · enter to label", true)
				return m, nil
			}
			return m, m.propose(it.Key, w)
		}
	case "o":
		if it := t.selected(); it != nil {
			m.picker = newPicker(m.src, pickOnce, nil, it.Key, spansWhat(it.Spans, it.TotalMS))
		}
	case "x":
		if it := t.selected(); it != nil {
			key := it.Key
			t.ignored = append(t.ignored, key)
			return m, m.act(func(a *app.App) (string, error) {
				return "ignored: " + report.Safe(key) + " (u undo)", a.Ignore(key, true)
			})
		}
	case "u":
		if n := len(t.ignored); n > 0 {
			key := t.ignored[n-1]
			t.ignored = t.ignored[:n-1]
			return m, m.act(func(a *app.App) (string, error) {
				return "showing again: " + report.Safe(key), a.Ignore(key, false)
			})
		}
	}
	return m, nil
}

// act runs fn on the app and reports its result as a doneMsg.
func (m Model) act(fn func(a *app.App) (string, error)) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		var d doneMsg
		src.do(func(a *app.App) { d.text, d.err = fn(a) })
		if d.err != nil {
			d.text = ""
		}
		return d
	}
}

// rowAt is the item shown on body line line, if any.
func (t *inboxTab) rowAt(line int) (int, bool) {
	i := t.top + line - inboxHeader
	if !t.loaded || line < inboxHeader || line-inboxHeader >= max(t.height, 1) || i >= len(t.items) {
		return 0, false
	}
	return i, true
}

func (t inboxTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	var total int64
	for _, it := range t.items {
		total += it.TotalMS
	}
	head := fmt.Sprintf(" · last %d days · %d keys · %s unlabelled", InboxDays, len(t.items), report.Dur(total))
	if n := t.suggested(); n > 0 {
		head += fmt.Sprintf(" · %d suggested", n)
	}
	out := []string{" " + sHead.Render("Inbox") + sDim.Render(head), ""}
	if len(t.items) == 0 {
		return append(out, " "+sDim.Render("Inbox empty: everything is labelled."))
	}
	end := min(t.top+max(h-inboxHeader, 1), len(t.items))
	keyW := max(min(w/2-20, 40), 16)
	for i := t.top; i < end; i++ {
		it := t.items[i]
		ex := ""
		if it.Example != "" {
			ex = fmt.Sprintf("%q", report.Safe(it.Example))
		}
		rest := max(w-keyW-21, 0)
		tail := fit(ex, rest)
		if wnt, ok := t.sugg[it.Key]; ok {
			sug := "→ " + report.Safe(report.LabelText(wnt.Category, wnt.Project))
			if sw := ansi.StringWidth(sug); sw+2 > rest {
				tail = fit(sug, rest)
			} else {
				tail = fit(ex, rest-sw-2) + "  " + sug
			}
		}
		line := fitRight(report.Dur(it.TotalMS), 5) + fitRight(plural(it.Spans, "span"), 10) + "  " +
			fit(report.Safe(it.Key), keyW) + "  " + sDim.Render(tail)
		if i == t.cursor {
			out = append(out, sSel.Render(sBrand.Bold(true).Render("▸")+line))
		} else {
			out = append(out, " "+line)
		}
	}
	return out
}
