package span

import "math"

// key is the identity of a span: a change in any component starts a new span.
type key struct {
	bundle  string
	title   string
	url     string
	private bool
}

// closeReason says why a span is being closed. Anything but reasonKeyChange is
// a hard boundary: folding never crosses it (rule 7).
type closeReason int

const (
	reasonKeyChange closeReason = iota
	reasonIdle
	reasonSleep
	reasonLock
	reasonJump
	reasonFlush
)

// openSpan is the span currently being extended.
type openSpan struct {
	id            int64
	start         int64
	key           key
	bundleID      string
	idle          bool
	afterBoundary bool // previous close was hard, or nothing was closed before
}

// closedSpan is the most recently closed span.
type closedSpan struct {
	id       int64
	start    int64
	end      int64
	key      key
	bundleID string
	idle     bool
	hardEnd  bool // closed by anything except a key change

	afterBoundary bool // carried over so a Reopen restores the open span exactly
}

// Merger turns inputs into storage ops. It is pure: no clock, no I/O (D-15).
type Merger struct {
	cfg    Config
	nextID int64

	cur  *openSpan
	prev *closedSpan

	asleep, locked  bool
	sleepTS, lockTS int64
	lastTS          int64 // last accepted input time
	haveLast        bool
	lastHB          int64
	ignoreIdleNext  bool
}

// New returns a Merger. A zero cfg.NextID starts ids at 1.
func New(cfg Config) *Merger {
	next := cfg.NextID
	if next == 0 {
		next = 1
	}
	return &Merger{cfg: cfg, nextID: next}
}

func (m *Merger) keyOf(s *Sample) key {
	k := key{bundle: s.BundleID, private: s.Private}
	if s.Private {
		return k
	}
	if s.Title != nil {
		t := *s.Title
		if m.cfg.TitleKey != nil {
			t = m.cfg.TitleKey(t)
		}
		k.title = t
	}
	if s.URL != nil {
		k.url = *s.URL
	}
	return k
}

// idleMillis converts IdleS to integer milliseconds once, truncating toward
// zero. NaN, Inf and negative values count as 0.
func idleMillis(idleS float64) int64 {
	if math.IsNaN(idleS) || math.IsInf(idleS, 0) || idleS < 0 {
		return 0
	}
	return int64(math.Floor(idleS * 1000))
}

// foldMS is the fold threshold in ms (2000 with DefaultConfig). A span is short
// when it is strictly shorter, so exactly FoldMin is kept.
func (m *Merger) foldMS() int64 { return m.cfg.FoldMin.Milliseconds() }

// isShort reports whether the open span, closed at end, is a non-idle span
// under the fold threshold. Idle spans are never folded.
func (m *Merger) isShort(end int64) bool {
	c := m.cur
	if c == nil || c.idle {
		return false
	}
	if end < c.start {
		end = c.start
	}
	return end-c.start < m.foldMS()
}

// adjacentPrev returns the previous span when the open one may be folded into
// it: a non-idle span closed by a key change (not a hard boundary) at exactly
// the open span's start. Otherwise nil.
func (m *Merger) adjacentPrev() *closedSpan {
	p, c := m.prev, m.cur
	if p == nil || c == nil || p.idle || p.hardEnd || p.end != c.start {
		return nil
	}
	return p
}

// closeCur closes the open span at end (clamped to its start) and remembers it
// as prev. Every close in the merger goes through here.
//
// A short non-idle span closed by a hard boundary (rule 7) is folded into an
// adjacent previous span (Delete + Extend), or kept with a normal Close when
// there is none. Either way prev ends up marked hardEnd, so nothing folds
// across the boundary. Key-change closes are handled by switchTo.
func (m *Merger) closeCur(end int64, reason closeReason) []Op {
	c := m.cur
	if c == nil {
		return nil
	}
	if end < c.start {
		end = c.start
	}
	if reason != reasonKeyChange && m.isShort(end) {
		if p := m.adjacentPrev(); p != nil {
			p.end, p.hardEnd = end, true
			m.cur = nil
			return []Op{
				{Kind: OpDelete, ID: c.id},
				{Kind: OpExtend, ID: p.id, TS: end},
			}
		}
	}
	m.prev = &closedSpan{
		id: c.id, start: c.start, end: end, key: c.key, bundleID: c.bundleID,
		idle: c.idle, hardEnd: reason != reasonKeyChange, afterBoundary: c.afterBoundary,
	}
	m.cur = nil
	return []Op{{Kind: OpClose, ID: c.id, TS: end}}
}

// openNew opens a span at start for sample s (or an idle span when idle is
// true). Every open goes through here. The start never precedes the previous
// span's end, so spans cannot overlap.
func (m *Merger) openNew(start int64, s *Sample, idle bool) []Op {
	if m.prev != nil && start < m.prev.end {
		start = m.prev.end
	}
	sp := Span{ID: m.nextID, Start: start}
	var k key
	if idle {
		sp.BundleID, sp.App = IdleBundleID, IdleApp
		k = key{bundle: IdleBundleID}
	} else {
		sp.BundleID, sp.App = s.BundleID, s.App
		k = m.keyOf(s)
		if !s.Private {
			sp.Title, sp.URL = s.Title, s.URL
		}
		sp.Private = s.Private
	}
	m.nextID++
	m.cur = &openSpan{
		id: sp.ID, start: start, key: k, bundleID: sp.BundleID, idle: idle,
		afterBoundary: m.prev == nil || m.prev.hardEnd,
	}
	return []Op{{Kind: OpOpen, Span: sp}}
}

// switchTo is the key-change path (rule 7). When the open span is long enough
// it is closed at `at` and the sample's span opens there, so prev.end ==
// next.start. When it is short (under FoldMin, never idle) it is folded, with
// a deterministic preference:
//
//	(a) the sample has the same BundleID: Delete the short span and open the
//	    sample's span at the short span's start (title settling after a switch).
//	(b) an adjacent previous span exists (see adjacentPrev): Delete the short
//	    span; if the sample's key equals the previous span's key, Reopen the
//	    previous span, otherwise Extend it to `at` and open the sample's span
//	    there.
//	(c) otherwise: Delete the short span and open the sample's span at the
//	    short span's start.
func (m *Merger) switchTo(s *Sample, at int64) []Op {
	c := m.cur
	if c == nil || !m.isShort(at) {
		ops := m.closeCur(at, reasonKeyChange)
		return append(ops, m.openNew(at, s, false)...)
	}
	ops := []Op{{Kind: OpDelete, ID: c.id}}
	if s.BundleID != c.bundleID {
		if p := m.adjacentPrev(); p != nil {
			if m.keyOf(s) == p.key {
				ops = append(ops, Op{Kind: OpReopen, ID: p.id})
				m.cur = &openSpan{
					id: p.id, start: p.start, key: p.key, bundleID: p.bundleID,
					afterBoundary: p.afterBoundary,
				}
				// The reopened span is at least FoldMin long and the span
				// before it is no longer needed: nothing can fold into it.
				m.prev = nil
				return ops
			}
			ops = append(ops, Op{Kind: OpExtend, ID: p.id, TS: at})
			p.end = at
			m.cur = nil
			return append(ops, m.openNew(at, s, false)...)
		}
	}
	// (a) and (c): the sample's span inherits the short span's start.
	start := c.start
	m.cur = nil
	return append(ops, m.openNew(start, s, false)...)
}

const (
	// sleepHealMS: a sample this long after a Sleep event implies an
	// undelivered Wake (RESEARCH Q8 rule 3).
	sleepHealMS = 60_000
	// lockSettleMS: a polled Locked=false only clears a lock this long after
	// it was taken, so a stale poll right after a Lock event cannot undo it.
	lockSettleMS = 2_000
)

// hbMS is the heartbeat threshold in ms: Heartbeat - HeartbeatLead (7500 with
// DefaultConfig), never negative. A heartbeat can only land on a sample, so
// the lead (one poll interval plus poll lateness) keeps crash loss within
// Heartbeat even when polls run late (D-16, TRK-06).
func (m *Merger) hbMS() int64 {
	ms := (m.cfg.Heartbeat - m.cfg.HeartbeatLead).Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

// sanitize applies rule 1 (clock sanity) to an input timestamp. A gap larger
// than JumpMax in either direction is a clock jump: the open span closes at
// the last accepted TS (never at the new one). Smaller backward skew is
// clamped up to the last accepted TS. Comparisons are strict: exactly JumpMax
// is not a jump (D-14).
func (m *Merger) sanitize(ts int64) (int64, []Op) {
	if !m.haveLast {
		return ts, nil
	}
	jmax := m.cfg.JumpMax.Milliseconds()
	gap := ts - m.lastTS
	if gap > jmax || gap < -jmax {
		ops := m.closeCur(m.lastTS, reasonJump)
		m.lastTS = ts
		m.lastHB = 0 // a backward jump would otherwise silence heartbeats
		m.ignoreIdleNext = true
		// Re-anchor away-state timers to the new clock so a backward jump
		// cannot leave sleep or lock stuck until the old clock is reached.
		if m.asleep && m.sleepTS > ts {
			m.sleepTS = ts
		}
		if m.locked && m.lockTS > ts-lockSettleMS {
			m.lockTS = ts - lockSettleMS
		}
		return ts, ops
	}
	if ts < m.lastTS {
		ts = m.lastTS
	}
	return ts, nil
}

// lockNow closes the open span at ts and marks the console locked.
func (m *Merger) lockNow(ts int64) []Op {
	ops := m.closeCur(ts, reasonLock)
	m.locked, m.lockTS = true, ts
	return ops
}

// Feed consumes one input and returns the ops to apply, in order: closes,
// then opens, then heartbeat.
func (m *Merger) Feed(in Input) []Op {
	switch {
	case in.Event != nil:
		return m.feedEvent(*in.Event)
	case in.Sample != nil:
		return m.feedSample(in.Sample)
	}
	return nil
}

// feedEvent applies rule 2 (hard boundary events).
func (m *Merger) feedEvent(ev Event) []Op {
	switch ev.Kind {
	case EventSleep, EventWake, EventLock, EventUnlock:
	default:
		return nil
	}
	ts, ops := m.sanitize(ev.TS)
	switch ev.Kind {
	case EventSleep:
		ops = append(ops, m.closeCur(ts, reasonSleep)...) // closeCur clamps to span start
		m.asleep, m.sleepTS = true, ts
	case EventLock:
		ops = append(ops, m.lockNow(ts)...)
	case EventWake, EventUnlock:
		// An event not newer than the open span is stale: it was delivered
		// after the first post-boundary sample already opened a span.
		if m.cur != nil && ev.TS <= m.cur.start {
			return ops
		}
		if ev.Kind == EventWake {
			// Span still open means WillSleep was missed: the last accepted
			// TS is the last known-good time, never the wake time.
			ops = append(ops, m.closeCur(m.lastTS, reasonSleep)...)
			m.asleep = false
		} else {
			m.locked = false
		}
		m.ignoreIdleNext = true
	}
	m.lastTS, m.haveLast = ts, true
	return ops
}

// feedSample applies rules 1 and 3..6 and 8 to a sample.
func (m *Merger) feedSample(s *Sample) []Op {
	if s.BundleID == "" {
		return nil
	}
	ts, ops := m.sanitize(s.TS)
	defer func() { m.lastTS, m.haveLast = ts, true }()

	// Rule 3: self-heal.
	if m.asleep && ts-m.sleepTS >= sleepHealMS {
		m.asleep = false
		m.ignoreIdleNext = true
	}
	if s.BundleID == LoginWindowBundleID {
		// The lock screen shows loginwindow frontmost: treat it as a lock.
		if m.locked {
			m.lockTS = ts
		} else {
			ops = append(ops, m.lockNow(ts)...)
		}
		return ops
	}
	if s.Locked && !m.locked {
		ops = append(ops, m.lockNow(ts)...)
	} else if !s.Locked && m.locked && ts >= m.lockTS+lockSettleMS {
		m.locked = false
		m.ignoreIdleNext = true
	}

	// Rule 4: away time leaves no rows.
	if m.asleep || m.locked {
		return ops
	}

	idleMS := idleMillis(s.IdleS)
	if m.ignoreIdleNext {
		idleMS = 0
		m.ignoreIdleNext = false
	}
	thr := m.cfg.IdleThreshold.Milliseconds()
	// Once idle, only input ends it: a video that starts on its own does not.
	if s.Video && m.cfg.VideoIdleMax > m.cfg.IdleThreshold && (m.cur == nil || !m.cur.idle) {
		thr = m.cfg.VideoIdleMax.Milliseconds()
	}

	switch {
	case idleMS >= thr:
		// Rule 5 (D-11): idle. The activity time is when the user last
		// touched the machine, never earlier than what is already recorded.
		act := ts - idleMS
		floor := act
		if m.cur != nil {
			floor = m.cur.start
		} else if m.prev != nil {
			floor = m.prev.end
		}
		if act < floor {
			act = floor
		}
		switch {
		case m.cur != nil && m.cur.idle:
			// already idle: keep the span
		case m.cur != nil:
			ops = append(ops, m.closeCur(act, reasonIdle)...)
			ops = append(ops, m.openNew(act, s, true)...)
		default:
			ops = append(ops, m.openNew(act, s, true)...)
		}
	case m.cur != nil && m.cur.idle:
		// Activity resumed: end the idle span when the user came back.
		end := ts - idleMS
		if end < m.cur.start {
			end = m.cur.start
		}
		ops = append(ops, m.closeCur(end, reasonKeyChange)...)
		ops = append(ops, m.openNew(end, s, false)...)
	case m.cur != nil:
		if m.keyOf(s) != m.cur.key {
			ops = append(ops, m.switchTo(s, ts)...)
		}
	default:
		ops = append(ops, m.openNew(ts, s, false)...)
	}

	// Rule 8: heartbeat, only while a span is open.
	if m.cur != nil && ts-m.lastHB >= m.hbMS() {
		ops = append(ops, Op{Kind: OpHeartbeat, TS: ts})
		m.lastHB = ts
	}
	return ops
}

// Tick emits a pending heartbeat for the last accepted input when the
// threshold has passed since the previous one and a span is open.
func (m *Merger) Tick(nowMS int64) []Op {
	if m.cur == nil || m.lastTS <= m.lastHB || nowMS-m.lastHB < m.hbMS() {
		return nil
	}
	m.lastHB = m.lastTS
	return []Op{{Kind: OpHeartbeat, TS: m.lastTS}}
}

// Flush closes the open span at the last accepted input time, if any.
func (m *Merger) Flush() []Op {
	return m.closeCur(m.lastTS, reasonFlush)
}
