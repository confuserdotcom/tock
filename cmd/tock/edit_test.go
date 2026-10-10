package main

import (
	"strings"
	"testing"
)

func TestEditAndUndo(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "edit"); code != 0 || !strings.Contains(out, "no edits on 2026-10-10") {
		t.Fatalf("empty list = %d %q", code, out)
	}
	code, out := te.run(t, "", "edit", "add", "12:30-13:30", "Code/tock", "pairing", "with", "Sam")
	if code != 0 || !strings.Contains(out, "added 12:30–13:30 (1h00) → Code / tock") {
		t.Fatalf("add = %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "edit", "delete", "09:00-09:15"); code != 0 || !strings.Contains(out, "deleted 09:00–09:15") {
		t.Fatalf("delete = %d %q %s", code, out, te.errb)
	}
	code, out = te.run(t, "", "edit")
	if code != 0 || !strings.Contains(out, "add    12:30–13:30   1h00  Code / tock  pairing with Sam") ||
		!strings.Contains(out, "delete 09:00–09:15") {
		t.Fatalf("list = %q", out)
	}
	_, today := te.run(t, "", "today")
	if !strings.Contains(today, "tock 2h25") { // 1h40 - 15m cut + 1h entry
		t.Fatalf("today lacks the entry:\n%s", today)
	}
	if code, out := te.run(t, "", "undo", "--list"); code != 0 || !strings.Contains(out, "delete Sat 09:00–09:15") {
		t.Fatalf("undo list = %q", out)
	}
	if code, out := te.run(t, "", "undo"); code != 0 || out != "undid: delete Sat 09:00–09:15\n" {
		t.Fatalf("undo = %d %q", code, out)
	}
	if code, out := te.run(t, "", "edit", "remove", "1"); code != 0 || !strings.Contains(out, "removed edit 1") {
		t.Fatalf("remove = %d %q %s", code, out, te.errb)
	}
	te.run(t, "", "undo")
	te.run(t, "", "undo")
	if _, out := te.run(t, "", "undo"); out != "nothing to undo\n" {
		t.Fatalf("empty undo = %q", out)
	}
	for _, bad := range [][]string{{"edit", "add", "12:30"}, {"edit", "label", "13:00-12:00", "Code"},
		{"edit", "delete", "09:00-09:30", "Code"}, {"edit", "frob", "09:00-09:30"}} {
		if code, _ := te.run(t, "", bad...); code != 2 {
			t.Errorf("%q: exit %d, want 2", bad, code)
		}
	}
	if code, _ := te.run(t, "", "edit", "add", "14:00-16:00", "Code"); code != 1 {
		t.Error("future range accepted")
	}
}
