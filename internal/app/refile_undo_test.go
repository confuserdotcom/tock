package app

import (
	"errors"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/store"
)

// UndoRefile reverts the entry it checked, not whatever is newest later: an
// entry pushed between the check and the revert is left alone.
func TestUndoRefileChecksTheEntryItReverts(t *testing.T) {
	_, a, x := refileEnv(t)
	if _, _, err := a.RefileSession(x.ID, "ATLAS"); err != nil {
		t.Fatal(err)
	}
	var seen string
	what, err := a.undo(func(u store.Undo) error {
		seen = u.What
		// another tock pushes a change after the check read the newest entry
		return a.journal("tag 1 span Code", undoData{EditsAdded: []int64{999}})
	})
	if err != nil || !strings.HasPrefix(what, "refile session #1 ") || what != seen {
		t.Fatalf("undo = %q, %v (checked %q)", what, err, seen)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 0 {
		t.Fatalf("the refile edits were not reverted: %+v", es)
	}
	if us, _ := a.UndoList(5); len(us) != 1 || us[0].What != "tag 1 span Code" {
		t.Fatalf("the newer entry was touched: %+v", us)
	}
	// and a refusing check changes nothing
	if _, _, err := a.RefileSession(x.ID, "ATLAS"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.undo(func(store.Undo) error { return errors.New("no") }); err == nil {
		t.Fatal("refused check still undid")
	}
	if es, _ := a.Edits(apptest.Now); len(es) == 0 {
		t.Fatal("a refused undo reverted the refile")
	}
}
