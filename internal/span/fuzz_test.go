package span

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
)

// This file fuzzes the merger's global invariants (D-27, TRK-05).
//
// Three independent pieces check one random stream:
//
//   - fzDecode turns bytes into a deterministic Input stream.
//   - fzLedger applies every op strictly: an op that references an unknown or
//     deleted id, reuses an id, opens while another span is open, shrinks a
//     span with Extend or reopens an open span is an error.
//   - fzOracle re-derives the hard boundaries, away intervals, forward clock
//     jumps and idle triggers from the input stream alone, using the rules of
//     RESEARCH Q8 (clamp and jump, sleep, lock, self-heal, settle). It never
//     reads merger state. The only thing it reads from the merger side is the
//     start of the currently open span, taken from the op stream, because the
//     "stale Wake/Unlock" rule is defined relative to it.
//
// fzCheck then asserts the invariants on the final replayed spans.

// Rule constants, repeated here on purpose so the oracle does not depend on
// the merger's own constants.
const (
	fzIdleThr   = 300_000 // D-11
	fzJumpMax   = 120_000 // D-14
	fzSleepHeal = 60_000  // RESEARCH Q8 rule 3
	fzSettle    = 2_000   // lock settle
	fzFoldMin   = 2_000   // D-13
	fzBase      = int64(1_000_000)
	fzStepBytes = 5
)

var (
	// Bundle index = byte % 8. Index 7 is the lock screen.
	fzBundles = [8]string{"com.a", "com.b", "com.c", "com.d", "com.a", "com.b", "com.c", LoginWindowBundleID}
	// Title index = byte % 4. Index 0 is a nil title.
	fzTitles = [4]string{"", "t1", "t2", "t3"}
	// IdleS options, selected by byte % 16 (mostly 0).
	fzIdle = [16]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1.5, 299.999, 300, 400, 11000}
	// Time deltas in ms, selected by byte % 32 (mostly small, some jumps).
	fzDelta = [32]int64{
		150, 150, 150, 500, 500, 1000, 1000, 1999, 1999, 1999, 2000, 2000, 2000, 0, 0, 3000,
		10000, 10000, 10000, 30000, 30000, 120000, 120001, 600000, -5000, -5000, -200000, 500, 150, 2000, 1999, 10000,
	}
)

// ---------------------------------------------------------------------------
// Decoder
// ---------------------------------------------------------------------------

type fzStep struct {
	in   Input
	tick bool
	now  int64
}

// fzDecode reads five bytes per step: kind, bundle (and activation), title
// (and private/locked flags), idle, delta. A trailing partial step is dropped.
func fzDecode(data []byte) []fzStep {
	clock := fzBase
	var steps []fzStep
	for ; len(data) >= fzStepBytes; data = data[fzStepBytes:] {
		b := data[:fzStepBytes]
		clock += fzDelta[int(b[4])%len(fzDelta)]
		if clock < 1000 {
			clock = 1000
		}
		typ := int(b[0]) % 20
		switch {
		case typ < 14:
			bundle := fzBundles[int(b[1])%len(fzBundles)]
			s := &Sample{
				TS: clock, BundleID: bundle, App: bundle, IdleS: fzIdle[int(b[3])%len(fzIdle)],
				Activation: b[1]&8 != 0, AXTrusted: true, Locked: b[2]>>5 == 0, Video: b[1]&16 != 0,
			}
			ti := int(b[2]) % 4
			if t := fzTitles[ti]; t != "" {
				tt := t
				s.Title = &tt
			}
			if ti == 3 {
				u := "host/path"
				s.URL = &u
			}
			if (b[2]>>2)&7 == 0 {
				// Private samples carry a title and URL on purpose so the
				// invariant proves the merger drops them.
				s.Private = true
				tt, u := "secret", "secret.example/p"
				s.Title, s.URL = &tt, &u
			}
			steps = append(steps, fzStep{in: Input{Sample: s}})
		case typ >= 14 && typ <= 17:
			kind := [4]EventKind{EventSleep, EventWake, EventLock, EventUnlock}[typ-14]
			steps = append(steps, fzStep{in: Input{Event: &Event{TS: clock, Kind: kind}}})
		default:
			steps = append(steps, fzStep{tick: true, now: clock})
		}
	}
	return steps
}

func fzTrace(steps []fzStep) string {
	var sb strings.Builder
	for i, st := range steps {
		switch {
		case st.tick:
			fmt.Fprintf(&sb, "  %3d tick now=%d\n", i, st.now)
		case st.in.Event != nil:
			fmt.Fprintf(&sb, "  %3d event %s ts=%d\n", i, st.in.Event.Kind, st.in.Event.TS)
		default:
			s := st.in.Sample
			title := "-"
			if s.Title != nil {
				title = *s.Title
			}
			fmt.Fprintf(&sb, "  %3d sample ts=%d %s title=%s idle=%v locked=%v private=%v\n",
				i, s.TS, s.BundleID, title, s.IdleS, s.Locked, s.Private)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Ledger: strict op application
// ---------------------------------------------------------------------------

type fzLedger struct {
	spans  map[int64]*Span
	maxID  int64
	openID int64 // 0 when no span is open
}

func newLedger() *fzLedger { return &fzLedger{spans: map[int64]*Span{}} }

// openStart returns the start of the open span, if any.
func (l *fzLedger) openStart() (int64, bool) {
	if l.openID == 0 {
		return 0, false
	}
	return l.spans[l.openID].Start, true
}

func (l *fzLedger) apply(op Op) error {
	switch op.Kind {
	case OpOpen:
		sp := op.Span
		if sp.ID <= l.maxID {
			return fmt.Errorf("open id %d does not exceed the highest id so far (%d): ids must never be reused", sp.ID, l.maxID)
		}
		if l.openID != 0 {
			return fmt.Errorf("open id %d while span %d is still open", sp.ID, l.openID)
		}
		if sp.End != nil {
			return fmt.Errorf("open id %d carries an end", sp.ID)
		}
		l.maxID, l.openID = sp.ID, sp.ID
		l.spans[sp.ID] = &sp
	case OpClose:
		sp, ok := l.spans[op.ID]
		if !ok || op.ID != l.openID {
			return fmt.Errorf("close of id %d which is not the open span (%d)", op.ID, l.openID)
		}
		if op.TS < sp.Start {
			return fmt.Errorf("close id %d at %d before its start %d", op.ID, op.TS, sp.Start)
		}
		end := op.TS
		sp.End, l.openID = &end, 0
	case OpExtend:
		sp, ok := l.spans[op.ID]
		if !ok {
			return fmt.Errorf("extend of unknown or deleted id %d", op.ID)
		}
		if sp.End == nil {
			return fmt.Errorf("extend of open id %d", op.ID)
		}
		if op.TS < *sp.End {
			return fmt.Errorf("extend id %d to %d shrinks its end %d", op.ID, op.TS, *sp.End)
		}
		end := op.TS
		sp.End = &end
	case OpReopen:
		sp, ok := l.spans[op.ID]
		if !ok {
			return fmt.Errorf("reopen of unknown or deleted id %d", op.ID)
		}
		if sp.End == nil {
			return fmt.Errorf("reopen of already open id %d", op.ID)
		}
		if l.openID != 0 {
			return fmt.Errorf("reopen id %d while span %d is open", op.ID, l.openID)
		}
		sp.End, l.openID = nil, op.ID
	case OpDelete:
		if _, ok := l.spans[op.ID]; !ok {
			return fmt.Errorf("delete of unknown or deleted id %d", op.ID)
		}
		if op.ID == l.openID {
			l.openID = 0
		}
		delete(l.spans, op.ID)
	case OpHeartbeat:
	default:
		return fmt.Errorf("unknown op kind %d", op.Kind)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Oracle: boundaries, away intervals, jumps, idle triggers
// ---------------------------------------------------------------------------

// fzInterval is a time range whose interior must stay empty. After a backward
// clock jump the same millisecond values are used again by later spans, so an
// interval only constrains spans opened in the same era (the era counter
// increases at every backward jump).
type fzInterval struct {
	from, to int64
	era      int
}

type fzOracle struct {
	haveLast        bool
	last            int64
	asleep, locked  bool
	sleepTS, lockTS int64
	ignoreIdle      bool

	era       int // incremented at every backward clock jump
	away      bool
	awaySince int64
	awayEra   int
	aways     []fzInterval // closed away intervals; the interior must stay empty
	jumps     []fzInterval // forward clock-jump gaps
	hard      map[int64]bool
	acts      []int64 // idle backdate targets: ts - floor(IdleS*1000)

	haveFirst  bool
	first, max int64
}

func newOracle() *fzOracle { return &fzOracle{hard: map[int64]bool{}} }

// accept applies the clock rule: a gap beyond +-JumpMax is a jump, a smaller
// backward skew is clamped up to the last accepted TS.
func (o *fzOracle) accept(ts int64) (int64, bool) {
	jumped := false
	if o.haveLast {
		gap := ts - o.last
		switch {
		case gap > fzJumpMax || gap < -fzJumpMax:
			jumped = true
			o.hard[o.last], o.hard[ts] = true, true
			if gap > 0 {
				o.jumps = append(o.jumps, fzInterval{o.last, ts, o.era})
			} else {
				// Backward jump: the away time so far ends at the last
				// accepted TS, and a new era starts at the new clock.
				if o.away {
					o.aways = append(o.aways, fzInterval{o.awaySince, o.last, o.awayEra})
					o.awaySince = ts
				}
				o.era++
				o.awayEra = o.era
			}
			o.last = ts
			o.ignoreIdle = true
			// Away timers follow the new clock.
			if o.asleep && o.sleepTS > ts {
				o.sleepTS = ts
			}
			if o.locked && o.lockTS > ts-fzSettle {
				o.lockTS = ts - fzSettle
			}
		case ts < o.last:
			ts = o.last
		}
	}
	if !o.haveFirst {
		o.haveFirst, o.first, o.max = true, ts, ts
	} else if ts > o.max {
		o.max = ts
	}
	return ts, jumped
}

func (o *fzOracle) refreshAway(ts int64) {
	now := o.asleep || o.locked
	switch {
	case now && !o.away:
		o.away, o.awaySince, o.awayEra = true, ts, o.era
	case !now && o.away:
		o.away = false
		o.aways = append(o.aways, fzInterval{o.awaySince, ts, o.awayEra})
	}
}

func (o *fzOracle) lockNow(ts int64) {
	o.hard[ts] = true
	o.locked, o.lockTS = true, ts
	o.refreshAway(ts)
}

// event models Sleep, Wake, Lock and Unlock. haveOpen/openStart describe the
// span open before this event.
func (o *fzOracle) event(ev Event, haveOpen bool, openStart int64) {
	ts, jumped := o.accept(ev.TS)
	switch ev.Kind {
	case EventSleep:
		o.hard[ts] = true
		o.asleep, o.sleepTS = true, ts
		o.refreshAway(ts)
	case EventLock:
		o.lockNow(ts)
	case EventWake, EventUnlock:
		// An event not newer than the open span is stale and ignored. A jump
		// has already closed the span, so nothing is stale then.
		if haveOpen && !jumped && ev.TS <= openStart {
			return
		}
		if ev.Kind == EventWake {
			o.hard[o.last] = true // an open span closes at the last known-good time
			o.asleep = false
		} else {
			o.locked = false
		}
		o.hard[ts] = true
		o.ignoreIdle = true
		o.refreshAway(ts)
	}
	o.last, o.haveLast = ts, true
}

func (o *fzOracle) sample(s Sample) {
	ts, _ := o.accept(s.TS)
	defer func() { o.last, o.haveLast = ts, true }()

	if o.asleep && ts-o.sleepTS >= fzSleepHeal {
		o.asleep = false
		o.ignoreIdle = true
		o.hard[ts] = true
		o.refreshAway(ts)
	}
	if s.BundleID == LoginWindowBundleID {
		if o.locked {
			o.lockTS = ts
		} else {
			o.lockNow(ts)
		}
		return
	}
	if s.Locked && !o.locked {
		o.lockNow(ts)
	} else if !s.Locked && o.locked && ts >= o.lockTS+fzSettle {
		o.locked = false
		o.ignoreIdle = true
		o.hard[ts] = true
		o.refreshAway(ts)
	}
	if o.asleep || o.locked {
		return
	}
	var idleMS int64
	if !(math.IsNaN(s.IdleS) || math.IsInf(s.IdleS, 0) || s.IdleS < 0) {
		idleMS = int64(math.Floor(s.IdleS * 1000))
	}
	if o.ignoreIdle {
		idleMS, o.ignoreIdle = 0, false
	}
	if idleMS >= fzIdleThr {
		act := ts - idleMS
		o.acts = append(o.acts, act)
		o.hard[act] = true
	}
}

// finish records the Flush boundary and the still-open away interval.
func (o *fzOracle) finish() {
	o.hard[o.last] = true
	if o.away {
		o.aways = append(o.aways, fzInterval{o.awaySince, math.MaxInt64, o.awayEra})
	}
}

// ---------------------------------------------------------------------------
// Invariants
// ---------------------------------------------------------------------------

// fzOverlap reports whether [start,end] shares tracked time with the interior
// of iv. A zero-length span tracks no time, so it never overlaps: this matters
// after a backward clock jump, where the merger clamps the first new span up to
// the previous span's end.
func fzOverlap(start, end int64, iv fzInterval) bool {
	lo, hi := start, end
	if iv.from > lo {
		lo = iv.from
	}
	if iv.to < hi {
		hi = iv.to
	}
	return lo < hi
}

// fzCheck asserts the seven invariants on the spans left after Flush
// (sorted by start, then id). eras maps span id to the era it was opened in.
func fzCheck(spans []Span, o *fzOracle, eras map[int64]int) error {
	hard := map[int64]bool{}
	for k := range o.hard {
		hard[k] = true
	}
	for _, iv := range o.aways {
		hard[iv.from], hard[iv.to] = true, true
	}
	for _, sp := range spans {
		if sp.BundleID == IdleBundleID {
			hard[sp.Start] = true
			if sp.End != nil {
				hard[*sp.End] = true
			}
		}
	}

	var total, minStart int64
	minStart = math.MaxInt64
	for i, sp := range spans {
		// (2) closed, and end >= start.
		if sp.End == nil {
			return fmt.Errorf("(2) span %s is still open after Flush", fmtSpan(sp))
		}
		end := *sp.End
		if end < sp.Start {
			return fmt.Errorf("(2) span %s ends before it starts", fmtSpan(sp))
		}
		// (1) ordered and non-overlapping.
		if i > 0 && sp.Start < *spans[i-1].End {
			return fmt.Errorf("(1) span %s overlaps %s", fmtSpan(sp), fmtSpan(spans[i-1]))
		}
		// (3) a short non-idle span only survives next to a hard boundary.
		if sp.BundleID != IdleBundleID && end-sp.Start < fzFoldMin && !hard[sp.Start] && !hard[end] {
			return fmt.Errorf("(3) short span %s is not adjacent to a hard boundary", fmtSpan(sp))
		}
		// (4) nothing inside an away interval or a forward jump gap.
		for _, iv := range o.aways {
			if iv.era == eras[sp.ID] && fzOverlap(sp.Start, end, iv) {
				return fmt.Errorf("(4) span %s overlaps away interval (%d,%d)", fmtSpan(sp), iv.from, iv.to)
			}
		}
		for _, iv := range o.jumps {
			if iv.era == eras[sp.ID] && fzOverlap(sp.Start, end, iv) {
				return fmt.Errorf("(4) span %s overlaps clock-jump gap (%d,%d)", fmtSpan(sp), iv.from, iv.to)
			}
		}
		// (5) private spans carry no title or URL.
		if sp.Private && (sp.Title != nil || sp.URL != nil) {
			return fmt.Errorf("(5) private span %s carries a title or URL", fmtSpan(sp))
		}
		// (7) an idle span starts at an idle backdate target, or at the end
		// of the span before it when the target was clamped to what is
		// already recorded.
		if sp.BundleID == IdleBundleID {
			ok := false
			for _, act := range o.acts {
				if act == sp.Start || (act <= sp.Start && i > 0 && sp.Start == *spans[i-1].End) {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("(7) idle span %s does not start at a backdate target %v", fmtSpan(sp), o.acts)
			}
		}
		total += end - sp.Start
		if sp.Start < minStart {
			minStart = sp.Start
		}
	}
	// (6) tracked time cannot exceed wall time. An idle span on the very first
	// sample may start before the first accepted TS (backdated), so wall time
	// begins at the earliest of the two.
	if len(spans) > 0 {
		lo := o.first
		if minStart < lo {
			lo = minStart
		}
		if wall := o.max - lo; total > wall {
			return fmt.Errorf("(6) tracked %d ms exceeds wall time %d ms", total, wall)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Driver
// ---------------------------------------------------------------------------

// fzRun feeds one decoded stream to a fresh merger and checks everything. The
// returned error carries the stream trace.
func fzRun(t testing.TB, data []byte) (err error) {
	t.Helper()
	steps := fzDecode(data)
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w\nstream (%d steps, %d bytes):\n%s", err, len(steps), len(data), fzTrace(steps))
		}
	}()

	m := New(DefaultConfig())
	led, or := newLedger(), newOracle()
	var all []Op
	eras := map[int64]int{} // span id -> era it was opened in
	feed := func(i int, ops []Op) error {
		for _, op := range ops {
			if err := led.apply(op); err != nil {
				return fmt.Errorf("op validity at step %d (%s): %w", i, op.Kind, err)
			}
			if op.Kind == OpOpen {
				eras[op.Span.ID] = or.era
			}
		}
		all = append(all, ops...)
		return nil
	}
	for i, st := range steps {
		var ops []Op
		switch {
		case st.tick:
			ops = m.Tick(st.now)
		case st.in.Event != nil:
			start, open := led.openStart()
			or.event(*st.in.Event, open, start)
			ops = m.Feed(st.in)
		default:
			or.sample(*st.in.Sample)
			ops = m.Feed(st.in)
		}
		if err := feed(i, ops); err != nil {
			return err
		}
	}
	if err := feed(len(steps), m.Flush()); err != nil {
		return err
	}
	or.finish()
	if led.openID != 0 {
		return fmt.Errorf("(2) span %d is open after Flush", led.openID)
	}

	spans := replay(t, all)
	if len(spans) != len(led.spans) {
		return fmt.Errorf("replay has %d spans, ledger has %d", len(spans), len(led.spans))
	}
	sort.SliceStable(spans, func(a, b int) bool { return spans[a].Start < spans[b].Start })
	if err := fzCheck(spans, or, eras); err != nil {
		var sb strings.Builder
		for _, sp := range spans {
			fmt.Fprintf(&sb, "  %s\n", fmtSpan(sp))
		}
		return fmt.Errorf("%w\nspans:\n%s", err, sb.String())
	}
	return nil
}

// ---------------------------------------------------------------------------
// Seed builders (readable corpus entries)
// ---------------------------------------------------------------------------

func fzIdx(table []int64, v int64) byte {
	for i, d := range table {
		if d == v {
			return byte(i)
		}
	}
	panic(fmt.Sprintf("no delta %d in table", v))
}

func fzIdleIdx(v float64) byte {
	for i, d := range fzIdle {
		if d == v {
			return byte(i)
		}
	}
	panic(fmt.Sprintf("no idle %v in table", v))
}

// fzS builds a plain sample step: bundle index, title index, delta in ms.
func fzS(bundle, title int, dt int64) []byte { return fzSX(bundle, title, false, false, 0, dt) }

// fzSX builds a sample step with every knob.
func fzSX(bundle, title int, private, locked bool, idle float64, dt int64) []byte {
	b2 := byte(0x24) + byte(title) // not private, not locked
	if private {
		b2 = 0x20 + byte(title)
	}
	if locked {
		b2 = 0x04 + byte(title)
	}
	return []byte{0, byte(bundle), b2, fzIdleIdx(idle), fzIdx(fzDelta[:], dt)}
}

// fzE builds an event step.
func fzE(k EventKind, dt int64) []byte {
	return []byte{byte(14 + int(k) - 1), 0, 0, 0, fzIdx(fzDelta[:], dt)}
}

// fzT builds a Tick step.
func fzT(dt int64) []byte { return []byte{18, 0, 0, 0, fzIdx(fzDelta[:], dt)} }

func fzCat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ---------------------------------------------------------------------------
// Fuzz target and fixed-seed replay
// ---------------------------------------------------------------------------

func FuzzInvariants(f *testing.F) {
	// Title settle after an app switch.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 2000), fzS(0, 0, 2000), fzS(1, 1, 2000), fzS(1, 2, 150), fzS(1, 2, 10000)))
	// Quick-switch chain folded into the long span before it.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzS(1, 0, 1999), fzS(2, 0, 1000), fzS(3, 0, 1999), fzS(3, 0, 10000)))
	// Back to the previous key after a short detour: reopen.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzS(1, 0, 1000), fzS(0, 0, 1000), fzS(0, 0, 10000)))
	// Short span before sleep, then wake.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzS(1, 0, 1000), fzE(EventSleep, 1000), fzE(EventWake, 30000), fzS(0, 0, 1000), fzS(0, 0, 10000)))
	// Lock event, unlock event, short span, polled lock and lock-screen bundle.
	f.Add(fzCat(fzS(0, 0, 150), fzE(EventLock, 10000), fzE(EventUnlock, 3000), fzS(1, 0, 500), fzS(2, 0, 1000),
		fzSX(0, 0, false, true, 0, 10000), fzS(7, 0, 3000), fzS(0, 0, 3000), fzS(0, 0, 10000)))
	// Idle trigger and return, including 299.999 s and 300 s.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzSX(0, 0, false, false, 299.999, 2000), fzSX(0, 0, false, false, 300, 2000),
		fzSX(0, 0, false, false, 400, 2000), fzSX(1, 0, false, false, 1.5, 3000), fzS(1, 0, 10000)))
	// Clock jumps: forward 120001, forward 600000, backward -200000 and -5000.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzS(1, 0, 120001), fzS(2, 0, 600000), fzS(3, 0, -200000), fzS(1, 0, -5000), fzS(1, 0, 2000)))
	// Private spans: reopen of a private key, titles dropped.
	f.Add(fzCat(fzSX(0, 1, true, false, 0, 150), fzSX(0, 2, true, false, 0, 10000), fzS(1, 0, 1000), fzSX(0, 3, true, false, 0, 1000), fzSX(0, 1, true, false, 0, 10000)))
	// Equal timestamps and ticks mixed with samples.
	f.Add(fzCat(fzS(0, 0, 150), fzS(1, 0, 0), fzS(2, 0, 0), fzT(2000), fzS(2, 0, 2000), fzT(10000), fzS(3, 0, 1999), fzE(EventSleep, 0), fzE(EventWake, 0)))
	// Sleep self-heal: samples resume 60 s after Sleep with no Wake.
	f.Add(fzCat(fzS(0, 0, 150), fzS(0, 0, 10000), fzE(EventSleep, 1000), fzS(0, 0, 30000), fzS(0, 0, 30000), fzS(1, 0, 2000), fzS(1, 0, 10000)))
	// Empty stream.
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4000 {
			data = data[:4000]
		}
		if err := fzRun(t, data); err != nil {
			t.Fatal(err)
		}
	})
}

// TestInvariantsRandomSeeds replays the same checker over 500 fixed random
// streams so plain `go test` exercises it without -fuzz.
func TestInvariantsRandomSeeds(t *testing.T) {
	for seed := uint64(1); seed <= 500; seed++ {
		r := rand.New(rand.NewPCG(seed, seed*0x9E3779B97F4A7C15))
		data := make([]byte, (5+r.IntN(120))*fzStepBytes)
		for i := range data {
			data[i] = byte(r.UintN(256))
		}
		if err := fzRun(t, data); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

// TestInvariantSeedsDecode guards the seed builders: every seed decodes to the
// steps it was written as.
func TestInvariantSeedsDecode(t *testing.T) {
	steps := fzDecode(fzCat(fzS(2, 3, 1999), fzE(EventLock, 150), fzT(2000), fzSX(1, 2, true, false, 400, 0)))
	if len(steps) != 4 {
		t.Fatalf("decoded %d steps, want 4", len(steps))
	}
	s := steps[0].in.Sample
	if s == nil || s.BundleID != "com.c" || s.Title == nil || *s.Title != "t3" || s.URL == nil || s.Private || s.Locked || s.TS != fzBase+1999 {
		t.Fatalf("step 0 decoded wrong: %+v", s)
	}
	if ev := steps[1].in.Event; ev == nil || ev.Kind != EventLock || ev.TS != fzBase+1999+150 {
		t.Fatalf("step 1 decoded wrong: %+v", steps[1].in.Event)
	}
	if !steps[2].tick || steps[2].now != fzBase+1999+150+2000 {
		t.Fatalf("step 2 decoded wrong: %+v", steps[2])
	}
	p := steps[3].in.Sample
	if p == nil || !p.Private || p.IdleS != 400 || p.Locked || p.Title == nil || *p.Title != "secret" {
		t.Fatalf("step 3 decoded wrong: %+v", p)
	}
}
