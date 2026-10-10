package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"tock/internal/classify"
	"tock/internal/report"
)

// inboxHeader is the rows above the list: title and a blank line.
const inboxHeader = 2

// inboxTab lists unlabelled keys.
type inboxTab struct {
	items  []classify.Item
	loaded bool
	cursor int
	top    int
	height int // list rows that fit; set by Model.Update
}

func (t *inboxTab) count() int { return len(t.items) }

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
	}
	return m, nil
}

func (t inboxTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	var total int64
	for _, it := range t.items {
		total += it.TotalMS
	}
	out := []string{
		" " + sHead.Render("Inbox") + sDim.Render(fmt.Sprintf(" · last %d days · %d keys · %s unlabelled", InboxDays, len(t.items), report.Dur(total))),
		"",
	}
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
		line := fitRight(report.Dur(it.TotalMS), 5) + fitRight(plural(it.Spans, "span"), 10) + "  " +
			fit(report.Safe(it.Key), keyW) + "  " + sDim.Render(fit(ex, max(w-keyW-21, 0)))
		if i == t.cursor {
			out = append(out, sSel.Render("▶"+line))
		} else {
			out = append(out, " "+line)
		}
	}
	return out
}
