package span

import (
	"sort"
	"strconv"
	"testing"
)

// t0 is the base timestamp of table tests (unix ms). It is >= 1000000 because
// lastHB starts at 0 and the first sample with a span open must heartbeat.
const t0 = int64(1_000_000)

// S builds a Sample input. An empty title gives a nil Title.
func S(ts int64, bundle, title string, idle float64) *Input {
	s := &Sample{TS: ts, BundleID: bundle, App: bundle, IdleS: idle}
	if title != "" {
		tt := title
		s.Title = &tt
	}
	return &Input{Sample: s}
}

// SU builds a sample with a URL.
func SU(ts int64, bundle, title, url string) *Input {
	in := S(ts, bundle, title, 0)
	u := url
	in.Sample.URL = &u
	return in
}

// SP builds a Private sample. Title and URL are set on purpose so tests prove
// the merger drops them.
func SP(ts int64, bundle string) *Input {
	in := S(ts, bundle, "secret title", 0)
	u := "secret.example/path"
	in.Sample.URL = &u
	in.Sample.Private = true
	return in
}

// SL builds a sample with Locked set.
func SL(ts int64, bundle string, locked bool) *Input {
	in := S(ts, bundle, "", 0)
	in.Sample.Locked = locked
	return in
}

// Ev builds an Event input.
func Ev(ts int64, k EventKind) *Input {
	return &Input{Event: &Event{TS: ts, Kind: k}}
}

// feedAll feeds each input and concatenates the resulting ops.
func feedAll(m *Merger, ins ...*Input) []Op {
	var ops []Op
	for _, in := range ins {
		ops = append(ops, m.Feed(*in)...)
	}
	return ops
}

// polls feeds 2 s polls of one bundle from `from` through `to` inclusive.
// IdleS is the time since `activity` (zero up to and including it).
func polls(m *Merger, bundle string, from, to, activity int64) []Op {
	var ops []Op
	for ts := from; ts <= to; ts += 2000 {
		idle := 0.0
		if ts > activity {
			idle = float64(ts-activity) / 1000.0
		}
		ops = append(ops, m.Feed(*S(ts, bundle, "", idle))...)
	}
	return ops
}

// withoutHB drops Heartbeat ops.
func withoutHB(ops []Op) []Op {
	var out []Op
	for _, op := range ops {
		if op.Kind != OpHeartbeat {
			out = append(out, op)
		}
	}
	return out
}

// kinds returns the op kinds of ops, Heartbeats excluded.
func kinds(ops []Op) []OpKind {
	var ks []OpKind
	for _, op := range withoutHB(ops) {
		ks = append(ks, op.Kind)
	}
	return ks
}

// replay is a pure oracle that applies ops to a span list. Open appends;
// Close and Extend set End; Delete removes; Reopen sets End to nil; Heartbeat
// is ignored. It fails the test when an op references an unknown or deleted
// id, when an Open arrives while another span is open, or when an end lands
// before its start. Spans are returned sorted by Start.
func replay(t testing.TB, ops []Op) []Span {
	t.Helper()
	byID := map[int64]*Span{}
	open := map[int64]bool{}
	for i, op := range ops {
		switch op.Kind {
		case OpOpen:
			if _, dup := byID[op.Span.ID]; dup {
				t.Fatalf("op %d: Open reuses id %d", i, op.Span.ID)
			}
			if len(open) > 0 {
				t.Fatalf("op %d: Open id %d while another span is open", i, op.Span.ID)
			}
			sp := op.Span
			sp.End = nil
			byID[sp.ID] = &sp
			open[sp.ID] = true
		case OpClose, OpExtend:
			sp, ok := byID[op.ID]
			if !ok {
				t.Fatalf("op %d: %s of unknown or deleted id %d", i, op.Kind, op.ID)
			}
			if op.TS < sp.Start {
				t.Fatalf("op %d: %s id %d ends at %d before its start %d", i, op.Kind, op.ID, op.TS, sp.Start)
			}
			end := op.TS
			sp.End = &end
			delete(open, op.ID)
		case OpDelete:
			if _, ok := byID[op.ID]; !ok {
				t.Fatalf("op %d: Delete of unknown or deleted id %d", i, op.ID)
			}
			delete(byID, op.ID)
			delete(open, op.ID)
		case OpReopen:
			sp, ok := byID[op.ID]
			if !ok {
				t.Fatalf("op %d: Reopen of unknown or deleted id %d", i, op.ID)
			}
			if len(open) > 0 && !open[op.ID] {
				t.Fatalf("op %d: Reopen id %d while another span is open", i, op.ID)
			}
			sp.End = nil
			open[op.ID] = true
		case OpHeartbeat:
		default:
			t.Fatalf("op %d: unknown op kind %d", i, op.Kind)
		}
	}
	out := make([]Span, 0, len(byID))
	for _, sp := range byID {
		out = append(out, *sp)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Start != out[b].Start {
			return out[a].Start < out[b].Start
		}
		return out[a].ID < out[b].ID
	})
	return out
}

// W builds an expected closed span.
func W(start, end int64, bundle string) Span {
	e := end
	return Span{Start: start, End: &e, BundleID: bundle}
}

// WO builds an expected open span.
func WO(start int64, bundle string) Span {
	return Span{Start: start, BundleID: bundle}
}

// WT adds an expected title to a span.
func WT(sp Span, title string) Span {
	sp.Title = &title
	return sp
}

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqI64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fmtSpan(sp Span) string {
	end := "open"
	if sp.End != nil {
		end = strconv.FormatInt(*sp.End, 10)
	}
	title := "<nil>"
	if sp.Title != nil {
		title = *sp.Title
	}
	url := "<nil>"
	if sp.URL != nil {
		url = *sp.URL
	}
	priv := ""
	if sp.Private {
		priv = " private"
	}
	return "[" + strconv.FormatInt(sp.Start, 10) + "," + end + "] " + sp.BundleID + " title=" + title + " url=" + url + priv
}

// assertSpans compares the ID-independent fields of got and want.
func assertSpans(t testing.TB, got, want []Span) {
	t.Helper()
	ok := len(got) == len(want)
	if ok {
		for i := range got {
			g, w := got[i], want[i]
			if g.Start != w.Start || !eqI64Ptr(g.End, w.End) || g.BundleID != w.BundleID ||
				!eqStrPtr(g.Title, w.Title) || !eqStrPtr(g.URL, w.URL) || g.Private != w.Private {
				ok = false
				break
			}
		}
	}
	if ok {
		return
	}
	t.Errorf("spans differ")
	for _, g := range got {
		t.Logf("  got:  %s", fmtSpan(g))
	}
	for _, w := range want {
		t.Logf("  want: %s", fmtSpan(w))
	}
	t.FailNow()
}
