package report

import (
	"reflect"
	"testing"
	"time"

	"tock/internal/classify"
)

func TestBuildDayTotalsAndCategories(t *testing.T) {
	vs, ls := build(t,
		sv{1, "09:00", "10:40", "term", "tock │ x", "", "Code", "deep", "tock", false},
		sv{2, "10:40", "11:14", "term", "ATLAS │ y", "", "Code", "deep", "ATLAS", false},
		sv{3, "11:14", "11:26", "arc", "Video", "www.youtube.com/watch", "Entertainment", "distract", "", false},
		sv{4, "11:26", "11:35", "arc", "Chat", "chatgpt.com/c/1", "", "", "", false},
		sv{5, "11:35", "12:16", "", "", "", "", "idle", "", false},
		sv{6, "12:16", "12:22", "arc", "Lab", "labs.x.tv/lab/1", "", "", "", false},
	)
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "15:00"), Views: vs, Labels: ls,
		Ignored: map[string]bool{"arc · labs.x.tv/lab": true}})
	if d.TrackedMS != (100+34+12+9+6)*60_000 || d.IdleMS != 41*60_000 {
		t.Fatalf("tracked %s idle %s", Dur(d.TrackedMS), Dur(d.IdleMS))
	}
	wantKinds := map[string]int64{"deep": 134 * 60_000, "distract": 12 * 60_000, "": 15 * 60_000}
	if !reflect.DeepEqual(d.ByKind, wantKinds) {
		t.Fatalf("ByKind = %v", d.ByKind)
	}
	var names []string
	for _, c := range d.Categories {
		names = append(names, c.Name)
	}
	if !reflect.DeepEqual(names, []string{"Code", "Entertainment", Unclassified}) {
		t.Fatalf("category order = %v", names)
	}
	code := d.Categories[0]
	if code.Kind != "deep" || !reflect.DeepEqual(code.Projects, []NameMS{{"tock", 100 * 60_000}, {"ATLAS", 34 * 60_000}}) {
		t.Fatalf("Code row = %+v", code)
	}
	if len(code.Items) != 2 {
		t.Fatalf("Code keys = %+v", code.Items)
	}
	un := d.Categories[2]
	if un.MS != 15*60_000 || len(un.Items) != 1 || un.Items[0].Key != "arc · chatgpt.com/c" {
		t.Fatalf("Unclassified must skip ignored keys: %+v", un)
	}
}

func TestHeatmapSplitsAtHours(t *testing.T) {
	vs, ls := build(t,
		sv{1, "09:50", "10:20", "term", "x", "", "Code", "deep", "", false},
		sv{2, "10:20", "10:30", "arc", "y", "", "", "", "", false},
		sv{3, "10:30", "11:00", "", "", "", "", "idle", "", false},
	)
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "23:00") + 3_600_000*2, Views: vs, Labels: ls})
	deep := d.Hours.MS["deep"]
	if deep[9] != 10*60_000 || deep[10] != 20*60_000 {
		t.Fatalf("deep hours = %v", deep)
	}
	if d.Hours.MS[""][10] != 10*60_000 {
		t.Fatalf("unlabelled hour 10 = %d", d.Hours.MS[""][10])
	}
	if _, ok := d.Hours.MS["idle"]; ok {
		t.Fatal("idle must not appear in the heatmap")
	}
	if d.Hours.First != 9 || d.Hours.Last != 16 {
		t.Fatalf("range %d..%d, want 9..16 (widened to %d hours)", d.Hours.First, d.Hours.Last, MinHours)
	}
}

func TestHeatmapRange(t *testing.T) {
	late, lateL := build(t, sv{1, "21:00", "23:30", "term", "x", "", "Code", "deep", "", false})
	if d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "23:59") + 86_400_000, Views: late, Labels: lateL}); d.Hours.First != 16 || d.Hours.Last != 23 {
		t.Fatalf("late day widens backward: %d..%d", d.Hours.First, d.Hours.Last)
	}
	if d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "12:00") + 86_400_000}); d.Hours.First != 8 || d.Hours.Last != 17 {
		t.Fatalf("empty past day: %d..%d", d.Hours.First, d.Hours.Last)
	}
	if d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "06:10")}); d.Hours.First != 6 || d.Hours.Last != 13 {
		t.Fatalf("empty today starts at the current hour: %d..%d", d.Hours.First, d.Hours.Last)
	}
	early, earlyL := build(t, sv{1, "07:00", "07:30", "term", "x", "", "Code", "deep", "", false})
	if d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "18:20"), Views: early, Labels: earlyL}); d.Hours.First != 7 || d.Hours.Last != 18 {
		t.Fatalf("today extends to the current hour: %d..%d", d.Hours.First, d.Hours.Last)
	}
}

func TestHeatmapDSTRepeatedHour(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 2026-10-25: clocks go back from 03:00 CEST to 02:00 CET (01:00 UTC).
	date := time.Date(2026, 10, 25, 0, 0, 0, 0, paris)
	start := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).UnixMilli() // 02:30 CEST
	end := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC).UnixMilli()   // 02:30 CET
	vs := []classify.SpanView{{ID: 1, Start: start, End: end, BundleID: "term", App: "WezTerm"}}
	d := BuildDay(DayInput{Date: date, NowMS: end + 86_400_000, Views: vs, Labels: []classify.Label{{Category: "Code", Kind: "deep"}}})
	if got := d.Hours.MS["deep"][2]; got != 60*60_000 {
		t.Fatalf("repeated hour 02 = %s, want 1h00", Dur(got))
	}
}

func TestTimelineMergesRuns(t *testing.T) {
	vs, ls := build(t,
		sv{1, "09:00", "09:10", "term", "tock │ ✳ a", "", "Code", "deep", "tock", false},
		sv{2, "09:10", "09:20", "term", "tock │ ◐ a", "", "Code", "deep", "tock", false},
		sv{3, "09:20", "09:25", "", "", "", "", "idle", "", false},
		sv{4, "09:25", "09:30", "", "", "", "", "idle", "", false},
		sv{5, "09:30", "09:40", "arc", "secret", "x.com/a", "", "", "", true},
		sv{6, "09:40", "10:00", "term", "tock │ ✳ a", "", "Code", "deep", "tock", false},
	)
	rows := BuildDay(DayInput{Date: day0, NowMS: hm(t, "10:00"), Views: vs, Labels: ls}).Timeline
	if len(rows) != 4 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	if !reflect.DeepEqual(r.IDs, []int64{1, 2}) || r.Start != hm(t, "09:00") || r.End != hm(t, "09:20") ||
		r.Text != "tock │ ◐ a" || r.Label.Project != "tock" || r.Open {
		t.Fatalf("merged run = %+v", r)
	}
	if !rows[1].Idle || !reflect.DeepEqual(rows[1].IDs, []int64{3, 4}) || rows[1].Key != "" {
		t.Fatalf("idle run = %+v", rows[1])
	}
	if rows[2].Text != "" {
		t.Fatalf("private row shows text %q", rows[2].Text)
	}
	if !rows[3].Open || rows[3].App != "term" {
		t.Fatalf("last row must be open: %+v", rows[3])
	}
}

func TestRowTextSanitisesAndFallsBackToURL(t *testing.T) {
	vs, ls := build(t,
		sv{1, "09:00", "09:10", "arc", "a\x1bb", "", "", "", "", false},
		sv{2, "09:10", "09:20", "safari", "", "x.com/a", "", "", "", false},
	)
	rows := BuildDay(DayInput{Date: day0, NowMS: hm(t, "12:00"), Views: vs, Labels: ls}).Timeline
	if rows[0].Text != "a?b" || rows[1].Text != "x.com/a" {
		t.Fatalf("texts %q %q", rows[0].Text, rows[1].Text)
	}
}
