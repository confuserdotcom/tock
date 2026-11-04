package classify

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"tock/internal/span"
)

// The skip-ahead in fill and yieldToRemote must change nothing: compare
// them with the plain loops on random timelines, sorted marks or not.
func TestFillMatchesPlainLoop(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 3000; n++ {
		views := randViews(r)
		var marks []Interval
		for k := r.Intn(6); k > 0; k-- {
			a := r.Int63n(120)
			marks = append(marks, Interval{a, a + 1 + r.Int63n(40)})
		}
		if r.Intn(2) == 0 {
			sort.Slice(marks, func(i, j int) bool { return marks[i].From < marks[j].From })
		}
		mk := func(i int, s, e int64) SpanView {
			return SpanView{ID: -int64(i + 1), Start: s, End: e, BundleID: "mark"}
		}
		got := fill(cloneViews(views), marks, 0, 150, mk)
		want := fillNaive(cloneViews(views), marks, 0, 150, mk)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fill differs\nviews %+v\nmarks %+v\ngot  %+v\nwant %+v", views, marks, got, want)
		}
		var rm []RemoteMark
		for k := r.Intn(6); k > 0; k-- {
			a := r.Int63n(120)
			rm = append(rm, RemoteMark{ID: int64(k), View: SpanView{Start: a, End: a + 1 + r.Int63n(40), BundleID: "r"}})
		}
		if r.Intn(2) == 0 {
			sort.Slice(rm, func(i, j int) bool { return rm[i].View.Start < rm[j].View.Start })
		}
		if g, w := yieldToRemote(cloneViews(views), rm), yieldNaive(cloneViews(views), rm); !reflect.DeepEqual(g, w) {
			t.Fatalf("yieldToRemote differs\nviews %+v\nmarks %+v\ngot  %+v\nwant %+v", views, rm, g, w)
		}
	}
}

func randViews(r *rand.Rand) []SpanView {
	var vs []SpanView
	at := int64(r.Intn(10))
	for id := int64(1); at < 140; id++ {
		end := at + 1 + r.Int63n(30)
		b := "app"
		if r.Intn(3) == 0 {
			b = span.IdleBundleID
		}
		vs = append(vs, SpanView{ID: id, Start: at, End: end, BundleID: b, InCall: r.Intn(5) == 0})
		at = end + r.Int63n(8)
	}
	return vs
}

func cloneViews(vs []SpanView) []SpanView { return append([]SpanView{}, vs...) }

func fillNaive(views []SpanView, marks []Interval, from, to int64, mk func(i int, start, end int64) SpanView) []SpanView {
	if len(marks) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(marks))
	for _, v := range views {
		if v.BundleID != span.IdleBundleID {
			out = append(out, v)
			continue
		}
		at := v.Start
		for _, m := range marks {
			if m.To <= at || m.From >= v.End {
				continue
			}
			if m.From > at {
				p := v
				p.Start, p.End = at, m.From
				out = append(out, p)
			}
			at = min(m.To, v.End) // the piece inside the mark is filled below
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	covered := append([]SpanView{}, out...)
	sort.SliceStable(covered, func(i, j int) bool { return covered[i].Start < covered[j].Start })
	for i, m := range marks {
		lo, hi := max(m.From, from), min(m.To, to)
		at := lo
		for _, v := range covered {
			if v.End <= at || v.Start >= hi {
				continue
			}
			if v.Start > at {
				out = append(out, mk(i, at, v.Start))
			}
			at = max(at, v.End)
		}
		if at < hi {
			out = append(out, mk(i, at, hi))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func yieldNaive(views []SpanView, marks []RemoteMark) []SpanView {
	out := make([]SpanView, 0, len(views))
	for _, v := range views {
		if v.ID <= 0 || v.InCall || v.BundleID == span.IdleBundleID {
			out = append(out, v)
			continue
		}
		at := v.Start
		for _, m := range marks { // in start order
			lo, hi := max(m.View.Start, at), min(m.View.End, v.End)
			if m.View.Start <= v.Start+SwitchGraceMS || hi <= lo {
				continue
			}
			if lo > at {
				p := v
				p.Start, p.End = at, lo
				out = append(out, p)
			}
			p := v
			p.Start, p.End, p.BundleID, p.App = lo, hi, span.IdleBundleID, span.IdleApp
			p.Title, p.URL, p.Private, p.Dir, p.Kept = nil, nil, false, nil, nil
			out = append(out, p)
			at = hi
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}
