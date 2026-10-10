package app

import (
	"errors"
	"fmt"

	"tock/internal/classify"
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
	_, err := a.Reload()
	return err
}

// Tag sets a manual label on spans. An empty field of want keeps the span's
// existing override value.
// A negative id is an away view (-row id): its away row is relabelled. A
// meeting view id records an away row over its event, which then wins over
// the event; an empty category keeps the calendar category.
func (a *App) Tag(ids []int64, want classify.Want) error {
	at := a.nowMS()
	ovs := make([]store.Override, 0, len(ids))
	for _, id := range ids {
		if _, ok := classify.RemoteRowID(id); ok {
			return ErrRemote
		}
		if eid, ok := classify.MeetingEventID(id); ok {
			if err := a.tagMeeting(eid, want, at); err != nil {
				return err
			}
			continue
		}
		if id < 0 {
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
	if err := a.st.SetOverrides(ovs); err != nil {
		if len(ids) == 1 {
			return fmt.Errorf("no span with id %d", ids[0])
		}
		return err
	}
	return nil
}

func (a *App) tagMeeting(eventID int64, want classify.Want, at int64) error {
	e, err := a.st.Event(eventID)
	if err != nil {
		return err
	}
	if want.Category == "" {
		want.Category = a.rs.CalendarCategory
	}
	end := min(e.EndMS, at)
	if end <= e.StartMS {
		return fmt.Errorf("the meeting has not started")
	}
	_, err = a.st.AddAway(store.Away{StartMS: e.StartMS, EndMS: end, Category: want.Category, Project: want.Project, AtMS: at})
	return err
}

// Ignore hides key from the inbox, or shows it again when on is false.
func (a *App) Ignore(key string, on bool) error {
	return a.st.SetIgnored(key, on, a.nowMS())
}
