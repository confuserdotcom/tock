package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// rulesTab lists the learned rules, then the rules.toml rules, in the order
// they are checked. The cursor indexes rows.
type rulesTab struct {
	rows    []classify.Rule              // learned rules first
	learned int                          // how many of rows are learned
	stats   map[string]classify.RuleStat // by Rule.Ref(); nil when unavailable
	cursor  int
	top     int // first visible line
	height  int
}

// rulesSnap is the ruleset copied out of the app for one refresh, with the
// rule stats of the last app.StatsDays days.
type rulesSnap struct {
	learned, rules []classify.Rule
	stats          map[string]classify.RuleStat
}

func snapRules(a *app.App) *rulesSnap {
	rs := a.Rules()
	if rs == nil {
		return &rulesSnap{}
	}
	s := &rulesSnap{
		learned: append([]classify.Rule{}, rs.Learned...),
		rules:   append([]classify.Rule{}, rs.Rules...),
	}
	if stats, err := a.RuleStats(app.StatsDays); err == nil {
		s.stats = make(map[string]classify.RuleStat, len(stats))
		for _, st := range stats {
			s.stats[st.Rule.Ref()] = st
		}
	}
	return s
}

func (t *rulesTab) set(s rulesSnap) {
	t.rows = append(append([]classify.Rule{}, s.learned...), s.rules...)
	t.learned = len(s.learned)
	t.stats = s.stats
	t.clampScroll()
}

// statCell is the time r decided, right-aligned, or its status ("unused",
// "shadowed"); "" without stats.
func (t *rulesTab) statCell(r classify.Rule) string {
	st, ok := t.stats[r.Ref()]
	if !ok {
		return ""
	}
	if s := st.Status(); s != "" {
		return fmt.Sprintf("%8s", s)
	}
	return fmt.Sprintf("%8s", report.Dur(st.DecidedMS))
}

func (t *rulesTab) selected() *classify.Rule {
	if t.cursor < len(t.rows) {
		return &t.rows[t.cursor]
	}
	return nil
}

// lineOf is the line of row i: section titles and the blank line between
// the sections come before it.
func (t *rulesTab) lineOf(i int) int {
	if t.learned == 0 {
		return i + 1
	}
	if i < t.learned {
		return i + 1
	}
	return i + 3
}

func (t *rulesTab) lines() int {
	if len(t.rows) == 0 {
		return 2
	}
	return t.lineOf(len(t.rows)-1) + 1
}

func (t *rulesTab) clampScroll() {
	h := max(t.height, 1)
	t.cursor = min(max(t.cursor, 0), max(len(t.rows)-1, 0))
	cl := t.lineOf(t.cursor)
	if t.cursor == 0 {
		cl = 0 // keep the first section title visible
	}
	if cl < t.top {
		t.top = cl
	}
	if l := t.lineOf(t.cursor); l >= t.top+h {
		t.top = l - h + 1
	}
	t.top = max(min(t.top, t.lines()-h), 0)
}

// sets is what a rule assigns: "Code / tock", "Code" or "· / tock".
func sets(r classify.Rule) string {
	switch {
	case r.Project == "":
		return r.Category
	case r.Category == "":
		return "· / " + r.Project
	}
	return r.Category + " / " + r.Project
}

// rowAt is the rule shown on body line line, if any.
func (t *rulesTab) rowAt(line int) (int, bool) {
	if line < 0 || line >= max(t.height, 1) {
		return 0, false
	}
	for i := range t.rows {
		if t.lineOf(i) == t.top+line {
			return i, true
		}
	}
	return 0, false
}

func (t rulesTab) view(w, h int) []string {
	var lines []string
	if len(t.rows) == 0 {
		return []string{" " + sHead.Render(classify.RulesFile+" · 0 rules"), " " + sDim.Render("no rules · e opens rules.toml")}
	}
	title := func(s string) string { return " " + sHead.Render(s) }
	if t.learned > 0 {
		lines = append(lines, title(classify.LearnedFile+" · checked first · "+plural(t.learned, "rule")))
	}
	setsW := 0 // with stats, sets are left-aligned so the time column lines up
	for _, r := range t.rows {
		setsW = max(setsW, ansi.StringWidth(sets(r)))
	}
	for i, r := range t.rows {
		if i == t.learned {
			if t.learned > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, title(classify.RulesFile+" · "+plural(len(t.rows)-t.learned, "rule")))
		}
		ref := fmt.Sprint(r.Index)
		if r.File == classify.LearnedFile {
			ref = "L" + ref
		}
		mark := "  "
		if i == t.cursor {
			mark = "▸ "
		}
		right := sets(r)
		stat := t.statCell(r)
		if stat != "" {
			right = stat + "  " + right + strings.Repeat(" ", setsW-ansi.StringWidth(right))
		}
		var l string
		lw := max(w-ansi.StringWidth(right)-3, 10)
		left := fit(fmt.Sprintf("%s%-4s %s", mark, ref, r.Describe()), lw) + "  "
		switch {
		case i == t.cursor:
			l = sSel.Render(fit(left+right, w-1))
		case stat != "" && strings.TrimSpace(stat) == t.stats[r.Ref()].Status():
			l = sText.Render(left) + sDim.Render(stat) + sText.Render(right[len(stat):])
		default:
			l = sText.Render(left + right)
		}
		lines = append(lines, " "+l)
	}
	end := min(t.top+max(h, 1), len(lines))
	return lines[min(t.top, end):end]
}

func (m Model) rulesKey(k string) (tea.Model, tea.Cmd) {
	t := &m.rules
	switch k {
	case "j", "down":
		t.cursor++
	case "k", "up":
		t.cursor--
	case "g", "home":
		t.cursor = 0
	case "G", "end":
		t.cursor = len(t.rows) - 1
	case "J", "K":
		r := t.selected()
		if r == nil {
			return m, nil
		}
		up := k == "K"
		return m, m.moveRule(*r, up)
	case "e":
		file := classify.RulesFile
		if r := t.selected(); r != nil {
			file = r.File
		}
		var path string
		m.src.do(func(a *app.App) { path = a.RulePath(file) })
		return m, m.exec(editorCmd(path), func(err error) tea.Msg {
			if err != nil {
				return doneMsg{err: fmt.Errorf("editor: %w", err)}
			}
			return doneMsg{}
		})
	}
	t.clampScroll()
	return m, nil
}

// movedMsg reports a rule move; on success the cursor goes to row.
type movedMsg struct {
	done doneMsg
	row  int
}

// moveRule moves r one place within its file. The cursor follows only once
// the move succeeded; the reload after it shows the new order.
func (m *Model) moveRule(r classify.Rule, up bool) tea.Cmd {
	dir, row := "down", m.rules.cursor+1
	if up {
		dir, row = "up", m.rules.cursor-1
	}
	src := m.src
	return func() tea.Msg {
		var msg movedMsg
		src.do(func(a *app.App) {
			err := a.MoveRule(r.File, r.Index, up)
			switch {
			case errors.Is(err, classify.ErrRuleEdge) && up:
				msg.done.err = errors.New("already first")
			case errors.Is(err, classify.ErrRuleEdge):
				msg.done.err = errors.New("already last")
			case err != nil:
				msg.done.err = err
			default:
				msg.done.text = fmt.Sprintf("moved %s %s", r.Ref(), dir)
				msg.row = row
			}
		})
		return msg
	}
}

// editorCmd opens path in $VISUAL, else $EDITOR, else vi. The variable may
// carry arguments ("code -w").
func editorCmd(path string) *exec.Cmd {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	f := strings.Fields(ed)
	if len(f) == 0 {
		f = []string{"vi"}
	}
	return exec.Command(f[0], append(f[1:], path)...)
}
