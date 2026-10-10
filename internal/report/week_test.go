package report

import (
	"reflect"
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/stats"
)

func TestMondayOf(t *testing.T) {
	sat := time.Date(2026, 10, 10, 15, 0, 0, 0, zone)
	if got := MondayOf(sat); !got.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, zone)) {
		t.Fatalf("MondayOf(Sat) = %v", got)
	}
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, zone)
	if got := MondayOf(mon); !got.Equal(mon) {
		t.Fatalf("MondayOf(Mon 00:00) = %v", got)
	}
	sun := time.Date(2026, 10, 11, 23, 59, 0, 0, zone)
	if got := MondayOf(sun); !got.Equal(mon) {
		t.Fatalf("MondayOf(Sun) = %v", got)
	}
}

func TestBuildWeek(t *testing.T) {
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, zone)
	ms := func(d, h, m int) int64 {
		return mon.AddDate(0, 0, d).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).UnixMilli()
	}
	vs := []classify.SpanView{
		{ID: 1, Start: ms(0, 9, 0), End: ms(0, 11, 0), BundleID: "term", App: "WezTerm"},
		{ID: 2, Start: ms(1, 23, 30), End: ms(2, 0, 30), BundleID: "term", App: "WezTerm"}, // crosses midnight
		{ID: 3, Start: ms(2, 10, 0), End: ms(2, 10, 20), BundleID: "arc", App: "Arc"},
		{ID: 4, Start: ms(2, 11, 0), End: ms(2, 12, 0), BundleID: "tock.idle", App: "Idle"},
		{ID: 5, Start: ms(5, 14, 0), End: ms(5, 15, 0), BundleID: "arc", App: "Arc"},
	}
	ls := []classify.Label{
		{Category: "Code", Kind: "deep", Project: "tock"},
		{Category: "Code", Kind: "deep", Project: "ATLAS"},
		{Category: "Entertainment", Kind: "distract"},
		{},
		{},
	}
	w := BuildWeek(WeekInput{Monday: mon, NowMS: ms(5, 15, 0), Views: vs, Labels: ls})
	if w.TrackedMS != (120+60+20+60)*60_000 {
		t.Fatalf("tracked %s", Dur(w.TrackedMS))
	}
	got := []int64{}
	for _, d := range w.Days {
		got = append(got, d.TrackedMS/60_000)
	}
	if !reflect.DeepEqual(got, []int64{120, 30, 50, 0, 0, 60, 0}) {
		t.Fatalf("per-day minutes = %v", got)
	}
	if w.Days[2].ByKind["deep"] != 30*60_000 || w.Days[2].ByKind["distract"] != 20*60_000 {
		t.Fatalf("Wed by kind = %v", w.Days[2].ByKind)
	}
	if w.Days[5].Future || !w.Days[6].Future {
		t.Fatal("only Sunday is in the future on Saturday")
	}
	if !reflect.DeepEqual(w.Projects, []NameMS{{"tock", 120 * 60_000}, {"ATLAS", 60 * 60_000}}) {
		t.Fatalf("projects = %v", w.Projects)
	}
	if w.Categories[0].Name != "Code" || w.Categories[len(w.Categories)-1].Name != Unclassified {
		t.Fatalf("categories = %+v", w.Categories)
	}
}

func TestBuildWeekScoresEachDay(t *testing.T) {
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, zone)
	ms := func(d, h, m int) int64 {
		return mon.AddDate(0, 0, d).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).UnixMilli()
	}
	deep := classify.Label{Category: "Code", Kind: "deep"}
	vs := []classify.SpanView{
		{ID: 1, Start: ms(0, 9, 0), End: ms(0, 10, 0), BundleID: "term", App: "WezTerm"},
		{ID: 2, Start: ms(1, 23, 50), End: ms(2, 0, 20), BundleID: "term", App: "WezTerm"}, // 10m Tue, 20m Wed
		{ID: 3, Start: ms(5, 9, 0), End: ms(5, 9, 30), BundleID: "term", App: "WezTerm"},
		{ID: 4, Start: ms(6, 9, 0), End: ms(6, 10, 0), BundleID: "term", App: "WezTerm"}, // future
	}
	ls := []classify.Label{deep, deep, deep, deep}
	var targets [7]stats.Target
	for i := range targets {
		targets[i] = stats.Target{MS: int64(i+1) * 3_600_000}
	}
	w := BuildWeek(WeekInput{Monday: mon, NowMS: ms(5, 9, 30), Views: vs, Labels: ls,
		Targets: targets, Weights: stats.DefaultWeights})
	var focus []int64
	for _, d := range w.Days {
		focus = append(focus, d.Stats.FocusMS/60_000)
	}
	if !reflect.DeepEqual(focus, []int64{60, 10, 20, 0, 0, 30, 0}) {
		t.Fatalf("focus minutes per day = %v", focus)
	}
	for i, d := range w.Days[:6] {
		if want := stats.ScoreDay(d.Stats, targets[i], stats.DefaultWeights); d.Score != want {
			t.Errorf("day %d score = %+v, want %+v", i, d.Score, want)
		}
	}
	if sun := w.Days[6]; sun.Score != (stats.Score{}) || sun.Stats.ActiveMS != 0 {
		t.Fatalf("future day is scored: %+v", sun)
	}
}

func TestWeekApps(t *testing.T) {
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, zone)
	at := func(m int) int64 { return mon.Add(9*time.Hour + time.Duration(m)*time.Minute).UnixMilli() }
	u := func(s string) *string { return &s }
	vs := []classify.SpanView{
		{ID: 1, Start: at(0), End: at(50), BundleID: "term", App: "WezTerm"},
		{ID: 2, Start: at(50), End: at(80), BundleID: "arc", App: "Arc", URL: u("www.youtube.com/watch")},
		{ID: 3, Start: at(80), End: at(90), BundleID: "arc", App: "Arc", URL: u("youtube.com/feed")},
		{ID: 4, Start: at(90), End: at(110), BundleID: "arc", App: "Arc", URL: u("github.com/a/b"), Private: true},
		{ID: 5, Start: at(110), End: at(115), BundleID: "arc", App: "Arc"},
		{ID: 6, Start: at(115), End: at(200), BundleID: "tock.idle", App: "Idle"},
	}
	w := BuildWeek(WeekInput{Monday: mon, NowMS: at(200), Views: vs, Labels: make([]classify.Label, len(vs))})
	want := []NameMS{{"WezTerm", 50 * 60_000}, {"youtube.com", 40 * 60_000}, {"Arc", 25 * 60_000}}
	if !reflect.DeepEqual(w.Apps, want) {
		t.Fatalf("apps = %v, want %v", w.Apps, want)
	}
}

func TestStatus(t *testing.T) {
	code := &Row{Label: classify.Label{Category: "Code", Project: "tock"}}
	cases := []struct {
		in   StatusInput
		want string
	}{
		{StatusInput{TrackedMS: 312 * 60_000, FocusMS: 160 * 60_000, Score: 64, Active: true, Open: code, TrackdUp: true},
			"5h12 · score 64 · focus 2h40 · now Code / tock"},
		{StatusInput{TrackedMS: 60_000, Active: true, TrackdUp: true}, "1m · score 0"},
		{StatusInput{TrackedMS: 60_000, Active: true, Open: &Row{Idle: true}, TrackdUp: true}, "1m · score 0 · now idle"},
		{StatusInput{TrackedMS: 60_000, Active: true, Open: &Row{}, TrackdUp: true}, "1m · score 0 · now unlabelled"},
		{StatusInput{TrackdUp: true}, "0s"},
		{StatusInput{TrackedMS: 60_000, Active: true, Open: &Row{}, TrackdUp: true, InSession: true, SessionLeftMS: 12*60_000 + 4_000},
			"1m · score 0 · now unlabelled · ◷ 12m"},
		{StatusInput{TrackedMS: 312 * 60_000, FocusMS: 5, Score: 50, Active: true, Open: code}, "trackd down · 5h12"},
	}
	for _, c := range cases {
		if got := Status(c.in); got != c.want {
			t.Errorf("Status(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
