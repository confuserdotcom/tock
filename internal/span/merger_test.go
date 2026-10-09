package span

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// Idle (D-11)
// ---------------------------------------------------------------------------

func TestIdleBackdatesAndOpensIdleSpan(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000 // last activity
	ops := polls(m, "Terminal", t0, a+310000, a)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Terminal"),
		WO(a, IdleBundleID),
	})
}

func TestIdleThresholdExact(t *testing.T) {
	a := t0 + 10000

	// IdleS 300.0 (idleMS exactly 300000) triggers idle.
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, a+300000, a)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Terminal"),
		WO(a, IdleBundleID),
	})

	// IdleS 299.999 (idleMS 299999) emits no idle ops.
	m = New(DefaultConfig())
	ops = polls(m, "Terminal", t0, a+298000, a)
	more := m.Feed(*S(a+299999, "Terminal", "", 299.999))
	if got := kinds(more); len(got) != 0 {
		t.Fatalf("IdleS 299.999 emitted ops %v", got)
	}
	assertSpans(t, replay(t, append(ops, more...)), []Span{WO(t0, "Terminal")})
}

func TestIdleFloorPrecision(t *testing.T) {
	a := t0 + 10000

	// 300.0009 s floors to 300000 ms, so the boundary is TS-300000 = a.
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, a+298000, a)
	ops = append(ops, m.Feed(*S(a+300000, "Terminal", "", 300.0009))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Terminal"),
		WO(a, IdleBundleID),
	})

	for name, v := range map[string]float64{"NaN": math.NaN(), "-1": -1, "+Inf": math.Inf(1), "-Inf": math.Inf(-1)} {
		m := New(DefaultConfig())
		ops := polls(m, "Terminal", t0, t0+4000, t0)
		more := m.Feed(*S(t0+6000, "Terminal", "", v))
		if got := kinds(more); len(got) != 0 {
			t.Fatalf("IdleS %s emitted ops %v, want none (counts as 0)", name, got)
		}
		assertSpans(t, replay(t, append(ops, more...)), []Span{WO(t0, "Terminal")})
	}
}

func TestIdleWhileIdleKeepsSpan(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	ops := polls(m, "Terminal", t0, a+300000, a)
	for ts := a + 302000; ts <= a+320000; ts += 2000 {
		more := m.Feed(*S(ts, "Terminal", "", float64(ts-a)/1000))
		if got := kinds(more); len(got) != 0 {
			t.Fatalf("idle sample at %d emitted ops %v", ts, got)
		}
		ops = append(ops, more...)
	}
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Terminal"),
		WO(a, IdleBundleID),
	})
}

func TestIdleEndsOnActivity(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	T := a + 400000
	ops := polls(m, "Terminal", t0, T-2000, a)
	ops = append(ops, m.Feed(*S(T, "Mail", "", 1.5))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Terminal"),
		W(a, T-1500, IdleBundleID),
		WO(T-1500, "Mail"),
	})
}

func TestIdleEndsOnActivityClampedToIdleStart(t *testing.T) {
	// The activity time TS-10000 predates the idle span's start, so the idle
	// span closes at its own start (zero-length) and the new span begins there.
	m := New(DefaultConfig())
	ops := feedAll(m,
		S(t0, "Terminal", "", 0),
		S(t0+2000, "Terminal", "", 400.0), // idle opens at t0 (clamped)
		S(t0+4000, "Mail", "", 10.0),
	)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0, "Terminal"),
		W(t0, t0, IdleBundleID),
		WO(t0, "Mail"),
	})
}

func TestIdleClampedToSpanStart(t *testing.T) {
	// The idle backdate (TS-400000) is earlier than the open span's start, so
	// the boundary clamps to the span start and Terminal is zero-length.
	m := New(DefaultConfig())
	ops := feedAll(m,
		S(t0, "Terminal", "", 0),
		S(t0+2000, "Terminal", "", 400.0),
	)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0, "Terminal"),
		WO(t0, IdleBundleID),
	})
}

func TestIdleSpanShape(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	ops := polls(m, "Terminal", t0, a+300000, a)
	spans := replay(t, ops)
	idle := spans[len(spans)-1]
	if idle.BundleID != IdleBundleID || idle.App != IdleApp {
		t.Fatalf("idle span bundle/app = %q/%q", idle.BundleID, idle.App)
	}
	if idle.Title != nil || idle.URL != nil || idle.Private {
		t.Fatalf("idle span must have nil title/url and Private false: %s", fmtSpan(idle))
	}
}
