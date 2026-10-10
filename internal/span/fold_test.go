package span

import "testing"

// ---------------------------------------------------------------------------
// Fold (RESEARCH Q8 rule 7, TRK-05)
// ---------------------------------------------------------------------------

// settleStream is the title-settle tracer: A, then an app switch to B whose
// title is stale for 150 ms before the real one lands.
func settleStream() []*Input {
	old := S(t0+8000, "com.b", "old", 0)
	old.Sample.Activation = true
	return []*Input{
		S(t0, "com.a", "", 0),
		S(t0+5000, "com.a", "", 0),
		old,
		S(t0+8150, "com.b", "real", 0),
		S(t0+12000, "com.b", "real", 0),
	}
}

func TestFoldTitleSettleIntoNextSameBundle(t *testing.T) {
	m := New(DefaultConfig())
	var all []Op
	var settleOps []Op
	for i, in := range settleStream() {
		ops := m.Feed(*in)
		if i == 3 {
			settleOps = withoutHB(ops)
		}
		all = append(all, ops...)
	}
	all = append(all, m.Flush()...)

	got := replay(t, all)
	assertSpans(t, got, []Span{
		W(t0, t0+8000, "com.a"),
		WT(W(t0+8000, t0+12000, "com.b"), "real"),
	})

	// The t0+8150 Feed deletes the stale span and opens the real one at the
	// switch moment: Delete first, then Open.
	if len(settleOps) != 2 || settleOps[0].Kind != OpDelete || settleOps[1].Kind != OpOpen {
		t.Fatalf("settle feed ops = %v, want [delete open]", kinds(settleOps))
	}
	if settleOps[0].ID != 2 {
		t.Fatalf("deleted id = %d, want 2 (B-old)", settleOps[0].ID)
	}
	if settleOps[1].Span.Start != t0+8000 {
		t.Fatalf("real span start = %d, want %d", settleOps[1].Span.Start, t0+8000)
	}
	if settleOps[1].Span.ID != 3 {
		t.Fatalf("real span id = %d, want 3 (ids are never reused)", settleOps[1].Span.ID)
	}
	for _, sp := range got {
		if sp.ID == 2 {
			t.Fatalf("deleted span id 2 survived: %s", fmtSpan(sp))
		}
	}
}

// opsOf returns the ops of one Feed without heartbeats.
func opsOf(m *Merger, in *Input) []Op { return withoutHB(m.Feed(*in)) }

// wantKinds fails unless ops have exactly the given kinds, in order.
func wantKinds(t *testing.T, what string, ops []Op, want ...OpKind) {
	t.Helper()
	ok := len(ops) == len(want)
	for i := 0; ok && i < len(ops); i++ {
		ok = ops[i].Kind == want[i]
	}
	if !ok {
		t.Fatalf("%s ops = %v, want %v", what, kinds(ops), want)
	}
}

// ---------------------------------------------------------------------------
// Threshold: strictly under FoldMin folds, exactly FoldMin is kept
// ---------------------------------------------------------------------------

// runThreshold runs A (long), B for dur ms, then a long C.
func runThreshold(t *testing.T, dur int64) []Span {
	t.Helper()
	m := New(DefaultConfig())
	ops := feedAll(m,
		S(t0, "A", "", 0),
		S(t0+10000, "B", "", 0),
		S(t0+10000+dur, "C", "", 0),
		S(t0+20000, "C", "", 0),
	)
	return replay(t, append(ops, m.Flush()...))
}

func TestFoldThreshold1999Folded(t *testing.T) {
	// B lived 1999 ms: it goes to the adjacent previous span A.
	assertSpans(t, runThreshold(t, 1999), []Span{
		W(t0, t0+11999, "A"),
		W(t0+11999, t0+20000, "C"),
	})
}

func TestFoldThreshold2000Kept(t *testing.T) {
	// B lived exactly 2000 ms: kept.
	assertSpans(t, runThreshold(t, 2000), []Span{
		W(t0, t0+10000, "A"),
		W(t0+10000, t0+12000, "B"),
		W(t0+12000, t0+20000, "C"),
	})
}

// ---------------------------------------------------------------------------
// Fold into the adjacent previous span
// ---------------------------------------------------------------------------

func TestFoldIntoPreviousExtend(t *testing.T) {
	m := New(DefaultConfig())
	var all []Op
	all = append(all, feedAll(m, S(t0, "A", "", 0), S(t0+6000, "C", "", 0))...)
	ops := opsOf(m, S(t0+7000, "D", "", 0)) // C lived 1000 ms
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+12000, "D", "", 0))...)
	all = append(all, m.Flush()...)

	// Order inside the Feed: Delete and Extend before Open.
	wantKinds(t, "fold feed", ops, OpDelete, OpExtend, OpOpen)
	if ops[0].ID != 2 || ops[1].ID != 1 || ops[1].TS != t0+7000 || ops[2].Span.Start != t0+7000 {
		t.Fatalf("fold feed ops wrong: delete id %d, extend id %d to %d, open start %d",
			ops[0].ID, ops[1].ID, ops[1].TS, ops[2].Span.Start)
	}
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+7000, "A"),
		W(t0+7000, t0+12000, "D"),
	})
}

func TestFoldIntoPreviousReopenWhenNextEqualsPrevious(t *testing.T) {
	m := New(DefaultConfig())
	var all []Op
	all = append(all, feedAll(m, S(t0, "A", "", 0), S(t0+6000, "C", "", 0))...)
	ops := opsOf(m, S(t0+7000, "A", "", 0)) // back to A after 1000 ms of C
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+12000, "A", "", 0))...)
	closing := m.Flush()
	all = append(all, closing...)

	// No new span opens: A is reopened with its original id.
	wantKinds(t, "reopen feed", ops, OpDelete, OpReopen)
	if ops[0].ID != 2 || ops[1].ID != 1 {
		t.Fatalf("reopen feed targets delete %d reopen %d, want 2 and 1", ops[0].ID, ops[1].ID)
	}
	wantKinds(t, "flush", closing, OpClose)
	if closing[0].ID != 1 || closing[0].TS != t0+12000 {
		t.Fatalf("flush closes id %d at %d, want id 1 at %d", closing[0].ID, closing[0].TS, t0+12000)
	}
	assertSpans(t, replay(t, all), []Span{W(t0, t0+12000, "A")})
}

func TestFoldChainAbsorbedIntoPrevious(t *testing.T) {
	// Two quick switches in a row both end up in A.
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "", 0), S(t0+6000, "C", "", 0))
	d := opsOf(m, S(t0+7000, "D", "", 0))
	wantKinds(t, "first fold", d, OpDelete, OpExtend, OpOpen)
	all = append(all, d...)
	e := opsOf(m, S(t0+8000, "E", "", 0))
	wantKinds(t, "second fold", e, OpDelete, OpExtend, OpOpen)
	if e[0].ID != 3 || e[1].ID != 1 || e[1].TS != t0+8000 || e[2].Span.ID != 4 {
		t.Fatalf("second fold: delete %d extend %d to %d open id %d, want 3, 1, %d, 4",
			e[0].ID, e[1].ID, e[1].TS, e[2].Span.ID, t0+8000)
	}
	all = append(all, e...)
	all = append(all, feedAll(m, S(t0+14000, "E", "", 0))...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+8000, "A"),
		W(t0+8000, t0+14000, "E"),
	})
}

func TestFoldPrivateReopenKeepsNoTitleOrURL(t *testing.T) {
	m := New(DefaultConfig())
	all := feedAll(m,
		SP(t0, "A"),
		S(t0+6000, "B", "", 0),
		SP(t0+7000, "A"), // back to the same private key: reopen
		SP(t0+12000, "A"),
	)
	all = append(all, m.Flush()...)
	want := W(t0, t0+12000, "A")
	want.Private = true
	assertSpans(t, replay(t, all), []Span{want})
}

// ---------------------------------------------------------------------------
// Fold into the next span
// ---------------------------------------------------------------------------

func TestFoldIntoNextWhenNoAdjacentPrevious(t *testing.T) {
	// The first span after a lock is short and has no adjacent previous span
	// (the lock is a hard boundary): it folds into the next span.
	m := New(DefaultConfig())
	all := feedAll(m,
		S(t0, "A", "", 0),
		Ev(t0+6000, EventLock),
		Ev(t0+9000, EventUnlock),
		S(t0+9500, "B", "", 0),
	)
	ops := opsOf(m, S(t0+10500, "C", "", 0)) // B lived 1000 ms
	wantKinds(t, "fold feed", ops, OpDelete, OpOpen)
	if ops[0].ID != 2 || ops[1].Span.Start != t0+9500 {
		t.Fatalf("fold feed: delete id %d, open start %d, want 2 and %d", ops[0].ID, ops[1].Span.Start, t0+9500)
	}
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+15000, "C", "", 0))...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+6000, "A"),
		W(t0+9500, t0+15000, "C"),
	})
}

// ---------------------------------------------------------------------------
// Short span right before a hard boundary
// ---------------------------------------------------------------------------

func TestFoldBeforeHardBoundaryIntoPrevious(t *testing.T) {
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "", 0), S(t0+6000, "B", "", 0))
	ops := opsOf(m, Ev(t0+7000, EventSleep)) // B lived 1000 ms
	wantKinds(t, "sleep feed", ops, OpDelete, OpExtend)
	all = append(all, ops...)
	// Nothing folds across the boundary: the next A is a separate span.
	all = append(all, feedAll(m,
		Ev(t0+20000, EventWake),
		S(t0+21000, "A", "", 0),
		S(t0+30000, "A", "", 0),
	)...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+7000, "A"),
		W(t0+21000, t0+30000, "A"),
	})
}

func TestFoldBeforeHardBoundaryKeptWithoutPrevious(t *testing.T) {
	m := New(DefaultConfig())
	all := feedAll(m,
		S(t0, "A", "", 0),
		Ev(t0+6000, EventLock),
		Ev(t0+9000, EventUnlock),
		S(t0+9500, "B", "", 0),
	)
	ops := opsOf(m, Ev(t0+10500, EventSleep)) // B lived 1000 ms, A ended at a lock
	wantKinds(t, "sleep feed", ops, OpClose)
	all = append(all, ops...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+6000, "A"),
		W(t0+9500, t0+10500, "B"),
	})
}

func TestFoldBeforeClockJumpIntoPrevious(t *testing.T) {
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "", 0), S(t0+6000, "B", "", 0), S(t0+7000, "B", "", 0))
	// 120001 ms after the last sample: a clock jump closes B at t0+7000.
	ops := opsOf(m, S(t0+7000+120001, "C", "", 0))
	wantKinds(t, "jump feed", ops, OpDelete, OpExtend, OpOpen)
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+130000, "C", "", 0))...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+7000, "A"),
		W(t0+127001, t0+130000, "C"),
	})
}

// ---------------------------------------------------------------------------
// Idle, flush and ordering
// ---------------------------------------------------------------------------

func TestFoldNeverIdle(t *testing.T) {
	m := New(DefaultConfig())
	all := feedAll(m,
		S(t0, "A", "", 0),
		S(t0+20000, "B", "", 0),
		// Idle 300 s: the start clamps to B's start, so B is zero-length and
		// folds into A, and the idle span opens at t0+20000.
		S(t0+21000, "B", "", 300.0),
		// Back after 500 ms of idle time: the idle span lasts 1500 ms but is
		// idle, so it is kept.
		S(t0+22000, "C", "", 0.5),
	)
	// C lived 1000 ms right after the idle span: it cannot extend idle, so it
	// folds into the next span.
	ops := opsOf(m, S(t0+23000, "D", "", 0))
	wantKinds(t, "post-idle fold", ops, OpDelete, OpOpen)
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+30000, "D", "", 0))...)
	all = append(all, m.Flush()...)

	for i, op := range all {
		if (op.Kind == OpExtend || op.Kind == OpReopen || op.Kind == OpDelete) && op.ID == 3 {
			t.Fatalf("op %d (%s) touches the idle span id 3", i, op.Kind)
		}
	}
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+20000, "A"),
		W(t0+20000, t0+21500, IdleBundleID),
		W(t0+21500, t0+30000, "D"),
	})
}

func TestFoldFlushShortKeptWhenLone(t *testing.T) {
	// A lone sample followed by Flush is kept (matches
	// TestMergeSingleSampleFlushZeroLength).
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "", 0))
	ops := m.Flush()
	wantKinds(t, "flush", ops, OpClose)
	assertSpans(t, replay(t, append(all, ops...)), []Span{W(t0, t0, "A")})

	// A short span after a hard boundary with Flush: no neighbour, kept.
	m = New(DefaultConfig())
	all = feedAll(m,
		S(t0, "A", "", 0),
		Ev(t0+6000, EventLock),
		Ev(t0+9000, EventUnlock),
		S(t0+9500, "B", "", 0),
		S(t0+10000, "B", "", 0),
	)
	ops = m.Flush()
	wantKinds(t, "flush after boundary", ops, OpClose)
	assertSpans(t, replay(t, append(all, ops...)), []Span{
		W(t0, t0+6000, "A"),
		W(t0+9500, t0+10000, "B"),
	})
}

func TestFoldShortBeforeFlushFoldsIntoAdjacentPrevious(t *testing.T) {
	// Flush is a hard boundary like sleep: a short last span next to its
	// previous span goes there.
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "", 0), S(t0+6000, "B", "", 0), S(t0+7000, "B", "", 0))
	ops := m.Flush()
	wantKinds(t, "flush", ops, OpDelete, OpExtend)
	assertSpans(t, replay(t, append(all, ops...)), []Span{W(t0, t0+7000, "A")})
}

func TestFoldPreferenceOrder(t *testing.T) {
	// B has a stale title, then the real one. Both (a) same-bundle next and
	// (b) adjacent previous apply; (a) wins, so A keeps its end.
	m := New(DefaultConfig())
	all := feedAll(m, S(t0, "A", "t1", 0), S(t0+6000, "B", "x", 0))
	ops := opsOf(m, S(t0+7000, "B", "z", 0))
	wantKinds(t, "preference feed", ops, OpDelete, OpOpen)
	if ops[1].Span.Start != t0+6000 {
		t.Fatalf("open start = %d, want the short span's start %d", ops[1].Span.Start, t0+6000)
	}
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+12000, "B", "z", 0))...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		WT(W(t0, t0+6000, "A"), "t1"),
		WT(W(t0+6000, t0+12000, "B"), "z"),
	})
}

func TestFoldNeverAcrossHardBoundaryEvenWhenAdjacent(t *testing.T) {
	// Sleep and Wake carry the same TS, so B opens exactly at A's end. The
	// boundary is hard: B must not be folded back into A, so it goes to the
	// next span instead.
	m := New(DefaultConfig())
	all := feedAll(m,
		S(t0, "A", "", 0),
		Ev(t0+6000, EventSleep),
		Ev(t0+6000, EventWake),
		S(t0+6000, "B", "", 0),
	)
	ops := opsOf(m, S(t0+7000, "C", "", 0))
	wantKinds(t, "fold feed", ops, OpDelete, OpOpen)
	all = append(all, ops...)
	all = append(all, feedAll(m, S(t0+12000, "C", "", 0))...)
	all = append(all, m.Flush()...)
	assertSpans(t, replay(t, all), []Span{
		W(t0, t0+6000, "A"),
		W(t0+6000, t0+12000, "C"),
	})
}
