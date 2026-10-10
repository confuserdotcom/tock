package main

import (
	"errors"
	"math"
	"os"

	"tock/internal/classify"
	"tock/internal/store"
)

var errNoDB = errors.New("no data yet: is trackd running? (make install)")

// world is what most commands need: validated rules and the open database.
type world struct {
	rs    *classify.Ruleset
	st    *store.Store
	nowMS int64
}

func openWorld(env Env) (*world, error) {
	rs, err := classify.Load(env.rulesPath(), env.learnedPath())
	if err != nil {
		return nil, err
	}
	st, err := openStore(env)
	if err != nil {
		return nil, err
	}
	return &world{rs: rs, st: st, nowMS: env.Now().UnixMilli()}, nil
}

// openStore opens tock.db but never creates it: an absent database means
// trackd has not run yet.
func openStore(env Env) (*store.Store, error) {
	if _, err := os.Stat(env.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, errNoDB
	}
	return store.Open(env.dbPath())
}

func (w *world) close() { w.st.Close() }

// views converts rows to classify views clipped to [from, to); the open span
// ends at nowMS. Spans with nothing left after clipping are dropped.
func views(rows []store.SpanRow, from, to, nowMS int64) []classify.SpanView {
	out := make([]classify.SpanView, 0, len(rows))
	for _, r := range rows {
		end := nowMS
		if r.End != nil {
			end = *r.End
		}
		start := max(r.Start, from)
		end = min(end, to)
		if end <= start {
			continue
		}
		out = append(out, classify.SpanView{
			ID: r.ID, Start: start, End: end,
			BundleID: r.BundleID, App: r.App,
			Title: r.Title, URL: r.URL, Private: r.Private,
		})
	}
	return out
}

func overrides(m map[int64]store.Override) map[int64]classify.Override {
	out := make(map[int64]classify.Override, len(m))
	for id, o := range m {
		out[id] = classify.Override{Category: o.Category, Project: o.Project}
	}
	return out
}

// labelled loads the spans overlapping [from, to) with their labels.
func (w *world) labelled(from, to int64) ([]classify.SpanView, []classify.Label, error) {
	rows, err := w.st.Spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	ovs, err := w.st.Overrides()
	if err != nil {
		return nil, nil, err
	}
	vs := views(rows, from, to, w.nowMS)
	return vs, w.rs.LabelAll(vs, overrides(ovs)), nil
}

// history is labelled over all time.
func (w *world) history() ([]classify.SpanView, []classify.Label, map[int64]classify.Override, error) {
	rows, err := w.st.AllSpans()
	if err != nil {
		return nil, nil, nil, err
	}
	ovs, err := w.st.Overrides()
	if err != nil {
		return nil, nil, nil, err
	}
	o := overrides(ovs)
	vs := views(rows, math.MinInt64, math.MaxInt64, w.nowMS)
	return vs, w.rs.LabelAll(vs, o), o, nil
}
