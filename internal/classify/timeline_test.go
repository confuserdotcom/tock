package classify

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"tock/internal/span"
)

// render shows a timeline with pointers read, adjacent equal pieces merged,
// so two timelines that differ only in how they were cut compare equal.
func render(vs []SpanView) string {
	str := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	var parts []string
	var last string
	var lastEnd int64
	for _, v := range vs {
		k := fmt.Sprintf("%d %s %s t=%s u=%s p=%v d=%s call=%v kept=%v plan=%s scr=%v",
			v.ID, v.BundleID, v.App, str(v.Title), str(v.URL), v.Private, str(v.Dir), v.InCall, v.Kept != nil, v.Planned, v.Screen)
		if n := len(parts); n > 0 && k == last && v.Start == lastEnd {
			parts[n-1] = parts[n-1][:strings.Index(parts[n-1], "-")+1] + fmt.Sprint(v.End) + " " + k
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d %s", v.Start, v.End, k))
		}
		last, lastEnd = k, v.End
	}
	return strings.Join(parts, "\n")
}

// passes is the timeline as the six passes in order built it before
// Resolve: the oracle Resolve must match where their rules agree.
func passes(views []SpanView, src Sources, from, to int64) []SpanView {
	hi := min(to, src.NowMS)
	vs := MarkCalls(views, src.Calls)
	vs = MarkRemote(vs, src.Remote, from, hi)
	vs = MarkAway(vs, src.Away, from, to)
	vs = MarkPlan(vs, src.Plan)
	if src.Events != nil {
		vs = MarkEvents(vs, src.Events, from, hi)
	}
	return ApplyEdits(vs, src.Edits, from, hi)
}

// randIntervals is up to n intervals in [0, 150), in start order; disjoint
// unless overlap.
func randIntervals(r *rand.Rand, n int, overlap bool) []Interval {
	var out []Interval
	at := int64(r.Intn(20))
	for k := r.Intn(n + 1); k > 0 && at < 150; k-- {
		iv := Interval{at, at + 1 + r.Int63n(40)}
		out = append(out, iv)
		if overlap && r.Intn(2) == 0 {
			at += r.Int63n(30)
		} else {
			at = iv.To + r.Int63n(30)
		}
	}
	return out
}

func randSources(r *rand.Rand, overlap bool) Sources {
	src := Sources{NowMS: 200}
	if r.Intn(4) == 0 {
		src.NowMS = 100 + r.Int63n(60)
	}
	src.Calls = randIntervals(r, 3, false)
	for i, iv := range randIntervals(r, 5, overlap) {
		title := fmt.Sprint("r", i)
		src.Remote = append(src.Remote, RemoteMark{ID: int64(i + 1), Machine: "ryuk",
			View: SpanView{Start: iv.From, End: iv.To, BundleID: "rb", App: "R", Title: &title}})
	}
	for i, iv := range randIntervals(r, 3, overlap) {
		src.Away = append(src.Away, AwayMark{ID: int64(i + 1), Interval: iv})
	}
	for i, iv := range randIntervals(r, 3, false) {
		src.Plan = append(src.Plan, PlanSlot{Interval: iv, Project: fmt.Sprint("P", i)})
	}
	if r.Intn(4) > 0 {
		src.Events = []EventMark{}
		for i, iv := range randIntervals(r, 3, true) {
			src.Events = append(src.Events, EventMark{ID: int64(i + 1), Title: fmt.Sprint("e", i), Interval: iv})
		}
	}
	kinds := []string{EditAdd, EditCut, EditLabel}
	for i, iv := range randIntervals(r, 2, true) {
		src.Edits = append(src.Edits, EditMark{ID: int64(i + 1), Kind: kinds[r.Intn(3)], Interval: iv})
	}
	return src
}

func baseViews(r *rand.Rand) []SpanView {
	vs := randViews(r)
	for i := range vs {
		vs[i].InCall = false
		if r.Intn(4) == 0 {
			t := fmt.Sprint("t", i)
			vs[i].Title = &t
		}
	}
	return vs
}

// Resolve is the six passes in one: on timelines where their rules are
// defined (claims of one kind do not overlap, except events), it builds
// the same timeline.
func TestResolveMatchesPasses(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for n := 0; n < 5000; n++ {
		views := baseViews(r)
		src := randSources(r, false)
		from, to := int64(r.Intn(15)), 130+int64(r.Intn(30))
		if src.NowMS < to {
			to = src.NowMS // the passes drop idle time inside a claim past now
		}
		got := render(Resolve(cloneViews(views), src, from, to))
		want := render(passes(clipViews(cloneViews(views), from, to), src, from, to))
		if got != want {
			t.Fatalf("case %d differs\nviews %+v\nsources %+v\nfrom %d to %d\ngot\n%s\nwant\n%s", n, views, src, from, to, got, want)
		}
	}
}

func clipViews(vs []SpanView, from, to int64) []SpanView {
	out := vs[:0:0]
	for _, v := range vs {
		v.Start, v.End = max(v.Start, from), min(v.End, to)
		if v.End > v.Start {
			out = append(out, v)
		}
	}
	return out
}

// TestResolveProperties checks the invariants in Resolve's doc on random
// timelines with overlapping claims of every kind.
func TestResolveProperties(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for n := 0; n < 3000; n++ {
		views := baseViews(r)
		src := randSources(r, true)
		from, to := int64(r.Intn(15)), 130+int64(r.Intn(30))
		got := Resolve(cloneViews(views), src, from, to)
		for i, v := range got {
			if v.End <= v.Start || v.Start < from || v.End > to {
				t.Fatalf("case %d: piece %+v outside [%d, %d) or empty", n, v, from, to)
			}
			if i > 0 && got[i-1].End > v.Start {
				t.Fatalf("case %d: overlap\n%s", n, render(got))
			}
		}
		// Shuffled sources give the same timeline.
		sh := src
		sh.Remote = shuffled(r, src.Remote)
		sh.Away = shuffled(r, src.Away)
		sh.Events = shuffled(r, src.Events)
		sh.Plan = shuffled(r, src.Plan)
		sh.Calls = shuffled(r, src.Calls)
		sh.Edits = shuffled(r, src.Edits)
		if a, b := render(got), render(Resolve(cloneViews(views), sh, from, to)); a != b {
			t.Fatalf("case %d: order matters\n%s\nvs\n%s", n, a, b)
		}
		if len(src.Edits) > 0 {
			continue
		}
		// Without edits, time is conserved: the pieces cover exactly the
		// views and the claims that can fill (before now).
		hi := min(to, src.NowMS)
		var want []Interval
		for _, v := range views {
			want = append(want, Interval{max(v.Start, from), min(v.End, to)})
		}
		for _, m := range src.Remote {
			want = append(want, Interval{max(m.View.Start, from), min(m.View.End, hi)})
		}
		for _, m := range src.Away {
			want = append(want, Interval{max(m.From, from), min(m.To, hi)})
		}
		for _, e := range src.Events {
			want = append(want, Interval{max(e.From, from), min(e.To, hi)})
		}
		var have []Interval
		for _, v := range got {
			have = append(have, Interval{v.Start, v.End})
		}
		if a, b := union(want), union(have); fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatalf("case %d: covers %v, want %v\n%s", n, b, a, render(got))
		}
	}
}

func shuffled[T any](r *rand.Rand, xs []T) []T {
	if xs == nil {
		return nil
	}
	out := append([]T{}, xs...)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func union(ivs []Interval) []Interval {
	var in []Interval
	for _, iv := range ivs {
		if iv.To > iv.From {
			in = append(in, iv)
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].From < in[j].From })
	var out []Interval
	for _, iv := range in {
		if n := len(out); n > 0 && iv.From <= out[n-1].To {
			out[n-1].To = max(out[n-1].To, iv.To)
			continue
		}
		out = append(out, iv)
	}
	return out
}

// TestResolveTable pins each row of the table in Resolve's doc.
func TestResolveTable(t *testing.T) {
	idle := func(id, s, e int64) SpanView {
		return SpanView{ID: id, Start: s, End: e, BundleID: span.IdleBundleID, App: span.IdleApp}
	}
	act := func(id, s, e int64) SpanView { return SpanView{ID: id, Start: s, End: e, BundleID: "a", App: "A"} }
	rm := func(id, s, e int64) RemoteMark {
		return RemoteMark{ID: id, Machine: "ryuk", View: SpanView{Start: s, End: e, BundleID: "r", App: "R"}}
	}
	owner := func(v SpanView) string {
		switch {
		case IsRemote(v):
			return "remote"
		case IsAway(v):
			return "away"
		case IsMeeting(v):
			return "meeting"
		case v.BundleID == span.CallBundleID:
			return "call"
		case IsEntry(v):
			return "entry"
		case v.BundleID == span.IdleBundleID:
			return "idle"
		}
		return "activity"
	}
	show := func(vs []SpanView) string {
		var s []string
		for _, v := range vs {
			x := fmt.Sprintf("%s %d-%d", owner(v), v.Start, v.End)
			if v.InCall {
				x += " call"
			}
			if v.Planned != "" {
				x += " plan=" + v.Planned
			}
			s = append(s, x)
		}
		return strings.Join(s, ", ")
	}
	sg := int64(SwitchGraceMS)
	cases := []struct {
		name  string
		views []SpanView
		src   Sources
		want  string
	}{
		{"in a call, activity keeps its view", []SpanView{act(1, 0, 100)},
			Sources{Calls: []Interval{{0, 100}}, Remote: []RemoteMark{rm(1, sg + 10, 100)}},
			"activity 0-100 call"},
		{"in a call, idle time is the call", []SpanView{idle(1, 0, 100)},
			Sources{Calls: []Interval{{0, 100}}, Remote: []RemoteMark{rm(1, 0, 100)}, Away: []AwayMark{{1, Interval{0, 100}}}},
			"call 0-100 call"},
		{"in a call, an event explains idle time", []SpanView{idle(1, 0, 100)},
			Sources{Calls: []Interval{{0, 100}}, Events: []EventMark{{ID: 1, Interval: Interval{0, 50}}}},
			"meeting 0-50, call 50-100 call"},
		{"a later switch takes activity", []SpanView{act(1, 0, 2 * sg)},
			Sources{Remote: []RemoteMark{rm(1, sg + 1, 2 * sg)}},
			fmt.Sprintf("activity 0-%d, remote %d-%d", sg+1, sg+1, 2*sg)},
		{"within the grace, activity stays", []SpanView{act(1, 0, 2 * sg)},
			Sources{Remote: []RemoteMark{rm(1, sg, 2 * sg)}},
			fmt.Sprintf("activity 0-%d", 2*sg)},
		{"idle: remote, then away, then event", []SpanView{idle(1, 0, 100)},
			Sources{Remote: []RemoteMark{rm(1, 0, 20)}, Away: []AwayMark{{1, Interval{0, 40}}}, Events: []EventMark{{ID: 1, Interval: Interval{0, 60}}}},
			"remote 0-20, away 20-40, meeting 40-60, idle 60-100"},
		{"gaps take claims too, a call does not reach them", nil,
			Sources{Calls: []Interval{{0, 100}}, Away: []AwayMark{{1, Interval{10, 20}}}},
			"away 10-20"},
		{"plan is context, not for meetings", []SpanView{act(1, 0, 20), idle(2, 20, 40)},
			Sources{Plan: []PlanSlot{{Interval{10, 40}, "M"}}, Events: []EventMark{{ID: 1, Interval: Interval{30, 40}}}},
			"activity 0-10, activity 10-20 plan=M, idle 20-30 plan=M, meeting 30-40"},
		{"edits win, the later first", []SpanView{act(1, 0, 100)},
			Sources{Calls: []Interval{{0, 100}}, Edits: []EditMark{{ID: 2, Kind: EditCut, Interval: Interval{0, 50}}, {ID: 1, Kind: EditAdd, Interval: Interval{0, 60}}}},
			"entry 50-60, activity 60-100 call"},
		{"claims stop at now", []SpanView{idle(1, 0, 100)},
			Sources{Away: []AwayMark{{1, Interval{0, 100}}}, NowMS: 30},
			"away 0-30, idle 30-100"},
		{"overlapping absences count once", []SpanView{idle(1, 0, 100)},
			Sources{Away: []AwayMark{{2, Interval{20, 60}}, {1, Interval{0, 40}}}},
			"away 0-40, away 40-60, idle 60-100"},
	}
	for _, c := range cases {
		if got := show(Resolve(c.views, c.src, 0, 3*sg)); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, c.want)
		}
	}
}
