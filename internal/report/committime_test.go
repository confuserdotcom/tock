package report

import (
	"reflect"
	"testing"
)

func TestCommitTime(t *testing.T) {
	vs, ls := build(t,
		sv{id: 1, start: "09:00", end: "10:00", app: "Code", proj: "tock", cat: "Code"},
		sv{id: 2, start: "10:00", end: "10:20", app: "idle", kind: "idle"},
		sv{id: 3, start: "10:20", end: "10:50", app: "Code", proj: "tock", cat: "Code"},
		sv{id: 4, start: "10:50", end: "11:30", app: "Code", proj: "other", cat: "Code"},
		sv{id: 5, start: "11:30", end: "12:00", app: "Code", proj: "tock", cat: "Code"},
	)
	in := []ProjectCommit{
		{Project: "tock", AtMS: hm(t, "09:30"), Hash: "a", Subject: "one"},
		{Project: "tock", AtMS: hm(t, "10:40"), Hash: "b", Subject: "two"},
		{Project: "other", AtMS: hm(t, "11:10"), Hash: "c", Subject: "three"},
	}
	got := CommitTime(in, vs, ls, hm(t, "09:00"), hm(t, "12:00"), 8*3600_000)
	want := CommitReport{
		Rows: []CommitRow{
			{ProjectCommit: in[0], WorkMS: 30 * 60_000},
			{ProjectCommit: in[1], WorkMS: 50 * 60_000}, // 09:30-10:00 + 10:20-10:40, idle skipped
			{ProjectCommit: in[2], WorkMS: 20 * 60_000},
		},
		Open: []NameMS{{"tock", 40 * 60_000}, {"other", 20 * 60_000}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestCommitTimeCapAndFirst(t *testing.T) {
	vs, ls := build(t, sv{id: 1, start: "00:00", end: "12:00", app: "Code", proj: "tock", cat: "Code"})
	in := []ProjectCommit{{Project: "tock", AtMS: hm(t, "11:00"), Hash: "a"}}
	got := CommitTime(in, vs, ls, hm(t, "00:00"), hm(t, "12:00"), 2*3600_000)
	if got.Rows[0].WorkMS != 2*3600_000 {
		t.Fatalf("cap: %d", got.Rows[0].WorkMS)
	}
	// Open time after the last commit is capped too.
	if !reflect.DeepEqual(got.Open, []NameMS{{"tock", 3600_000}}) {
		t.Fatalf("open: %+v", got.Open)
	}
	if r := CommitTime(nil, vs, ls, 0, 1, 1); r.Rows != nil || r.Open != nil {
		t.Fatalf("empty: %+v", r)
	}
}
