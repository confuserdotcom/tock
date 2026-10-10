package app

import (
	"fmt"

	"tock/internal/classify"
	"tock/internal/store"
)

// Propose returns the learned-rule candidates for key over all history.
func (a *App) Propose(key string, want classify.Want) ([]classify.Candidate, error) {
	hist, _, ovs, err := a.history()
	if err != nil {
		return nil, err
	}
	return a.rs.Propose(key, want, hist, ovs)
}

// Learn appends c's rule to learned.toml and reloads the rules, so the next
// read applies it to all history.
func (a *App) Learn(key string, c classify.Candidate, want classify.Want) error {
	comment := fmt.Sprintf("learned %s from %q, %d spans", a.now().Format("2006-01-02"), key, c.ThisKey)
	if err := classify.AppendLearned(a.paths.Learned, c.Rule, comment); err != nil {
		return err
	}
	_, err := a.Reload()
	return err
}

// Tag sets a manual label on spans. An empty field of want keeps the span's
// existing override value.
// A negative id is an away view (-row id): its away row is relabelled.
func (a *App) Tag(ids []int64, want classify.Want) error {
	at := a.nowMS()
	ovs := make([]store.Override, 0, len(ids))
	for _, id := range ids {
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

// Ignore hides key from the inbox, or shows it again when on is false.
func (a *App) Ignore(key string, on bool) error {
	return a.st.SetIgnored(key, on, a.nowMS())
}
