package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/config"
	"tock/internal/plan"
	"tock/internal/report"
)

// planEvery is how often the open plan tab reruns [plan] command.
const planEvery = time.Minute

// planCmdTimeout bounds one run of [plan] command.
const planCmdTimeout = time.Minute

// planHeadMax is how many lines of the command's output the tab shows.
const planHeadMax = 6

// planTab shows the [plan] todo.txt by day: tick, add, delete, edit.
type planTab struct {
	cfg    config.Plan
	lines  []string
	rows   []plan.Row
	day    string
	err    string
	loaded bool

	head []string         // the command's lines, shown above the list
	seen map[string]int64 // seconds per +project, from the command
	// cmdAt is when the command last started; force runs it at the next load.
	cmdAt   time.Time
	force   bool
	running bool

	cursor, top, height int

	adding bool
	input  string

	last   *planUndo
	follow string // the line the cursor goes to at the next load
}

// planUndo reverses the tab's last edit.
type planUndo struct {
	kind string // "toggle", "add", "delete"
	line string // the line as it is in the file after the edit
	prev string // the deleted line
	at   int    // its index
}

func (t *planTab) set(cfg config.Plan, lines []string, err error, now time.Time) {
	keep := t.follow
	if r := t.selected(); r != nil && keep == "" {
		keep = r.Raw
	}
	t.follow = ""
	t.cfg, t.lines, t.loaded, t.err = cfg, lines, true, ""
	if err != nil {
		t.err = err.Error()
	}
	t.day = now.Format("2006-01-02")
	t.rows = plan.Rows(lines, t.day, now.AddDate(0, 0, 1).Format("2006-01-02"))
	for i, r := range t.rows {
		if r.Raw == keep {
			t.cursor = i
			return
		}
	}
}

func (t *planTab) selected() *plan.Row {
	if t.cursor >= 0 && t.cursor < len(t.rows) {
		return &t.rows[t.cursor]
	}
	return nil
}

// planLine is one line of the list: a section heading (row -1) or a row.
type planLine struct {
	section int
	row     int
}

// planTop is the body lines above the list: the command's lines and a gap.
func (t *planTab) planTop() int {
	if len(t.head) == 0 {
		return 1
	}
	return min(len(t.head), planHeadMax) + 2
}

// layout is the list's lines: each section's heading, then its rows, with a
// blank line between sections.
func (t *planTab) layout() []planLine {
	var out []planLine
	sec := -1
	for i, r := range t.rows {
		if r.Section != sec {
			if sec >= 0 {
				out = append(out, planLine{section: -1, row: -1})
			}
			sec = r.Section
			out = append(out, planLine{section: sec, row: -1})
		}
		out = append(out, planLine{section: sec, row: i})
	}
	return out
}

func (t *planTab) clampScroll() {
	t.cursor = min(max(t.cursor, 0), max(len(t.rows)-1, 0))
	h := max(t.height-t.planTop(), 1)
	ls := t.layout()
	at, head := 0, 0
	for i, l := range ls {
		if l.row == t.cursor {
			at, head = i, i
			if i > 0 && ls[i-1].row < 0 && ls[i-1].section >= 0 {
				head = i - 1 // a section's first row brings its heading
			}
			break
		}
	}
	if head < t.top {
		t.top = head
	}
	if at >= t.top+h {
		t.top = at - h + 1
	}
	t.top = max(min(t.top, len(ls)-h), 0)
}

// rowAt is the row shown on body line line, if any.
func (t *planTab) rowAt(line int) (int, bool) {
	i := t.top + line - t.planTop()
	ls := t.layout()
	if line < t.planTop() || i < 0 || i >= len(ls) || ls[i].row < 0 {
		return 0, false
	}
	return ls[i].row, true
}

// due reports that the open tab should run its command now.
func (t *planTab) due(now time.Time) bool {
	return t.cfg.Command != "" && !t.running && (t.force || t.cmdAt.IsZero() || now.Sub(t.cmdAt) >= planEvery)
}

// planCmdMsg carries one run of the command.
type planCmdMsg struct {
	head []string
	seen map[string]int64
	err  error
}

// runCommand starts [plan] command in the background.
func (m *Model) runCommand() tea.Cmd {
	t := &m.plan
	t.running, t.force, t.cmdAt = true, false, m.now()
	cmdline, file := t.cfg.Command, t.cfg.File
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), planCmdTimeout)
		defer cancel()
		c := exec.CommandContext(ctx, "sh", "-c", cmdline)
		c.Env = append(os.Environ(), "TOCK_PLAN_FILE="+file)
		var stderr strings.Builder
		c.Stderr = &stderr
		out, err := c.Output()
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if i := strings.LastIndex(msg, "\n"); i >= 0 {
				msg = msg[i+1:]
			}
			if msg == "" {
				msg = err.Error()
			}
			return planCmdMsg{err: fmt.Errorf("plan command: %s", msg)}
		}
		head, seen := parseCommandOutput(out)
		return planCmdMsg{head: head, seen: seen}
	}
}

// parseCommandOutput reads {"lines": [...], "seen": {...}}, or plain lines.
func parseCommandOutput(out []byte) ([]string, map[string]int64) {
	var j struct {
		Lines []string         `json:"lines"`
		Seen  map[string]int64 `json:"seen"`
	}
	if err := json.Unmarshal(out, &j); err == nil {
		return nonEmpty(j.Lines), j.Seen
	}
	return nonEmpty(strings.Split(string(out), "\n")), nil
}

func nonEmpty(ls []string) []string {
	var out []string
	for _, l := range ls {
		if l = strings.TrimRight(l, " \t\r"); strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func (m Model) planCommandDone(msg planCmdMsg) (tea.Model, tea.Cmd) {
	m.plan.running = false
	if msg.err != nil {
		m.setFlash(msg.err.Error(), true)
		return m, nil
	}
	m.plan.head, m.plan.seen = msg.head, msg.seen
	return m, m.reload() // the command may have rewritten the file
}

// planEditedMsg reports one edit of the file.
type planEditedMsg struct {
	text   string
	err    error
	undo   *planUndo
	follow string // the edited line, for the cursor
}

func (m Model) planEdited(msg planEditedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setFlash(msg.err.Error(), true)
	} else {
		m.setFlash(msg.text, false)
		m.plan.last, m.plan.follow = msg.undo, msg.follow
	}
	m.plan.force = true
	return m, m.reload()
}

// editPlan applies f to the line old (see plan.Edit) in the background.
func (m Model) editPlan(old string, at int, f func(string) *string, done func(newLine string) planEditedMsg) tea.Cmd {
	path := m.plan.cfg.File
	return func() tea.Msg {
		var got string
		err := plan.Edit(path, old, at, func(l string) *string {
			n := f(l)
			if n != nil {
				got = *n
			}
			return n
		})
		if err != nil {
			return planEditedMsg{err: err}
		}
		return done(got)
	}
}

func (m Model) planToggle(r plan.Row) tea.Cmd {
	day := m.now().Format("2006-01-02")
	return m.editPlan(r.Raw, -1, func(l string) *string { s := plan.Toggle(l, day); return &s },
		func(n string) planEditedMsg {
			what := "done: "
			if !plan.Parse(n).Done {
				what = "not done: "
			}
			return planEditedMsg{text: what + report.Safe(r.Text) + " (u undo)", undo: &planUndo{kind: "toggle", line: n}, follow: n}
		})
}

func (m Model) planDelete(r plan.Row) tea.Cmd {
	at := plan.Index(m.plan.lines, r.Raw)
	return m.editPlan(r.Raw, -1, func(string) *string { return nil },
		func(string) planEditedMsg {
			return planEditedMsg{text: "deleted: " + report.Safe(r.Text) + " (u undo)", undo: &planUndo{kind: "delete", prev: r.Raw, at: at}}
		})
}

func (m Model) planAdd(text string) tea.Cmd {
	line := plan.New(text, m.now().Format("2006-01-02"))
	return m.editPlan("", -1, func(string) *string { return &line },
		func(n string) planEditedMsg {
			return planEditedMsg{text: "added: " + report.Safe(plan.Parse(n).Text) + " (u undo)", undo: &planUndo{kind: "add", line: n}, follow: n}
		})
}

// planUndoLast reverses the tab's last edit.
func (m Model) planUndoLast() tea.Cmd {
	u, day := m.plan.last, m.now().Format("2006-01-02")
	m.plan.last = nil
	done := func(n string) planEditedMsg { return planEditedMsg{text: "undone", follow: n} }
	switch u.kind {
	case "toggle":
		return m.editPlan(u.line, -1, func(l string) *string { s := plan.Toggle(l, day); return &s }, done)
	case "add":
		return m.editPlan(u.line, -1, func(string) *string { return nil }, done)
	default:
		prev := u.prev
		return m.editPlan("", u.at, func(string) *string { return &prev }, done)
	}
}

// noPlanFile is the error for keys that need a file.
var noPlanFile = errors.New("no plan file · set [plan] file in config.toml")

func (m Model) planKey(k string) (tea.Model, tea.Cmd) {
	t := &m.plan
	needFile := func() bool {
		if t.cfg.File == "" {
			m.setFlash(noPlanFile.Error(), true)
			return false
		}
		return true
	}
	switch k {
	case "j", "down":
		t.cursor++
	case "k", "up":
		t.cursor--
	case "g", "home":
		t.cursor = 0
	case "G", "end":
		t.cursor = len(t.rows) - 1
	case "enter", "space", " ", "x":
		if r := t.selected(); r != nil {
			return m, m.planToggle(*r)
		}
	case "d":
		if r := t.selected(); r != nil {
			return m, m.planDelete(*r)
		}
	case "a":
		if needFile() {
			t.adding, t.input = true, ""
		}
	case "e":
		if !needFile() {
			return m, nil
		}
		return m, m.exec(editorCmd(t.cfg.File), func(err error) tea.Msg {
			if err != nil {
				return planEditedMsg{err: fmt.Errorf("editor: %w", err)}
			}
			return planEditedMsg{text: "saved"}
		})
	case "f":
		return m.focusKey("s")
	}
	return m, nil
}

func (m Model) planAddKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := &m.plan
	switch msg.String() {
	case "esc":
		t.adding = false
	case "enter":
		t.adding = false
		if strings.TrimSpace(t.input) == "" {
			return m, nil
		}
		return m, m.planAdd(t.input)
	case "backspace":
		if r := []rune(t.input); len(r) > 0 {
			t.input = string(r[:len(r)-1])
		}
	default:
		if msg.Text != "" {
			t.input += msg.Text
		}
	}
	return m, nil
}

// addView is the add box.
func (t *planTab) addView(w int) []string {
	cw := min(w-8, 72)
	return center2(card("Add a task", []string{"",
		sBright.Render(fit(t.input+"▏", cw-4)),
		"",
		sDim.Render(fit("(A) priority · +project · @context · due:YYYY-MM-DD", cw-4)),
	}, cw), w)
}

func (t planTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	if t.cfg.File == "" {
		return emptyState(w, "Plan",
			sText.Render("No todo.txt to show."),
			sDim.Render(`Set it in config.toml:  [plan] file = "~/todo.txt"`))
	}
	var out []string
	if len(t.head) > 0 {
		out = append(out, "")
		for _, l := range t.head[:min(len(t.head), planHeadMax)] {
			out = append(out, " "+sText.Render(fit(report.Safe(l), w-2)))
		}
	}
	out = append(out, "")
	if t.err != "" {
		return append(out, " "+sAccent.Render(fit(t.err, w-2)))
	}
	if len(t.rows) == 0 {
		return append(out, emptyState(w, "Plan", sGood.Render("✓ Nothing planned."), sDim.Render("a adds a task"))...)
	}
	left := [plan.NumSections]int{}
	for _, r := range t.rows {
		if !r.Done {
			left[r.Section]++
		}
	}
	shown := map[string]bool{} // seen time goes on a project's first open row today
	ls := t.layout()
	end := min(t.top+max(h-len(out), 1), len(ls))
	for _, l := range ls[t.top:end] {
		switch {
		case l.section < 0:
			out = append(out, "")
		case l.row < 0:
			name := sHead.Render(plan.SectionNames[l.section])
			if l.section == plan.Overdue {
				name = sAccent.Bold(true).Render(plan.SectionNames[l.section])
			}
			if n := left[l.section]; n > 0 && l.section != plan.Done {
				name += sDim.Render(fmt.Sprintf(" · %d left", n))
			}
			out = append(out, " "+name)
		default:
			out = append(out, t.rowView(l.row, w, shown))
		}
	}
	return out
}

func (t planTab) rowView(i, w int, shown map[string]bool) string {
	r := t.rows[i]
	box, text := sDim.Render("[ ]"), sText
	if r.Done {
		box, text = sGood.Render("[x]"), sFaint
	}
	pri := "  "
	if r.Pri != "" && !r.Done {
		st := sDim
		switch r.Pri {
		case "A", "D":
			st = sAccent
		case "B":
			st = sShallow
		}
		pri = st.Render(r.Pri) + " "
	}
	var tags []string
	for _, c := range r.Contexts {
		tags = append(tags, "@"+c)
	}
	for _, p := range r.Projects {
		tags = append(tags, "+"+p)
	}
	tail := sDim.Render(strings.Join(tags, " "))
	if !r.Done && (r.Section == plan.Today || r.Section == plan.Overdue) {
		for _, p := range r.Projects {
			if s := t.seen[p]; s > 0 && !shown[p] {
				shown[p] = true
				tail += "  " + sBrand.Render(report.Dur(s*1000)+" seen")
			}
		}
	}
	// "▸" or " ", then box, priority and text; the tags take what is left.
	tw := ansiWidth(tail)
	room := w - 1 - 4 - 2 - 2
	textW := max(room-tw, min(room, 24))
	if textW+tw > room {
		tail = fit(tail, max(room-textW, 0))
	}
	line := box + " " + pri + text.Render(fit(report.Safe(r.Text), textW))
	line = spread(line, tail+" ", w-1)
	if i == t.cursor {
		return selRow(sBrand.Bold(true).Render("▸")+line, w)
	}
	return " " + line
}
