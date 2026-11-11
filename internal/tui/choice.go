package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/config"
)

// ChoiceDeps is what tock choice does outside the list.
type ChoiceDeps struct {
	// Save writes the changes, feature id to its new value, to config.toml.
	Save func(changes map[string]bool) error
	// Open starts the web dashboard and opens it in the browser; it
	// returns the link.
	Open func() (string, error)
	// Remember keeps "always open the dashboard without asking".
	Remember func() error
}

// Choice is tock choice: the features of config.Features, turned on and
// off with space and written with enter, and a last row that opens the web
// dashboard after asking.
type Choice struct {
	deps   ChoiceDeps
	saved  []bool // the value in config.toml
	on     []bool // the value shown; differs from saved until enter
	cur    int    // len(config.Features) is the dashboard row
	width  int
	asking bool // the dashboard question is open
	always bool // open the dashboard without asking
	status string
	done   map[string]bool // every change written this run
}

type choiceSaved struct {
	changes map[string]bool
	err     error
}

type choiceOpened struct {
	link string
	err  error
}

// NewChoice lists cfg's features; always skips the dashboard question.
func NewChoice(cfg config.Config, always bool, deps ChoiceDeps) Choice {
	m := Choice{deps: deps, always: always, done: map[string]bool{}, width: 80}
	for _, f := range config.Features {
		m.saved = append(m.saved, f.Set(cfg)) // the file's value: a member keeps it while its group is off
	}
	m.on = append([]bool(nil), m.saved...)
	return m
}

// Saved is every change written while the list was open.
func (m Choice) Saved() map[string]bool { return m.done }

// Pending is how many shown changes are not written.
func (m Choice) Pending() int { return len(m.changes()) }

func (m Choice) changes() map[string]bool {
	c := map[string]bool{}
	for i, f := range config.Features {
		if m.on[i] != m.saved[i] {
			c[f.ID] = m.on[i]
		}
	}
	return c
}

func (m Choice) Init() tea.Cmd { return nil }

func (m Choice) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case choiceSaved:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		var names []string
		for i, f := range config.Features {
			if v, ok := msg.changes[f.ID]; ok {
				m.saved[i] = v
				m.done[f.ID] = v
				names = append(names, f.Name+" "+onOff(v))
			}
		}
		m.status = "saved: " + strings.Join(names, ", ")
	case choiceOpened:
		if msg.err != nil {
			m.status = "dashboard: " + msg.err.Error()
		} else {
			m.status = "dashboard: " + msg.link
		}
	case tea.KeyPressMsg:
		if m.asking {
			return m.askKey(msg.String())
		}
		return m.listKey(msg.String())
	}
	return m, nil
}

func (m Choice) listKey(k string) (tea.Model, tea.Cmd) {
	last := len(config.Features)
	switch k {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cur = max(m.cur-1, 0)
	case "down", "j":
		m.cur = min(m.cur+1, last)
	case "g", "home":
		m.cur = 0
	case "G", "end":
		m.cur = last
	case "space", " ":
		if m.cur < last {
			m.on[m.cur] = !m.on[m.cur]
			m.status = ""
		}
	case "enter":
		if m.cur == last {
			if m.always {
				return m, m.open(false)
			}
			m.asking = true
			return m, nil
		}
		c := m.changes()
		if len(c) == 0 {
			m.status = "nothing to save"
			return m, nil
		}
		save := m.deps.Save
		return m, func() tea.Msg { return choiceSaved{c, save(c)} }
	}
	return m, nil
}

func (m Choice) askKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "o", "enter":
		m.asking = false
		return m, m.open(false)
	case "a":
		m.asking, m.always = false, true
		return m, m.open(true)
	case "n", "esc", "q":
		m.asking = false
	}
	return m, nil
}

func (m Choice) open(remember bool) tea.Cmd {
	d := m.deps
	return func() tea.Msg {
		if remember {
			if err := d.Remember(); err != nil {
				return choiceOpened{err: err}
			}
		}
		link, err := d.Open()
		return choiceOpened{link, err}
	}
}

func (m Choice) View() tea.View { return tea.NewView(m.draw()) }

func (m Choice) draw() string {
	nameW := len("Open dashboard")
	for _, f := range config.Features {
		nameW = max(nameW, len(f.Name)+len(choiceIndent(f)))
	}
	var b strings.Builder
	line := func(s string) { b.WriteString(ansi.Truncate(s, m.width, "…") + "\n") }
	line(sHead.Render("tock") + sDim.Render(" · choices   space turn on/off · enter save · q quit"))
	group := ""
	for i, f := range config.Features {
		if f.Group != group {
			group = f.Group
			line("")
			if !f.IsGroup {
				line(sDim.Render(group))
			}
		}
		dot := sFaint.Render("○")
		if m.on[i] {
			dot = sGood.Render("●")
		}
		tag := "         "
		switch {
		case m.on[i] != m.saved[i]:
			tag = sAccent.Render("changed") + "  "
		case m.on[i] && m.waits(f):
			tag = sFaint.Render("waits") + "    "
		}
		indent := choiceIndent(f)
		name := pad(f.Name, nameW-len(indent))
		if f.IsGroup {
			name = sHead.Render(name)
		}
		line(m.cursor(i) + indent + dot + " " + name + "  " + tag + sDim.Render(f.Note))
	}
	line("")
	line(m.cursor(len(config.Features)) + sBrand.Render("↗") + " " + pad("Open dashboard", nameW) + "           " +
		sDim.Render("in your browser, on this machine only"))
	if m.asking {
		line("")
		line("    " + sHead.Render("Open the web dashboard?"))
		line("    " + sDim.Render("It runs on this machine and stops after 30 minutes unused."))
		line("    " + sBrand.Render("o") + " Open   " + sBrand.Render("n") + " Not now   " + sBrand.Render("a") + " Always")
	}
	line("")
	status := m.status
	if n := m.Pending(); n > 0 && status == "" {
		status = fmt.Sprintf("%d not saved · enter to save", n)
	}
	b.WriteString(ansi.Truncate(sDim.Render(status), m.width, "…"))
	return b.String()
}

// choiceIndent sets a group's members under its toggle.
func choiceIndent(f config.Feature) string {
	if f.Needs != "" && config.FeatureByID(f.Needs).Group == f.Group {
		return "  "
	}
	return ""
}

// waits is whether f's group is shown off.
func (m Choice) waits(f config.Feature) bool {
	if f.Needs == "" {
		return false
	}
	for i, g := range config.Features {
		if g.ID == f.Needs {
			return !m.on[i]
		}
	}
	return false
}

func (m Choice) cursor(i int) string {
	if i == m.cur {
		return sBrand.Render("› ")
	}
	return "  "
}

func pad(s string, w int) string { return s + strings.Repeat(" ", max(w-len(s), 0)) }

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
