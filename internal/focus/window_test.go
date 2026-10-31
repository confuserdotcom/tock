package focus

import (
	"reflect"
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/span"
)

// wseg is one view of a window fixture: minutes after 10:00 on day0, an app,
// a category, a project and a kind.
type wseg struct {
	from, to int64
	app, cat string
	proj     string
	kind     string
}

func build(ss ...wseg) ([]classify.SpanView, []classify.Label) {
	views := make([]classify.SpanView, len(ss))
	labels := make([]classify.Label, len(ss))
	for i, s := range ss {
		views[i] = classify.SpanView{ID: int64(i + 1), Start: at(10, 0, 0) + s.from*minute, End: at(10, 0, 0) + s.to*minute,
			BundleID: s.app, App: s.app}
		labels[i] = classify.Label{Category: s.cat, Project: s.proj, Kind: s.kind}
	}
	return views, labels
}

// win is the window [from, to) in minutes after 10:00 as unix ms.
func win(from, to int64) (int64, int64) { return at(10, 0, 0) + from*minute, at(10, 0, 0) + to*minute }

func TestWindowTopNames(t *testing.T) {
	views, labels := build(
		wseg{0, 24, "a1", "A", "", ""},
		wseg{24, 36, "a2", "C", "", ""},
		wseg{36, 48, "a3", "B", "", ""},
		wseg{48, 54, "a4", "D", "", ""},
	)
	// 40 s of E, and a categorised view with no app name.
	views = append(views,
		classify.SpanView{ID: 6, Start: at(10, 54, 0), End: at(10, 54, 40), BundleID: "e", App: "e"},
		classify.SpanView{ID: 7, Start: at(10, 55, 0), End: at(10, 59, 0), BundleID: "x"})
	labels = append(labels, classify.Label{Category: "E"}, classify.Label{Category: "F"})
	from, to := win(0, 60)
	b := Window(views, labels, from, to, config.DefaultFocus())
	wantCat := []Share{{"A", 24 * minute}, {"B", 12 * minute}, {"C", 12 * minute}}
	if !reflect.DeepEqual(b.Categories, wantCat) {
		t.Fatalf("Categories = %+v, want %+v", b.Categories, wantCat)
	}
	wantApp := []Share{{"a1", 24 * minute}, {"a2", 12 * minute}, {"a3", 12 * minute}}
	if !reflect.DeepEqual(b.Apps, wantApp) {
		t.Fatalf("Apps = %+v, want %+v", b.Apps, wantApp)
	}
}

func TestWindowClipsToWindow(t *testing.T) {
	views, labels := build(
		wseg{0, 10, "before", "Before", "", ""},
		wseg{5, 20, "in", "In", "", ""},
		wseg{30, 40, "after", "After", "", ""},
	)
	from, to := win(10, 30)
	b := Window(views, labels, from, to, config.DefaultFocus())
	want := []Share{{"In", 10 * minute}}
	if !reflect.DeepEqual(b.Categories, want) || b.ActiveMS != 10*minute {
		t.Fatalf("Window = %+v, want categories %+v and 10 min active", b, want)
	}
}

func TestWindowIdleIgnored(t *testing.T) {
	views, labels := build(
		wseg{0, 10, span.IdleBundleID, "Idle", "P", classify.KindDeep},
		wseg{10, 20, span.AwayBundleID, "Away", "P", classify.KindDeep},
	)
	from, to := win(0, 20)
	b := Window(views, labels, from, to, config.DefaultFocus())
	if !reflect.DeepEqual(b, Breakdown{}) {
		t.Fatalf("Window = %+v, want the zero Breakdown", b)
	}
}

func TestWindowProjectAndRefile(t *testing.T) {
	from, to := win(0, 60)
	cfg := config.DefaultFocus()
	cases := []struct {
		name   string
		segs   []wseg
		proj   string
		refile bool
	}{
		{"mixed projects", []wseg{{0, 20, "a", "A", "P", classify.KindDeep}, {20, 30, "a", "A", "Q", classify.KindDeep}}, "P", true},
		{"only P", []wseg{{0, 20, "a", "A", "P", classify.KindDeep}, {20, 30, "a", "A", "P", classify.KindShallow}}, "P", false},
		{"unlabelled rest", []wseg{{0, 20, "a", "A", "P", classify.KindDeep}, {20, 25, "a", "A", "", classify.KindShallow}}, "P", true},
		{"no project", []wseg{{0, 20, "a", "A", "", classify.KindDeep}}, "", false},
		{"tie by name", []wseg{{0, 10, "a", "A", "Q", classify.KindDeep}, {10, 20, "a", "A", "P", classify.KindDeep}}, "P", true},
		{"shallow project only", []wseg{{0, 20, "a", "A", "P", classify.KindShallow}}, "", false},
	}
	for _, c := range cases {
		views, labels := build(c.segs...)
		b := Window(views, labels, from, to, cfg)
		if b.Project != c.proj || b.Refile() != c.refile {
			t.Errorf("%s: Project = %q Refile = %v, want %q %v (%+v)", c.name, b.Project, b.Refile(), c.proj, c.refile, b)
		}
	}
	views, labels := build(wseg{0, 20, "a", "A", "P", classify.KindDeep}, wseg{20, 30, "a", "A", "Q", classify.KindDeep})
	if b := Window(views, labels, from, to, cfg); b.Project != "P" || b.ProjectMS != 20*minute || b.ActiveMS != 30*minute {
		t.Fatalf("Window = %+v", b)
	}
}

func TestActiveRuns(t *testing.T) {
	views, _ := build(
		wseg{0, 10, "a", "", "", ""},
		wseg{10, 20, "b", "", "", ""},
		wseg{20, 25, span.IdleBundleID, "", "", ""},
		wseg{25, 30, span.AwayBundleID, "", "", ""},
		wseg{30, 40, "c", "", "", ""},
		wseg{45, 50, "d", "", "", ""},
	)
	from, to := win(5, 48)
	iv := func(a, b int64) classify.Interval { x, y := win(a, b); return classify.Interval{From: x, To: y} }
	want := []classify.Interval{iv(5, 20), iv(30, 40), iv(45, 48)}
	if got := ActiveRuns(views, from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveRuns = %+v, want %+v", got, want)
	}
	idle, _ := build(wseg{0, 10, span.IdleBundleID, "", "", ""})
	f, e := win(0, 10)
	if got := ActiveRuns(idle, f, e); got != nil {
		t.Fatalf("ActiveRuns of idle = %+v, want nil", got)
	}
}
