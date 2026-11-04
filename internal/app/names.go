package app

import (
	"tock/internal/rulesfile"

	"tock/internal/classify"
)

// Dup is a pair of look-alike names with the time each labelled over the
// window Duplicates looked at.
type Dup struct {
	classify.Dup
	AMS, BMS int64
}

// Duplicates returns the look-alike category and project pairs, with the
// time each name labelled in the last days.
func (a *App) Duplicates(days int) ([]Dup, error) {
	dups := classify.Duplicates(a.rs)
	if len(dups) == 0 {
		return nil, nil
	}
	now := a.nowMS()
	vs, labels, err := a.labelled(now-int64(days)*24*60*60*1000, now, now)
	if err != nil {
		return nil, err
	}
	ms := map[string]map[string]int64{classify.NameCategory: {}, classify.NameProject: {}}
	for i, v := range vs {
		ms[classify.NameCategory][labels[i].Category] += v.DurMS()
		ms[classify.NameProject][labels[i].Project] += v.DurMS()
	}
	out := make([]Dup, len(dups))
	for i, d := range dups {
		out[i] = Dup{Dup: d, AMS: ms[d.Kind][d.A], BMS: ms[d.Kind][d.B]}
	}
	return out, nil
}

// MergeNames folds the category or project from into to: the rule files
// are rewritten (rules.toml is kept as rules.toml.bak first), then manual
// tags, away labels and suggestions are renamed. It returns the declared
// spellings.
func (a *App) MergeNames(kind, from, to string) (src, dst string, err error) {
	err = a.withRulesLock(func() error {
		src, dst, err = a.mergeNamesLocked(kind, from, to)
		return err
	})
	return src, dst, err
}

func (a *App) mergeNamesLocked(kind, from, to string) (string, string, error) {
	f := a.rules()
	cur, err := f.Read()
	if err != nil {
		return "", "", err
	}
	m, err := classify.MergeNames(cur.Rules, cur.Learned, kind, from, to)
	if err != nil {
		return "", "", err
	}
	next := cur
	next.Rules, next.Learned = m.Rules, m.Learned
	// The tags take the new name before rules.toml does: a crash leaves
	// the old rules, and merging again finishes.
	if err := f.Write(cur, next, rulesfile.Options{BeforeRules: func() error {
		return a.st.RenameLabel(kind, m.From, m.To, a.nowMS())
	}}); err != nil {
		return "", "", err
	}
	_, err = a.Reload()
	return m.From, m.To, err
}
