package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	it := Parse("(A) 2026-10-16 Thermo · TD 1 +thermo @ven-07h00 due:2026-10-16 id:W42-28 see:https://x.org/a")
	want := Item{Raw: it.Raw, Pri: "A", Text: "Thermo · TD 1 see:https://x.org/a", Projects: []string{"thermo"},
		Contexts: []string{"ven-07h00"}, Due: "2026-10-16", ID: "W42-28"}
	if !reflect.DeepEqual(it, want) {
		t.Errorf("got  %+v\nwant %+v", it, want)
	}
	d := Parse("x 2026-10-16 2026-10-15 Méca +meca pri:B")
	if !d.Done || d.DoneOn != "2026-10-16" || d.Pri != "B" || d.Text != "Méca" {
		t.Errorf("done: %+v", d)
	}
}

func TestToggleRoundTrip(t *testing.T) {
	for _, l := range []string{"(A) Thermo +thermo due:2026-10-16 id:W42-28", "write it", "(C) 2026-10-01 old"} {
		done := Toggle(l, "2026-10-16")
		if !strings.HasPrefix(done, "x 2026-10-16 ") || !Parse(done).Done {
			t.Errorf("tick %q = %q", l, done)
		}
		if back := Toggle(done, "2026-10-16"); back != l {
			t.Errorf("untick %q = %q, want %q", done, back, l)
		}
	}
	if got := Toggle("x Thermo", "2026-10-16"); got != "Thermo" {
		t.Errorf("undated done: %q", got)
	}
}

func TestNew(t *testing.T) {
	if got := New(" (B) call the prof ", "2026-10-16"); got != "(B) 2026-10-16 call the prof" {
		t.Error(got)
	}
	if got := New("read ch. 2", "2026-10-16"); got != "2026-10-16 read ch. 2" {
		t.Error(got)
	}
}

func TestRows(t *testing.T) {
	lines := []string{
		"(D) late due:2026-10-15",
		"(A) b today due:2026-10-16",
		"x 2026-10-16 ticked due:2026-10-16",
		"x 2026-10-14 old history",
		"(B) tomorrow due:2026-10-17",
		"later due:2026-10-20",
		"hidden t:2026-10-18",
		"(A) a today due:2026-10-16",
		"no date",
	}
	var got []string
	for _, r := range Rows(lines, "2026-10-16", "2026-10-17") {
		got = append(got, SectionNames[r.Section]+": "+r.Text)
	}
	want := []string{"Overdue: late", "Today: b today", "Today: a today", "Tomorrow: tomorrow", "Later: later",
		"No date: no date", "Done: ticked"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestEditByText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.txt")
	if err := Write(path, []string{"one", "two", "three"}); err != nil {
		t.Fatal(err)
	}
	tick := func(l string) *string { s := Toggle(l, "2026-10-16"); return &s }
	if err := Edit(path, "two", -1, tick); err != nil {
		t.Fatal(err)
	}
	if err := Edit(path, "gone", -1, tick); err != ErrGone {
		t.Errorf("missing line: %v", err)
	}
	if err := Edit(path, "one", -1, func(string) *string { return nil }); err != nil {
		t.Fatal(err)
	}
	back := "one"
	if err := Edit(path, "", 0, func(string) *string { return &back }); err != nil {
		t.Fatal(err)
	}
	lines, _ := Read(path)
	if want := []string{"one", "x 2026-10-16 two", "three"}; !reflect.DeepEqual(lines, want) {
		t.Errorf("file = %q", lines)
	}
	if b, _ := os.ReadFile(path); !strings.HasSuffix(string(b), "three\n") {
		t.Errorf("no final newline: %q", b)
	}
	if ls, err := Read(filepath.Join(t.TempDir(), "none.txt")); err != nil || ls != nil {
		t.Errorf("missing file: %v %v", ls, err)
	}
}

func TestRowsByTime(t *testing.T) {
	lines := []string{"untimed due:2026-10-16", "b @ven-09h45 due:2026-10-16", "a @07:30 due:2026-10-16",
		"c @ven-07h00 due:2026-10-16", "later day @lun-06h00 due:2026-10-19", "sooner day @dim-23h00 due:2026-10-18"}
	var got []string
	for _, r := range Rows(lines, "2026-10-16", "2026-10-17") {
		got = append(got, r.Text)
	}
	want := []string{"c", "a", "b", "untimed", "sooner day", "later day"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
