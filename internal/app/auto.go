package app

import (
	"tock/internal/classify"
	"tock/internal/store"
)

// AutoAccept labels the inbox keys of the last days days whose guess is
// confident ([inbox] auto_accept): for each it learns the first safe rule
// Propose offers, as tock tag would, and records it. It returns what it
// labelled. Each rule is journaled, so tock undo reverts it.
func (a *App) AutoAccept(days int) ([]store.AutoLabel, error) {
	share := a.cfg.Inbox.AutoShare
	if share <= 0 {
		return nil, nil
	}
	items, err := a.Inbox(days)
	if err != nil {
		return nil, err
	}
	guesses, err := a.Guesses()
	if err != nil {
		return nil, err
	}
	done, err := a.st.AutoLabelled()
	if err != nil {
		return nil, err
	}
	var out []store.AutoLabel
	for _, p := range classify.AutoPicks(items, guesses, done, share) {
		cands, err := a.Propose(p.Key, p.Want)
		if err != nil {
			return out, err
		}
		for _, c := range cands {
			if !c.Safe {
				continue
			}
			if err := a.Learn(p.Key, c, p.Want); err != nil {
				return out, err
			}
			x := store.AutoLabel{Key: p.Key, Category: p.Category, Project: p.Project, Reason: p.Reason,
				Share: p.Share, Rule: c.Rule.Describe(), MS: p.TotalMS, AtMS: a.nowMS()}
			if err := a.st.AddAutoLabel(x); err != nil {
				return out, err
			}
			out = append(out, x)
			break
		}
	}
	return out, nil
}

// AutoEveryMS is how often AutoAcceptEvery runs AutoAccept.
const AutoEveryMS = 10 * 60_000

// AutoAcceptEvery is AutoAccept for long-lived callers (TUI, web): it runs
// at most once every AutoEveryMS per App.
func (a *App) AutoAcceptEvery(days int) error {
	now := a.nowMS()
	if a.autoAt != 0 && now-a.autoAt < AutoEveryMS {
		return nil
	}
	a.autoAt = now
	_, err := a.AutoAccept(days)
	return err
}

// AutoLabels returns the keys auto-labelled at or after sinceMS, newest
// first.
func (a *App) AutoLabels(sinceMS int64) ([]store.AutoLabel, error) { return a.st.AutoLabels(sinceMS) }
