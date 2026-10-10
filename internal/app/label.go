package app

import (
	"errors"
	"fmt"
	"os"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

// ErrRemote means a label was asked for another machine's time.
var ErrRemote = errors.New("this time was recorded on another machine: label it there")

// Propose returns the learned-rule candidates for key over all history.
func (a *App) Propose(key string, want classify.Want) ([]classify.Candidate, error) {
	hist, _, ovs, err := a.history()
	if err != nil {
		return nil, err
	}
	return a.rs.Propose(key, want, hist, ovs)
}

// Learn appends c's rule to learned.toml, removes the learned rules it
// replaces, and reloads the rules, so the next read applies it to all
// history.
func (a *App) Learn(key string, c classify.Candidate, want classify.Want) error {
	before, existed, err := readFile(a.paths.Learned)
	if err != nil {
		return err
	}
	comment := fmt.Sprintf("learned %s from %q, %d spans", a.now().Format("2006-01-02"), key, c.ThisKey)
	if err := classify.AppendLearned(a.paths.Learned, c.Rule, comment); err != nil {
		return err
	}
	// The new rule went in first, so the replaced ones moved down by one.
	shifted := make([]int, len(c.Replaces))
	for i, n := range c.Replaces {
		shifted[i] = n + 1
	}
	if err := classify.RemoveLearned(a.paths.Learned, shifted); err != nil {
		return err
	}
	if _, err := a.Reload(); err != nil {
		return err
	}
	after, _, err := readFile(a.paths.Learned)
	if err != nil {
		return err
	}
	return a.journal("learn "+c.Rule.Describe()+" → "+report.LabelText(want.Category, want.Project),
		undoData{File: &undoFile{Path: a.paths.Learned, Existed: existed, Before: before, After: after}})
}

// readFile returns path's content and whether it exists.
func readFile(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	return string(b), err == nil, err
}

// Tag sets a manual label on spans. An empty field of want keeps the span's
// existing override value.
// A negative id is an away view (-row id): its away row is relabelled. A
// meeting view id records an away row over its event, which then wins over
// the event; an empty category keeps the calendar category.
func (a *App) Tag(ids []int64, want classify.Want) error {
	at := a.nowMS()
	ovs := make([]store.Override, 0, len(ids))
	var undo undoData
	// The journal is written even when a later id fails, so what changed can
	// still be undone.
	defer func() {
		_ = a.journal(fmt.Sprintf("tag %s → %s", plural(len(ids), "span"), report.LabelText(want.Category, want.Project)), undo)
	}()
	for _, id := range ids {
		if _, ok := classify.RemoteRowID(id); ok {
			return ErrRemote
		}
		if eid, ok := classify.EditRowID(id); ok {
			e, err := a.st.EditByID(eid)
			if err != nil {
				return fmt.Errorf("no edit %d", eid)
			}
			undo.Edits = append(undo.Edits, e)
			if want.Category != "" {
				e.Category = want.Category
			}
			if want.Project != "" {
				e.Project = want.Project
			}
			e.AtMS = at
			if err := a.st.PutEdit(e); err != nil {
				return err
			}
			continue
		}
		if eid, ok := classify.MeetingEventID(id); ok {
			aid, err := a.tagMeeting(eid, want, at)
			if err != nil {
				return err
			}
			undo.AwaysAdded = append(undo.AwaysAdded, aid)
			continue
		}
		if id < 0 {
			if prev, err := a.st.AwayByID(-id); err == nil {
				undo.Aways = append(undo.Aways, prev)
			}
			if err := a.st.SetAwayLabel(-id, want.Category, want.Project, at); err != nil {
				return err
			}
			continue
		}
		ovs = append(ovs, store.Override{SpanID: id, Category: want.Category, Project: want.Project, At: at})
	}
	if len(ovs) == 0 {
		return nil
	}
	spanIDs := make([]int64, len(ovs))
	for i, o := range ovs {
		spanIDs[i] = o.SpanID
	}
	prev, err := a.st.OverridesFor(spanIDs)
	if err != nil {
		return err
	}
	if err := a.st.SetOverrides(ovs); err != nil {
		if len(ids) == 1 {
			return fmt.Errorf("no span with id %d", ids[0])
		}
		return err
	}
	undo.SpanIDs, undo.Overrides = spanIDs, prev
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (a *App) tagMeeting(eventID int64, want classify.Want, at int64) (int64, error) {
	e, err := a.st.Event(eventID)
	if err != nil {
		return 0, err
	}
	if want.Category == "" {
		want.Category = a.rs.CalendarCategory
	}
	end := min(e.EndMS, at)
	if end <= e.StartMS {
		return 0, fmt.Errorf("the meeting has not started")
	}
	return a.st.AddAway(store.Away{StartMS: e.StartMS, EndMS: end, Category: want.Category, Project: want.Project, AtMS: at})
}

// Ignore hides key from the inbox, or shows it again when on is false.
func (a *App) Ignore(key string, on bool) error {
	ig, err := a.st.Ignored()
	if err != nil {
		return err
	}
	if err := a.st.SetIgnored(key, on, a.nowMS()); err != nil {
		return err
	}
	what := "ignore "
	if !on {
		what = "show again "
	}
	return a.journal(what+report.Safe(key), undoData{Ignore: &undoIgnore{Key: key, Was: ig[key]}})
}
