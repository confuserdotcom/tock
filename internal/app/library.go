package app

import (
	"os"
	"strings"

	"tock/internal/classify"
)

// WithRulesUndo runs fn, a change to rules.toml, and journals it so Undo
// can put rules.toml back. A change that touched learned.toml too (rename,
// merge) or changed nothing is not journaled. If another process writes
// rules.toml between fn and the read after it, the entry covers that write
// too; Undo still refuses once the file differs from what it recorded.
func (a *App) WithRulesUndo(what string, fn func() error) error {
	before, rerr := os.ReadFile(a.paths.Rules)
	learned, _ := os.ReadFile(a.paths.Learned)
	if err := fn(); err != nil {
		return err
	}
	after, err := os.ReadFile(a.paths.Rules)
	if err != nil {
		return nil // the change stands; there is just nothing to journal
	}
	learnedNow, _ := os.ReadFile(a.paths.Learned)
	if string(after) == string(before) || string(learnedNow) != string(learned) {
		return nil
	}
	return a.journal(what, undoData{File: &undoFile{Path: a.paths.Rules, Existed: rerr == nil, Before: string(before), After: string(after)}})
}

// LibraryDays is how far back Library looks for a name's last use.
const LibraryDays = 90

const libDayMS = 24 * 3_600_000

// LibraryUse is a name's time in the last 30 days and the end of its last
// labelled time; LastSeenMS 0 = not in the last LibraryDays.
type LibraryUse struct{ Last30MS, LastSeenMS int64 }

// LibCategory is a declared category, how many projects file under it, and
// its use.
type LibCategory struct {
	classify.Category
	Projects int
	LibraryUse
}

// LibProject is a declared project (archived ones too) and its use.
type LibProject struct {
	classify.Project
	LibraryUse
}

// Library is every name the rules declare, with its use, for the
// dashboard's Library view.
type Library struct {
	Categories []LibCategory
	Projects   []LibProject
	Goals      []classify.Goal
	Lookalikes []classify.Dup
}

// Library labels the last LibraryDays once and returns each declared
// category and project with its time in the last 30 days and when it was
// last seen, plus the goals and the look-alike name pairs.
func (a *App) Library() (Library, error) {
	now := a.nowMS()
	vs, labels, err := a.labelled(now-LibraryDays*libDayMS, now, now)
	if err != nil {
		return Library{}, err
	}
	since := now - 30*libDayMS
	cat, proj := map[string]*LibraryUse{}, map[string]*LibraryUse{}
	add := func(m map[string]*LibraryUse, name string, v classify.SpanView) {
		if name == "" {
			return
		}
		k := strings.ToLower(name)
		u := m[k]
		if u == nil {
			u = &LibraryUse{}
			m[k] = u
		}
		u.Last30MS += max(0, min(v.End, now)-max(v.Start, since))
		u.LastSeenMS = max(u.LastSeenMS, min(v.End, now))
	}
	for i, v := range vs {
		add(cat, labels[i].Category, v)
		add(proj, labels[i].Project, v)
	}
	use := func(m map[string]*LibraryUse, name string) LibraryUse {
		if u := m[strings.ToLower(name)]; u != nil {
			return *u
		}
		return LibraryUse{}
	}
	var lib Library
	for _, c := range a.rs.Categories {
		n := 0
		for _, p := range a.rs.Projects {
			if strings.EqualFold(p.Category, c.Name) {
				n++
			}
		}
		lib.Categories = append(lib.Categories, LibCategory{Category: c, Projects: n, LibraryUse: use(cat, c.Name)})
	}
	for _, p := range a.rs.Projects {
		lib.Projects = append(lib.Projects, LibProject{Project: p, LibraryUse: use(proj, p.Name)})
	}
	lib.Goals = append(lib.Goals, a.rs.Goals...)
	lib.Lookalikes = classify.Duplicates(a.rs)
	return lib, nil
}
