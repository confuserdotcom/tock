package app

import (
	"errors"
	"fmt"
	"strings"

	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// RefileSession relabels the time of a finished session to project and
// returns the session and the project's canonical name. The label is the
// project's category when it is a module, else the project alone (the
// category the rules gave stays).
//
// Only the non-idle stretches of the session window are labelled, one edit
// each, so idle, away and untracked time never turns into project time and
// the day's tracked total does not change. All the edits form one journal
// entry: a single undo reverts the refile. A running session fails with
// store.ErrSessionOpen, an unknown id with store.ErrSessionGone, a session
// with no non-idle time with "nothing to refile in that session".
func (a *App) RefileSession(id int64, project string) (s focus.Session, canonical string, err error) {
	s, err = a.SessionByID(id)
	if err != nil {
		return focus.Session{}, "", err
	}
	if s.EndMS == 0 {
		return focus.Session{}, "", store.ErrSessionOpen
	}
	p, err := a.rs.ResolveProject(project)
	if err != nil {
		if errors.Is(err, classify.ErrUnknownName) {
			return focus.Session{}, "", fmt.Errorf("no project %q (the labels tab lists them)", project)
		}
		return focus.Session{}, "", err
	}
	want := classify.Want{Category: a.rs.ProjectCategory(p), Project: p}
	now := a.nowMS()
	vs, _, err := a.labelled(s.StartMS, s.EndMS, now)
	if err != nil {
		return focus.Session{}, "", err
	}
	runs := focus.ActiveRuns(vs, s.StartMS, s.EndMS)
	if len(runs) == 0 {
		return focus.Session{}, "", errors.New("nothing to refile in that session")
	}
	note := fmt.Sprintf("refile session #%d", id)
	for _, r := range runs {
		if err := a.checkEdit(classify.EditLabel, r.From, r.To, want, note, now); err != nil {
			return focus.Session{}, "", fmt.Errorf("refile session: %w", err)
		}
	}
	edits := make([]store.Edit, len(runs))
	for i, r := range runs {
		edits[i] = store.Edit{Kind: classify.EditLabel, StartMS: r.From, EndMS: r.To,
			Category: want.Category, Project: want.Project, Note: note, AtMS: now}
	}
	what := fmt.Sprintf("refile session #%d %s", id, report.LabelText(want.Category, want.Project))
	// The edits and their undo entry are written together: either both exist
	// or neither does.
	_, err = a.st.AddEditsWithUndo(edits, func(ids []int64) (store.Undo, error) {
		return a.undoEntry(what, undoData{EditsAdded: ids})
	})
	if err != nil {
		return focus.Session{}, "", fmt.Errorf("refile session: %w", err)
	}
	return s, p, nil
}

// refilePrefix starts the journal text of a refile of session id; the
// trailing space keeps #4 from matching #42.
func refilePrefix(id int64) string { return fmt.Sprintf("refile session #%d ", id) }

// RefileUndoable reports whether the newest undo entry is the refile of
// session id, that is whether Undo would revert exactly that refile.
func (a *App) RefileUndoable(id int64) (bool, error) {
	es, err := a.UndoList(1)
	if err != nil {
		return false, err
	}
	return len(es) == 1 && strings.HasPrefix(es[0].What, refilePrefix(id)), nil
}

// UndoRefile reverts the refile of session id, but only while it is still
// the newest change; otherwise it changes nothing and says so.
func (a *App) UndoRefile(id int64) (string, error) {
	ok, err := a.RefileUndoable(id)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("the latest change is not that refile")
	}
	return a.Undo()
}
