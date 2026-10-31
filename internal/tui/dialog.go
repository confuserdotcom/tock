package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// askDlg asks for one line of text.
type askDlg struct {
	title, prompt, text string
	max                 int // longest text in runes; 0 means askMax
	done                func(m Model, text string) (tea.Model, tea.Cmd)
}

// askMax is the default longest text of an askDlg, in runes.
const askMax = 80

// choice is one option of a chooseDlg.
type choice struct {
	label, value, note string
}

// chooseDlg asks to pick one option.
type chooseDlg struct {
	title   string
	opts    []choice
	cursor  int
	hotkeys bool // a key equal to an option's value picks it at once
	done    func(m Model, c choice) (tea.Model, tea.Cmd)
}

func (m Model) askKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := m.ask
	switch k := msg.String(); k {
	case "esc":
		m.ask = nil
	case "backspace":
		if _, n := utf8.DecodeLastRuneInString(d.text); n > 0 {
			d.text = d.text[:len(d.text)-n]
		}
	case "ctrl+u":
		d.text = ""
	case "enter":
		m.ask = nil
		return d.done(m, strings.TrimSpace(d.text))
	default:
		if msg.Text == "" {
			break
		}
		// One line: pasted line breaks become spaces, and only as many
		// runes as fit under the cap are kept.
		in := []rune(strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(msg.Text))
		limit := d.max
		if limit <= 0 {
			limit = askMax
		}
		if room := limit - utf8.RuneCountInString(d.text); room > 0 {
			d.text += string(in[:min(room, len(in))])
		}
	}
	return m, nil
}

func (d *askDlg) view(w int) []string {
	cw := min(max(w-8, 30), 60)
	body := []string{}
	if d.prompt != "" {
		body = append(body, sDim.Render(d.prompt))
	}
	body = append(body, sBrand.Render("› ")+sBright.Render(d.text)+sBrand.Render("▏"), "",
		keycap("enter", "ok")+"   "+keycap("esc", "cancel"))
	return center2(card(d.title, body, cw), w)
}

func (m Model) chooseKey(k string) (tea.Model, tea.Cmd) {
	d := m.choose
	switch k {
	case "esc", "q":
		m.choose = nil
	case "up", "k":
		d.cursor = max(d.cursor-1, 0)
	case "down", "j":
		d.cursor = min(d.cursor+1, len(d.opts)-1)
	case "enter":
		return m.pickChoice(d.cursor)
	default:
		if d.hotkeys {
			for i, o := range d.opts {
				if o.value == k {
					return m.pickChoice(i)
				}
			}
		}
	}
	return m, nil
}

// pickChoice closes the dialog with option i.
func (m Model) pickChoice(i int) (tea.Model, tea.Cmd) {
	d := m.choose
	if d == nil || i < 0 || i >= len(d.opts) {
		return m, nil
	}
	m.choose = nil
	return d.done(m, d.opts[i])
}

func (d *chooseDlg) view(w int) []string {
	lw, nw := 1, 0
	for _, o := range d.opts {
		lw = max(lw, utf8.RuneCountInString(o.label))
		nw = max(nw, utf8.RuneCountInString(o.note))
	}
	inner := min(lw+nw+6, max(w-12, 20))
	var body []string
	for i, o := range d.opts {
		mark := "  "
		if i == d.cursor {
			mark = sBrand.Bold(true).Render("▸ ")
		}
		row := mark + sText.Render(fit(o.label, lw)) + "  " + sDim.Render(o.note)
		if i == d.cursor {
			row = selRow(row, inner)
		}
		body = append(body, z("opt:"+strconv.Itoa(i), fit(row, inner)))
	}
	body = append(body, "", keycap("enter", "pick")+"   "+keycap("esc", "cancel"))
	return center2(card(d.title, body, inner+4), w)
}
