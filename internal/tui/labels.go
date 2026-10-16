package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// LabelDays is the window of the labels tab's times.
const LabelDays = 30

// labelsTab is the categories, each with its modules, then the projects
// filed under no category: where you make your own labels.
type labelsTab struct {
	tree      []app.TreeCategory
	loaded    bool
	collapsed map[string]bool // category names folded shut
	cursor    int
	top       int
	height    int
}

// labelRow is one line of the tree: a category (proj < 0) or a project of
// tree[cat]. A category with Name "" is the "no category" group.
type labelRow struct {
	cat, proj int
}

func (t *labelsTab) set(tree []app.TreeCategory) {
	t.tree, t.loaded = tree, true
	t.clampScroll()
}

func (t *labelsTab) rows() []labelRow {
	var out []labelRow
	for ci, c := range t.tree {
		out = append(out, labelRow{ci, -1})
		if t.collapsed[c.Name] {
			continue
		}
		for pi := range c.Projects {
			out = append(out, labelRow{ci, pi})
		}
	}
	return out
}

// selected is the row under the cursor: its category and project (nil on
// a category row). ok is false with no rows.
func (t *labelsTab) selected() (c *app.TreeCategory, p *app.TreeProject, ok bool) {
	rs := t.rows()
	if t.cursor < 0 || t.cursor >= len(rs) {
		return nil, nil, false
	}
	r := rs[t.cursor]
	c = &t.tree[r.cat]
	if r.proj >= 0 {
		p = &c.Projects[r.proj]
	}
	return c, p, true
}

func (t *labelsTab) clampScroll() {
	n := len(t.rows())
	t.cursor = min(max(t.cursor, 0), max(n-1, 0))
	h := max(t.height-2, 1) // the title and the blank line under it
	if t.cursor < t.top {
		t.top = t.cursor
	}
	if t.cursor >= t.top+h {
		t.top = t.cursor - h + 1
	}
	t.top = max(min(t.top, n-h), 0)
}

func (t labelsTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	out := []string{spread(" "+sHead.Render("Categories & modules")+sDim.Render(" · last 30 days"),
		keycap("a", "category")+"   "+keycap("m", "module")+" ", w), ""}
	var maxMS int64
	for _, c := range t.tree {
		maxMS = max(maxMS, c.MS)
	}
	rs := t.rows()
	end := min(t.top+max(h-2, 1), len(rs))
	for i := t.top; i < end; i++ {
		out = append(out, t.line(i, rs[i], w, maxMS))
	}
	if len(rs) == 0 {
		out = append(out, sDim.Render("   no categories yet · a adds one"))
	}
	return out
}

// line renders row r (number i) w cells wide.
func (t labelsTab) line(i int, r labelRow, w int, maxMS int64) string {
	sel := i == t.cursor
	mark := "  "
	if sel {
		mark = sBrand.Bold(true).Render("▸ ")
	}
	c := t.tree[r.cat]
	const nameW = 22
	barW := max(min(w-nameW-48, 24), 6)
	id := strconv.Itoa(i)
	var line string
	switch {
	case r.proj < 0 && c.Name == "":
		line = mark + sDim.Render(fit("no category", nameW+2))
	case r.proj < 0:
		fold := "▾ "
		if t.collapsed[c.Name] {
			fold = "▸ "
		}
		if len(c.Projects) == 0 {
			fold = "  "
		}
		st := kindStyle(c.Kind)
		kind := z("lab:kind:"+id, st.Render(fit(c.Kind, 8)))
		afk := sFaint.Render("afk ") + sDim.Render(classify.FormatDur(windowOf(c.Category)))
		if c.AFKMS > 0 {
			afk = sFaint.Render("afk ") + sText.Render(classify.FormatDur(c.AFKMS))
		}
		extra := ""
		if n := len(c.Projects); n > 0 {
			extra = sFaint.Render(" · ") + sDim.Render(plural(n, "module"))
		}
		line = mark + sFaint.Render(fold) + dot(c.Kind) + " " + st.Bold(true).Render(fit(report.Safe(c.Name), nameW-2)) + "  " +
			kind + "  " + sBright.Render(fitRight(dashDur(c.MS), 6)) + "  " + meter(c.MS, maxMS, barW, st) + "  " +
			z("lab:afk:"+id, afk) + extra
	default:
		p := c.Projects[r.proj]
		st := kindStyle(c.Kind)
		if c.Name == "" {
			st = sText
		}
		name := report.Safe(p.Name)
		var notes []string
		if len(p.Aliases) > 0 {
			notes = append(notes, report.Safe(strings.Join(p.Aliases, ", ")))
		}
		if p.Archived {
			notes = append(notes, "archived")
		}
		if len(p.Repos) > 0 {
			notes = append(notes, plural(len(p.Repos), "repo"))
		}
		if p.Rate != nil {
			notes = append(notes, fmt.Sprintf("rate %g", *p.Rate))
		}
		line = mark + "      " + sText.Render(fit(name, nameW-4)) + "  " + strings.Repeat(" ", 8) + "  " +
			sBright.Render(fitRight(dashDur(p.MS), 6)) + "  " + meter(p.MS, maxMS, barW, st) + "  " +
			z("lab:alias:"+id, sDim.Render(strings.Join(notes, " · ")))
	}
	line = z("row:"+id, fit(line, w))
	if sel {
		return selRow(line, w)
	}
	return line
}

func dashDur(ms int64) string {
	if ms == 0 {
		return "—"
	}
	return report.Dur(ms)
}

// windowOf is a category's AFK window as shown: its own, else the default.
func windowOf(c classify.Category) int64 {
	if c.AFKMS > 0 {
		return c.AFKMS
	}
	return classify.DefaultAFKMS
}

func (m Model) labelsKey(k string) (tea.Model, tea.Cmd) {
	t := &m.labels
	switch k {
	case "j", "down":
		t.cursor++
	case "k", "up":
		t.cursor--
	case "g", "home":
		t.cursor = 0
	case "G", "end":
		t.cursor = len(t.rows()) - 1
	case "a":
		return m.askNewCategory()
	case "m":
		return m.askNewModule()
	case "enter", " ":
		c, p, ok := t.selected()
		switch {
		case !ok:
		case p == nil && len(c.Projects) > 0:
			if t.collapsed == nil {
				t.collapsed = map[string]bool{}
			}
			t.collapsed[c.Name] = !t.collapsed[c.Name]
		case p != nil:
			return m.askAliases()
		}
	case "e":
		return m.askRename()
	case "K":
		return m.chooseKind()
	case "w":
		return m.chooseAFK()
	case "f":
		return m.chooseFile()
	case "l":
		return m.askAliases()
	case "x":
		return m.toggleArchive()
	case "d":
		return m.confirmDelete()
	case "M":
		return m.chooseMerge()
	}
	t.clampScroll()
	return m, nil
}

// labelsChip handles a click on a row's kind, afk or alias chip.
func (m Model) labelsChip(arg string) (tea.Model, tea.Cmd, bool) {
	what, n, _ := strings.Cut(arg, ":")
	i, err := strconv.Atoi(n)
	if err != nil {
		return m, nil, false
	}
	m.labels.cursor = i
	m.labels.clampScroll()
	var next tea.Model
	var cmd tea.Cmd
	switch what {
	case "kind":
		next, cmd = m.chooseKind()
	case "afk":
		next, cmd = m.chooseAFK()
	case "alias":
		next, cmd = m.askAliases()
	default:
		return m, nil, false
	}
	return next, cmd, true
}

// labelAct runs an app change and reloads, flashing what happened.
func (m Model) labelAct(f func(a *app.App) (string, error)) (tea.Model, tea.Cmd) {
	return m, m.act(f)
}

func (m Model) askNewCategory() (tea.Model, tea.Cmd) {
	m.ask = &askDlg{title: "New category", prompt: "its name", done: func(m Model, name string) (tea.Model, tea.Cmd) {
		if name == "" {
			return m, nil
		}
		m.choose = &chooseDlg{title: "Kind of " + name, opts: kindChoices(), done: func(m Model, c choice) (tea.Model, tea.Cmd) {
			return m.labelAct(func(a *app.App) (string, error) {
				return "added category " + name + " (" + c.value + ")", a.AddCategory(app.CategorySpec{Name: name, Kind: c.value})
			})
		}}
		return m, nil
	}}
	return m, nil
}

func (m Model) askNewModule() (tea.Model, tea.Cmd) {
	c, _, ok := m.labels.selected()
	if !ok || c.Name == "" {
		m.setFlash("select a category first: its modules go under it", true)
		return m, nil
	}
	cat := c.Name
	m.ask = &askDlg{title: "New module of " + cat, prompt: "its name; tock then finds it in window titles", done: func(m Model, name string) (tea.Model, tea.Cmd) {
		if name == "" {
			return m, nil
		}
		return m.labelAct(func(a *app.App) (string, error) {
			return "added module " + name + " under " + cat, a.AddProject(app.ProjectSpec{Name: name, Category: &cat})
		})
	}}
	return m, nil
}

func (m Model) askRename() (tea.Model, tea.Cmd) {
	c, p, ok := m.labels.selected()
	if !ok || (p == nil && c.Name == "") {
		return m, nil
	}
	kind, old := classify.NameCategory, c.Name
	if p != nil {
		kind, old = classify.NameProject, p.Name
	}
	m.ask = &askDlg{title: "Rename " + old, prompt: "everywhere: rules, goals and the time you tagged", text: old,
		done: func(m Model, name string) (tea.Model, tea.Cmd) {
			if name == "" || name == old {
				return m, nil
			}
			return m.labelAct(func(a *app.App) (string, error) {
				_, err := a.RenameName(kind, old, name)
				return "renamed " + old + " to " + name, err
			})
		}}
	return m, nil
}

func kindChoices() []choice {
	notes := map[string]string{
		classify.KindDeep: "focused work: counts for focus", classify.KindShallow: "work, not focus",
		classify.KindNeutral: "neither", classify.KindBrowse: "a third weighs like a distraction",
		classify.KindDistract: "nudged and blocked in sessions",
	}
	out := make([]choice, len(classify.Kinds))
	for i, k := range classify.Kinds {
		out[i] = choice{label: dot(k) + " " + k, value: k, note: notes[k]}
	}
	return out
}

func (m Model) chooseKind() (tea.Model, tea.Cmd) {
	c, p, ok := m.labels.selected()
	if !ok || p != nil || c.Name == "" {
		return m, nil
	}
	name := c.Name
	opts := kindChoices()
	d := &chooseDlg{title: "Kind of " + name, opts: opts, done: func(m Model, ch choice) (tea.Model, tea.Cmd) {
		return m.labelAct(func(a *app.App) (string, error) {
			return name + " is " + ch.value + " now", a.EditCategory(app.CategorySpec{Name: name, Kind: ch.value})
		})
	}}
	for i, o := range opts {
		if o.value == c.Kind {
			d.cursor = i
		}
	}
	m.choose = d
	return m, nil
}

func (m Model) chooseAFK() (tea.Model, tea.Cmd) {
	c, p, ok := m.labels.selected()
	if !ok || p != nil || c.Name == "" {
		return m, nil
	}
	name := c.Name
	opts := []choice{{label: "default", value: "-1", note: "5m without input"}}
	for _, mins := range []int{5, 10, 15, 20, 30, 45, 60, 90, 120} {
		opts = append(opts, choice{label: classify.FormatDur(int64(mins) * 60_000), value: strconv.Itoa(mins * 60_000)})
	}
	d := &chooseDlg{title: "AFK window of " + name, opts: opts, done: func(m Model, ch choice) (tea.Model, tea.Cmd) {
		ms, _ := strconv.ParseInt(ch.value, 10, 64)
		return m.labelAct(func(a *app.App) (string, error) {
			return name + ": afk " + ch.label, a.EditCategory(app.CategorySpec{Name: name, AFKMS: ms})
		})
	}}
	for i, o := range opts {
		if o.value == strconv.FormatInt(c.AFKMS, 10) {
			d.cursor = i
		}
	}
	m.choose = d
	return m, nil
}

func (m Model) chooseFile() (tea.Model, tea.Cmd) {
	_, p, ok := m.labels.selected()
	if !ok || p == nil {
		m.setFlash("select a project or module to file it", true)
		return m, nil
	}
	name := p.Name
	opts := []choice{{label: "no category", value: "", note: "a plain project"}}
	for _, c := range m.labels.tree {
		if c.Name != "" {
			opts = append(opts, choice{label: dot(c.Kind) + " " + c.Name, value: c.Name})
		}
	}
	d := &chooseDlg{title: "File " + name + " under", opts: opts, done: func(m Model, ch choice) (tea.Model, tea.Cmd) {
		cat := ch.value
		return m.labelAct(func(a *app.App) (string, error) {
			what := name + " is a module of " + cat
			if cat == "" {
				what = name + " is under no category"
			}
			return what, a.EditProject(app.ProjectSpec{Name: name, Category: &cat})
		})
	}}
	for i, o := range opts {
		if o.value == p.Category {
			d.cursor = i
		}
	}
	m.choose = d
	return m, nil
}

func (m Model) askAliases() (tea.Model, tea.Cmd) {
	_, p, ok := m.labels.selected()
	if !ok || p == nil {
		return m, nil
	}
	name := p.Name
	m.ask = &askDlg{title: "Other names of " + name, prompt: "comma-separated: todo +tags, words in titles",
		text: strings.Join(p.Aliases, ", "), done: func(m Model, text string) (tea.Model, tea.Cmd) {
			var als []string
			for _, a := range strings.Split(text, ",") {
				if a = strings.TrimSpace(a); a != "" {
					als = append(als, a)
				}
			}
			return m.labelAct(func(a *app.App) (string, error) {
				return name + ": " + plural(len(als), "alias"), a.EditProject(app.ProjectSpec{Name: name, Aliases: &als})
			})
		}}
	return m, nil
}

func (m Model) toggleArchive() (tea.Model, tea.Cmd) {
	_, p, ok := m.labels.selected()
	if !ok || p == nil {
		return m, nil
	}
	name, arch := p.Name, !p.Archived
	return m.labelAct(func(a *app.App) (string, error) {
		what := "archived " + name
		if !arch {
			what = "unarchived " + name
		}
		return what, a.EditProject(app.ProjectSpec{Name: name, Archived: &arch})
	})
}

func (m Model) confirmDelete() (tea.Model, tea.Cmd) {
	c, p, ok := m.labels.selected()
	if !ok || (p == nil && c.Name == "") {
		return m, nil
	}
	kind, name := classify.NameCategory, c.Name
	if p != nil {
		kind, name = classify.NameProject, p.Name
	}
	m.choose = &chooseDlg{title: "Delete " + name + "?", opts: []choice{
		{label: "keep it", value: ""},
		{label: "delete " + name, value: "y", note: "only when nothing names it"},
	}, done: func(m Model, ch choice) (tea.Model, tea.Cmd) {
		if ch.value != "y" {
			return m, nil
		}
		return m.labelAct(func(a *app.App) (string, error) {
			_, err := a.RemoveName(kind, name)
			return "deleted " + name, err
		})
	}}
	return m, nil
}

func (m Model) chooseMerge() (tea.Model, tea.Cmd) {
	c, p, ok := m.labels.selected()
	if !ok || (p == nil && c.Name == "") {
		return m, nil
	}
	kind, name := classify.NameCategory, c.Name
	var opts []choice
	if p == nil {
		for _, o := range m.labels.tree {
			if o.Name != "" && o.Name != name {
				opts = append(opts, choice{label: dot(o.Kind) + " " + o.Name, value: o.Name})
			}
		}
	} else {
		kind, name = classify.NameProject, p.Name
		for _, o := range m.labels.tree {
			for _, q := range o.Projects {
				if q.Name != name {
					opts = append(opts, choice{label: q.Name, value: q.Name, note: o.Name})
				}
			}
		}
	}
	if len(opts) == 0 {
		return m, nil
	}
	m.choose = &chooseDlg{title: "Merge " + name + " into", opts: opts, done: func(m Model, ch choice) (tea.Model, tea.Cmd) {
		return m.labelAct(func(a *app.App) (string, error) {
			_, _, err := a.MergeNames(kind, name, ch.value)
			return "merged " + name + " into " + ch.value, err
		})
	}}
	return m, nil
}
