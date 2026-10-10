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
