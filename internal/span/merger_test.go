package span

import (
	"math"
	"testing"
	"time"
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

// ---------------------------------------------------------------------------
// Hard boundaries: sleep, wake, lock, unlock (D-12)
// ---------------------------------------------------------------------------

func TestSleepClosesAtEventTS(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, feedAll(m,
		Ev(t0+7000, EventSleep),
		S(t0+9000, "Terminal", "", 0), // asleep: suppressed
		S(t0+11000, "Terminal", "", 0),
	)...)
	assertSpans(t, replay(t, ops), []Span{W(t0, t0+7000, "Terminal")})
}

func TestSleepEventOlderThanSpanClampsToStart(t *testing.T) {
	// The Sleep event is older than the open span's start: the span closes
	// at its own start (zero-length) and never ends before it starts.
	m := New(DefaultConfig())
	ops := feedAll(m,
		S(t0, "Terminal", "", 0),
		S(t0+4000, "Mail", "", 0),
		Ev(t0+3000, EventSleep),
		S(t0+6000, "Mail", "", 0), // asleep: suppressed
	)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+4000, "Terminal"),
		W(t0+4000, t0+4000, "Mail"),
	})
}

func TestSleepImplicitWakeAfter60s(t *testing.T) {
	m := New(DefaultConfig())
	sleep := t0 + 9000
	ops := polls(m, "Terminal", t0, t0+8000, t0)
	ops = append(ops, m.Feed(*Ev(sleep, EventSleep))...)
	early := m.Feed(*S(sleep+59999, "Terminal", "", 0))
	if got := kinds(early); len(got) != 0 {
		t.Fatalf("sample 59999 ms after sleep emitted %v, want none", got)
	}
	ops = append(ops, early...)
	ops = append(ops, m.Feed(*S(sleep+60000, "Terminal", "", 0))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, sleep, "Terminal"),
		WO(sleep+60000, "Terminal"),
	})
}

func TestWakeAfterMissedSleepClosesAtLastTS(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+10000, t0)
	ops = append(ops, m.Feed(*Ev(t0+60000, EventWake))...) // no Sleep was seen
	ops = append(ops, m.Feed(*S(t0+62000, "Terminal", "", 0))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+10000, "Terminal"), // never at the wake TS
		WO(t0+62000, "Terminal"),
	})
}

func TestWakeEventBeforeSpanStartIgnored(t *testing.T) {
	m := New(DefaultConfig())
	sleep := t0 + 9000
	ops := polls(m, "Terminal", t0, t0+8000, t0)
	ops = append(ops, m.Feed(*Ev(sleep, EventSleep))...)
	// First post-wake sample arrives (implicit wake) before the Wake event.
	ops = append(ops, m.Feed(*S(t0+80000, "Terminal", "", 0))...)
	late := m.Feed(*Ev(t0+79000, EventWake)) // older than the open span's start
	if got := kinds(late); len(got) != 0 {
		t.Fatalf("late Wake emitted %v, want none", got)
	}
	ops = append(ops, late...)
	ops = append(ops, m.Feed(*S(t0+82000, "Terminal", "", 0))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, sleep, "Terminal"),
		WO(t0+80000, "Terminal"),
	})
}

func TestLockClosesAndSuppresses(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+8000, t0)
	ops = append(ops, m.Feed(*Ev(t0+9000, EventLock))...)
	for ts := t0 + 10000; ts <= t0+20000; ts += 2000 {
		more := m.Feed(*SL(ts, "Terminal", true)) // polls report Locked while locked
		for _, op := range more {
			if op.Kind == OpOpen {
				t.Fatalf("Open at %d while locked", ts)
			}
		}
		ops = append(ops, more...)
	}
	assertSpans(t, replay(t, ops), []Span{W(t0, t0+9000, "Terminal")})
}

func TestLockedSampleActsAsLock(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, m.Feed(*SL(t0+8000, "Terminal", true))...)
	ops = append(ops, m.Feed(*SL(t0+10000, "Terminal", true))...)
	assertSpans(t, replay(t, ops), []Span{W(t0, t0+8000, "Terminal")})
}

func TestLockLoginWindowSampleActsAsLock(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, m.Feed(*S(t0+8000, LoginWindowBundleID, "", 0))...)
	ops = append(ops, m.Feed(*S(t0+10000, LoginWindowBundleID, "", 0))...)
	// Console shows a real app again, but the settle window (2 s after the
	// last loginwindow sample) has not passed yet.
	early := m.Feed(*S(t0+11000, "Terminal", "", 0))
	if got := kinds(early); len(got) != 0 {
		t.Fatalf("sample inside settle window emitted %v, want none", got)
	}
	ops = append(ops, early...)
	ops = append(ops, m.Feed(*S(t0+12000, "Terminal", "", 0))...)
	spans := replay(t, ops)
	assertSpans(t, spans, []Span{
		W(t0, t0+8000, "Terminal"),
		WO(t0+12000, "Terminal"),
	})
	for _, sp := range spans {
		if sp.BundleID == LoginWindowBundleID {
			t.Fatalf("loginwindow produced a span: %s", fmtSpan(sp))
		}
	}
}

func TestLockClearedByPollAfterSettle(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, m.Feed(*Ev(t0+8000, EventLock))...)
	early := m.Feed(*SL(t0+9999, "Terminal", false))
	if got := kinds(early); len(got) != 0 {
		t.Fatalf("Locked=false at lockTS+1999 emitted %v, want none", got)
	}
	ops = append(ops, early...)
	ops = append(ops, m.Feed(*SL(t0+10000, "Terminal", false))...) // lockTS+2000
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+8000, "Terminal"),
		WO(t0+10000, "Terminal"),
	})
}

func TestUnlockThenFirstSampleIgnoresIdle(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, feedAll(m,
		Ev(t0+8000, EventLock),
		Ev(t0+9000, EventUnlock),
		S(t0+11000, "Terminal", "", 400.0), // stale HID idle time after unlock
	)...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+8000, "Terminal"),
		WO(t0+11000, "Terminal"),
	})
}

// ---------------------------------------------------------------------------
// Clock jumps (D-14)
// ---------------------------------------------------------------------------

func TestJumpForwardExact120000NoJump(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+4000, t0)
	more := m.Feed(*S(t0+4000+120000, "Terminal", "", 0))
	if got := kinds(more); len(got) != 0 {
		t.Fatalf("gap of exactly 120000 ms emitted %v, want none", got)
	}
	assertSpans(t, replay(t, append(ops, more...)), []Span{WO(t0, "Terminal")})
}

func TestJumpForward120001ClosesAtLastTS(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+4000, t0)
	ops = append(ops, m.Feed(*S(t0+4000+120001, "Terminal", "", 0))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+4000, "Terminal"), // closed at the last accepted TS, not the new TS
		WO(t0+4000+120001, "Terminal"),
	})
}

func TestJumpBackwardClosesAtLastTS(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+4000, t0)
	ops = append(ops, m.Feed(*S(t0+4000-120001, "Terminal", "", 0))...)
	// The new span cannot start before the previous one ended.
	assertSpans(t, replay(t, ops), []Span{
		W(t0, t0+4000, "Terminal"),
		WO(t0+4000, "Terminal"),
	})
}

func TestJumpSmallBackwardSkewClamped(t *testing.T) {
	for _, back := range []int64{3000, 120000} {
		m := New(DefaultConfig())
		ops := polls(m, "Terminal", t0, t0+4000, t0)
		ops = append(ops, m.Feed(*S(t0+4000-back, "Mail", "", 0))...)
		assertSpans(t, replay(t, ops), []Span{
			W(t0, t0+4000, "Terminal"), // TS clamped up to lastTS
			WO(t0+4000, "Mail"),
		})
	}
}

func TestJumpResetsHeartbeatClock(t *testing.T) {
	// After a backward jump lastHB would lie in the future and silence
	// heartbeats until the clock caught up; the jump re-arms them instead.
	m := New(DefaultConfig())
	polls(m, "Terminal", t0, t0+8000, t0)
	back := t0 + 8000 - 120001
	ops := m.Feed(*S(back, "Terminal", "", 0))
	var hb []int64
	for _, op := range ops {
		if op.Kind == OpHeartbeat {
			hb = append(hb, op.TS)
		}
	}
	if len(hb) != 1 || hb[0] != back {
		t.Fatalf("heartbeats after backward jump = %v, want [%d]", hb, back)
	}
}

// ---------------------------------------------------------------------------
// Heartbeat (D-16, TRK-06)
// ---------------------------------------------------------------------------

func heartbeats(ops []Op) []int64 {
	var out []int64
	for _, op := range ops {
		if op.Kind == OpHeartbeat {
			out = append(out, op.TS)
		}
	}
	return out
}

func TestHeartbeatThreshold(t *testing.T) {
	// DefaultConfig: Heartbeat 10 s - HeartbeatLead 2.5 s = 7500 ms.
	for _, c := range []struct {
		gap  int64
		want int
	}{{7499, 0}, {7500, 1}} {
		m := New(DefaultConfig())
		first := m.Feed(*S(t0, "Terminal", "", 0))
		if hb := heartbeats(first); len(hb) != 1 || hb[0] != t0 {
			t.Fatalf("first sample heartbeats = %v, want [%d]", hb, t0)
		}
		got := heartbeats(m.Feed(*S(t0+c.gap, "Terminal", "", 0)))
		if len(got) != c.want {
			t.Fatalf("gap %d: %d heartbeats, want %d", c.gap, len(got), c.want)
		}
	}
}

func TestHeartbeatOnlyWhileOpen(t *testing.T) {
	m := New(DefaultConfig())
	// No span has ever been open: a suppressed sample carries no heartbeat.
	if hb := heartbeats(feedAll(m, Ev(t0, EventSleep), S(t0+2000, "Terminal", "", 0))); len(hb) != 0 {
		t.Fatalf("heartbeat with no open span: %v", hb)
	}

	m = New(DefaultConfig())
	feedAll(m, polls2(t0, t0+6000)...)
	m.Feed(*Ev(t0+9000, EventSleep))
	for ts := t0 + 11000; ts <= t0+60000; ts += 3000 {
		if hb := heartbeats(m.Feed(*S(ts, "Terminal", "", 0))); len(hb) != 0 {
			t.Fatalf("heartbeat at %d while asleep: %v", ts, hb)
		}
	}
	if ops := m.Tick(t0 + 80000); len(ops) != 0 {
		t.Fatalf("Tick with no open span emitted %v", ops)
	}
}

// polls2 builds 2 s Terminal samples as inputs (for feedAll).
func polls2(from, to int64) []*Input {
	var ins []*Input
	for ts := from; ts <= to; ts += 2000 {
		ins = append(ins, S(ts, "Terminal", "", 0))
	}
	return ins
}

func TestTickEmitsPendingHeartbeat(t *testing.T) {
	m := New(DefaultConfig())
	feedAll(m, polls2(t0, t0+6000)...) // heartbeat only at t0; lastTS = t0+6000
	if ops := m.Tick(t0 + 7499); len(ops) != 0 {
		t.Fatalf("Tick before threshold emitted %v", ops)
	}
	ops := m.Tick(t0 + 7500)
	if hb := heartbeats(ops); len(ops) != 1 || len(hb) != 1 || hb[0] != t0+6000 {
		t.Fatalf("Tick at threshold = %v, want one Heartbeat at lastTS %d", ops, t0+6000)
	}
	if ops := m.Tick(t0 + 20000); len(ops) != 0 {
		t.Fatalf("Tick with nothing new emitted %v", ops)
	}
}

func TestHeartbeatAlignedCadence(t *testing.T) {
	m := New(DefaultConfig())
	var hbs []int64
	for ts := t0; ts <= t0+40000; ts += 2000 {
		hbs = append(hbs, heartbeats(m.Feed(*S(ts, "Terminal", "", 0)))...)
	}
	want := []int64{t0, t0 + 8000, t0 + 16000, t0 + 24000, t0 + 32000, t0 + 40000}
	if len(hbs) != len(want) {
		t.Fatalf("heartbeats = %v, want %v", hbs, want)
	}
	for i := range want {
		if hbs[i] != want[i] {
			t.Fatalf("heartbeats = %v, want %v", hbs, want)
		}
		if i > 0 && hbs[i]-hbs[i-1] > 10000 {
			t.Fatalf("heartbeat gap %d exceeds 10000", hbs[i]-hbs[i-1])
		}
	}
}

// jitterDelays returns n poll delays in [0,500] ms from a deterministic LCG
// (seed 1), preceded by the adversarial run 1,0,0,0,0,500,0.
func jitterDelays(n int) []int64 {
	d := []int64{1, 0, 0, 0, 0, 500, 0}
	x := uint32(1)
	for len(d) < n {
		x = x*1103515245 + 12345
		d = append(d, int64((x>>16)%501))
	}
	return d
}

// worstLoss feeds one bundle on a fixed 2 s schedule where poll i is delayed
// by delays[i], and returns the worst crash loss: for every poll, its TS minus
// the latest heartbeat TS seen before it (a crash just before the poll).
func worstLoss(cfg Config, delays []int64) int64 {
	m := New(cfg)
	lastHB := int64(-1)
	var worst int64
	for i, d := range delays {
		ts := t0 + int64(i)*2000 + d
		if lastHB >= 0 && ts-lastHB > worst {
			worst = ts - lastHB
		}
		for _, hb := range heartbeats(m.Feed(*S(ts, "Terminal", "", 0))) {
			lastHB = hb
		}
	}
	return worst
}

func TestHeartbeatJitteredPollLossBound(t *testing.T) {
	delays := jitterDelays(300) // 600 s of 2 s polls
	for i, d := range delays {
		if d < 0 || d > 500 {
			t.Fatalf("delay %d = %d outside [0,500]", i, d)
		}
	}
	if got := worstLoss(DefaultConfig(), delays); got > 10000 || got < 2000 {
		t.Fatalf("worst crash loss with DefaultConfig = %d ms, want within (2000..10000]", got)
	}

	// Negative control: without the lead (threshold 8000) the adversarial run
	// 1,0,0,0,0,500 loses 10499 ms, so this test would catch a regression.
	cfg := DefaultConfig()
	cfg.HeartbeatLead = 2000 * time.Millisecond
	if got := worstLoss(cfg, delays[:7]); got != 10499 {
		t.Fatalf("negative control: worst loss = %d, want 10499 (> 10000)", got)
	}
}

func TestFlushClosesAtLastTS(t *testing.T) {
	m := New(DefaultConfig())
	ops := polls(m, "Terminal", t0, t0+6000, t0)
	ops = append(ops, m.Flush()...)
	assertSpans(t, replay(t, ops), []Span{W(t0, t0+6000, "Terminal")})
	if again := m.Flush(); len(again) != 0 {
		t.Fatalf("second Flush emitted %v", again)
	}
}
