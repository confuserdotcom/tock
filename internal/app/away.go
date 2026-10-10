package app

import (
	"tock/internal/classify"
	"tock/internal/store"
)

// pending is the absences in views (clipped to [from, to)) that no away row
// answers or skips.
func (a *App) pending(vs []classify.SpanView, from, to int64) ([]classify.Interval, error) {
	abs := classify.Absences(vs, a.cfg.Away.MinMS, a.cfg.Away.MaxMS)
	if len(abs) == 0 {
		return nil, nil
	}
	rows, err := a.st.Aways(from, to)
	if err != nil {
		return nil, err
	}
	var out []classify.Interval
	for _, iv := range abs {
		answered := false
		for _, r := range rows {
			answered = answered || (r.StartMS < iv.To && r.EndMS > iv.From)
		}
		if !answered {
			out = append(out, iv)
		}
	}
	return out, nil
}

// Answer records what the absence iv was. An empty want skips it: it stays
// idle and tock stops asking.
func (a *App) Answer(iv classify.Interval, want classify.Want) error {
	_, err := a.st.AddAway(store.Away{StartMS: iv.From, EndMS: iv.To, Category: want.Category,
		Project: want.Project, AtMS: a.nowMS()})
	return err
}
