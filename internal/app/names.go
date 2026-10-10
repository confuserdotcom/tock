package app

import (
	"errors"
	"os"

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
func (a *App) MergeNames(kind, from, to string) (string, string, error) {
	rules, err := os.ReadFile(a.paths.Rules)
	if err != nil {
		return "", "", err
	}
	learned, err := os.ReadFile(a.paths.Learned)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	m, err := classify.MergeNames(rules, learned, kind, from, to)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(a.paths.Rules+".bak", rules, 0o600); err != nil {
		return "", "", err
	}
	// learned.toml first: it then names only declared names in both states.
	if len(learned) > 0 {
		if err := classify.WriteFile(a.paths.Learned, m.Learned); err != nil {
			return "", "", err
		}
	}
	if err := classify.WriteFile(a.paths.Rules, m.Rules); err != nil {
		return "", "", err
	}
	if err := a.st.RenameLabel(kind, m.From, m.To, a.nowMS()); err != nil {
		return "", "", err
	}
	_, err = a.Reload()
	return m.From, m.To, err
}
