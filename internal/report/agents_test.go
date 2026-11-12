package report

import (
	"reflect"
	"testing"
)

func TestAgentLedger(t *testing.T) {
	vs, ls := build(t,
		sv{id: 1, start: "10:00", end: "10:10", app: "Terminal", title: "tock — ✳ Fix the inbox — node"},
		sv{id: 2, start: "10:10", end: "10:40", app: "Arc", title: "YouTube"},
		sv{id: 3, start: "10:40", end: "10:45", app: "Terminal", title: "tock — ◐ Fix the inbox"},
		sv{id: 4, start: "10:45", end: "10:50", app: "idle", kind: "idle"},
		sv{id: 5, start: "11:00", end: "11:05", app: "Terminal", title: "other — ✳ Notes"},
	)
	min := func(s string) int64 { return hm(t, s) }
	in := []AgentSession{
		{ID: "a", Project: "tock", Titles: []string{"Fix the inbox"},
			// 10:00-10:02 busy, gap 8m = waiting, 10:10-10:30 busy (events every 2m)
			Events:  append([]int64{min("10:00"), min("10:01"), min("10:02")}, every(min("10:10"), min("10:30"), 2*60_000)...),
			Prompts: []int64{min("10:00"), min("10:10")}},
		{ID: "b", Project: "tock", Titles: []string{"Other"},
			Events: every(min("10:20"), min("10:40"), 60_000), Prompts: []int64{min("10:20")}},
		{ID: "c", Project: "~/notes", Titles: []string{"Notes"}, Events: []int64{min("11:00"), min("11:01")}, Prompts: []int64{min("11:00")}},
	}
	got := AgentLedger(in, vs, ls, min("09:00"), min("12:00"), 3*60_000)
	want := Agents{
		Rows: []AgentRow{
			{Project: "tock", AgentMS: 42 * 60_000, YouMS: 15 * 60_000, Prompts: 3, Sessions: 2},
			{Project: "~/notes", AgentMS: 60_000, YouMS: 5 * 60_000, Prompts: 1, Sessions: 1},
		},
		AgentMS: 43 * 60_000, WallMS: 33 * 60_000, YouMS: 20 * 60_000, Peak: 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if got := AgentLedger(nil, vs, ls, 0, 1, 1); got.Rows != nil || got.Peak != 0 {
		t.Fatalf("empty: %+v", got)
	}
}

func every(from, to, step int64) []int64 {
	var out []int64
	for t := from; t <= to; t += step {
		out = append(out, t)
	}
	return out
}
