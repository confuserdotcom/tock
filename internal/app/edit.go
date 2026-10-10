package app

import (
	"errors"
	"fmt"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

// MaxEditMS is the longest range one edit may cover.
const MaxEditMS = 24 * 60 * 60 * 1000

// EditRange is one timeline edit as the CLI and TUI show it.
type EditRange = store.Edit

// AddEdit records a timeline edit over [from, to): an entry (classify.EditAdd)
// labelled want with note, a cut (classify.EditCut), or a label over the
// range (classify.EditLabel). The range may not reach past now. It returns
// the edit's id.
func (a *App) AddEdit(kind string, from, to int64, want classify.Want, note string) (int64, error) {
	now := a.nowMS()
	switch {
	case kind != classify.EditAdd && kind != classify.EditCut && kind != classify.EditLabel:
		return 0, fmt.Errorf("unknown edit %q", kind)
	case to <= from:
		return 0, errors.New("the range ends before it starts")
	case to-from > MaxEditMS:
		return 0, errors.New("an edit covers at most 24 hours")
	case to > now:
		return 0, errors.New("the range reaches into the future")
	case kind != classify.EditCut && want.Category == "" && want.Project == "":
		return 0, errors.New("an entry or a label needs a category or a project")
	case kind == classify.EditCut && (want != classify.Want{} || note != ""):
		return 0, errors.New("a cut takes no label")
	}
	id, err := a.st.AddEdit(store.Edit{Kind: kind, StartMS: from, EndMS: to, Category: want.Category,
		Project: want.Project, Note: report.CleanPlan(note), AtMS: now})
	if err != nil {
		return 0, err
	}
	err = a.journal(editWhat(kind, from, to, want, a.now().Location()), undoData{EditsAdded: []int64{id}})
	return id, err
}

func editWhat(kind string, from, to int64, want classify.Want, loc *time.Location) string {
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("Mon 15:04") }
	r := clock(from) + "–" + time.UnixMilli(to).In(loc).Format("15:04")
	switch kind {
	case classify.EditCut:
		return "delete " + r
	case classify.EditAdd:
		return "add " + r + " " + report.LabelText(want.Category, want.Project)
	}
	return "label " + r + " " + report.LabelText(want.Category, want.Project)
}

// Edits returns the edits overlapping the calendar day of day.
func (a *App) Edits(day time.Time) ([]EditRange, error) {
	from := a.midnight(day)
	return a.st.Edits(from.UnixMilli(), from.AddDate(0, 0, 1).UnixMilli())
}

// RemoveEdit deletes an edit; the time underneath shows again.
func (a *App) RemoveEdit(id int64) error {
	e, err := a.st.EditByID(id)
	if err != nil {
		return fmt.Errorf("no edit %d", id)
	}
	if err := a.st.DeleteEdit(id); err != nil {
		return err
	}
	return a.journal("remove edit: "+editWhat(e.Kind, e.StartMS, e.EndMS, classify.Want{Category: e.Category, Project: e.Project},
		a.now().Location()), undoData{Edits: []store.Edit{e}})
}

// markEdits applies the edits over [from, to) to vs and puts their labels
// in ovs under the edit view ids.
func (a *App) markEdits(vs []classify.SpanView, ovs map[int64]classify.Override, from, to int64) ([]classify.SpanView, error) {
	es, err := a.st.Edits(from, to)
	if err != nil || len(es) == 0 {
		return vs, err
	}
	marks := make([]classify.EditMark, len(es))
	for i, e := range es {
		marks[i] = classify.EditMark{ID: e.ID, Kind: e.Kind, Note: e.Note, Interval: classify.Interval{From: e.StartMS, To: e.EndMS}}
		if e.Kind != classify.EditCut {
			ovs[-(classify.EditIDBase + e.ID)] = classify.Override{Category: e.Category, Project: e.Project}
		}
	}
	return classify.ApplyEdits(vs, marks, from, min(to, a.nowMS())), nil
}
