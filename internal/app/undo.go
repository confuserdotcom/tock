package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"tock/internal/rulesfile"

	"tock/internal/store"
)

// ErrNothingToUndo means the undo journal is empty.
var ErrNothingToUndo = errors.New("nothing to undo")

// undoData is the state before one change, enough to put it back.
type undoData struct {
	SpanIDs    []int64          `json:"span_ids,omitempty"`  // spans whose overrides changed
	Overrides  []store.Override `json:"overrides,omitempty"` // their overrides before
	Aways      []store.Away     `json:"aways,omitempty"`     // away rows before a relabel
	AwaysAdded []int64          `json:"aways_added,omitempty"`
	Edits      []store.Edit     `json:"edits,omitempty"` // edit rows before a change or removal
	EditsAdded []int64          `json:"edits_added,omitempty"`
	Ignore     *undoIgnore      `json:"ignore,omitempty"`
	File       *undoFile        `json:"file,omitempty"`
}

type undoIgnore struct {
	Key string `json:"key"`
	Was bool   `json:"was"`
}

// undoFile is a rule file before and after a change; undo refuses when the
// file has changed since.
type undoFile struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Before  string `json:"before"`
	After   string `json:"after"`
}

func (d undoData) empty() bool {
	return len(d.SpanIDs) == 0 && len(d.Aways) == 0 && len(d.AwaysAdded) == 0 && len(d.Edits) == 0 &&
		len(d.EditsAdded) == 0 && d.Ignore == nil && d.File == nil
}

// journal records d under what. A failed write loses the undo, never the
// change itself, so it is returned for the caller to report.
func (a *App) journal(what string, d undoData) error {
	if d.empty() {
		return nil
	}
	u, err := a.undoEntry(what, d)
	if err != nil {
		return err
	}
	return a.st.PushUndo(u)
}

// undoEntry is the journal entry for d, not yet pushed.
func (a *App) undoEntry(what string, d undoData) (store.Undo, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return store.Undo{}, err
	}
	return store.Undo{AtMS: a.nowMS(), What: what, Data: string(b)}, nil
}

// UndoEntry is one change tock undo can reverse.
type UndoEntry struct {
	AtMS int64
	What string
}

// UndoList returns up to n changes, newest first.
func (a *App) UndoList(n int) ([]UndoEntry, error) {
	us, err := a.st.Undos(n)
	if err != nil {
		return nil, err
	}
	out := make([]UndoEntry, len(us))
	for i, u := range us {
		out[i] = UndoEntry{AtMS: u.AtMS, What: u.What}
	}
	return out, nil
}

// Undo reverses the newest change in the journal and returns what it was.
func (a *App) Undo() (string, error) { return a.undo(nil) }

// undo is Undo with a check: accept sees the newest entry and may refuse it,
// in which case nothing changes. The entry accept saw is the one reverted
// and dropped, even if another change is journaled meanwhile, so a caller that
// only undoes a certain kind of change cannot revert a different one.
func (a *App) undo(accept func(store.Undo) error) (string, error) {
	us, err := a.st.Undos(1)
	if err != nil {
		return "", err
	}
	if len(us) == 0 {
		return "", ErrNothingToUndo
	}
	u := us[0]
	if accept != nil {
		if err := accept(u); err != nil {
			return "", err
		}
	}
	var d undoData
	if err := json.Unmarshal([]byte(u.Data), &d); err != nil {
		return "", fmt.Errorf("undo journal: %w", err)
	}
	if f := d.File; f != nil {
		err := a.withRulesLock(func() error {
			err := a.rules().Restore(f.Path, f.Existed, []byte(f.Before), []byte(f.After))
			if errors.Is(err, rulesfile.ErrChanged) {
				return fmt.Errorf("cannot undo %q: %s changed since", u.What, f.Path)
			}
			return err
		})
		if err != nil {
			return "", err
		}
		if _, err := a.Reload(); err != nil {
			return "", err
		}
	}
	if len(d.SpanIDs) > 0 {
		if err := a.st.RestoreOverrides(d.SpanIDs, d.Overrides); err != nil {
			return "", err
		}
	}
	for _, aw := range d.Aways {
		if err := a.st.PutAway(aw); err != nil {
			return "", err
		}
	}
	for _, id := range d.AwaysAdded {
		if err := a.st.DeleteAway(id); err != nil {
			return "", err
		}
	}
	for _, id := range d.EditsAdded {
		if err := a.st.DeleteEdit(id); err != nil && !errors.Is(err, store.ErrNoEdit) {
			return "", err
		}
	}
	for _, e := range d.Edits {
		if err := a.st.PutEdit(e); errors.Is(err, store.ErrNoEdit) {
			_, err = a.st.RestoreEdit(e)
			if err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
	}
	if ig := d.Ignore; ig != nil {
		if err := a.st.SetIgnored(ig.Key, ig.Was, a.nowMS()); err != nil {
			return "", err
		}
	}
	return u.What, a.st.DropUndo(u.ID)
}
