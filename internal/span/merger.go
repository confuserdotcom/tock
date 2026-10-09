package span

import "math"

// key is the identity of a span: a change in any component starts a new span.
type key struct {
	bundle  string
	title   string
	url     string
	private bool
}

// closeReason says why a span is being closed. Plan 01-03 (folding) uses it to
// decide whether a boundary is hard (never folded across).
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

// closeCur closes the open span at end (clamped to its start) and remembers it
// as prev. Every close in the merger goes through here.
func (m *Merger) closeCur(end int64, reason closeReason) []Op {
	c := m.cur
	if c == nil {
		return nil
	}
	if end < c.start {
		end = c.start
	}
	m.prev = &closedSpan{
		id: c.id, start: c.start, end: end, key: c.key, bundleID: c.bundleID,
		idle: c.idle, hardEnd: reason != reasonKeyChange,
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

// switchTo is the key-change path: close the open span at `at` and open the
// sample's span there, so prev.end == next.start. Plan 01-03 replaces its body
// with fold-aware logic.
func (m *Merger) switchTo(s *Sample, at int64) []Op {
	ops := m.closeCur(at, reasonKeyChange)
	return append(ops, m.openNew(at, s, false)...)
}

// Feed consumes one input and returns the ops to apply, in order.
func (m *Merger) Feed(in Input) []Op {
	s := in.Sample
	if s == nil || s.BundleID == "" {
		return nil
	}
	ts := s.TS
	m.lastTS, m.haveLast = ts, true

	idleMS := idleMillis(s.IdleS)
	thr := m.cfg.IdleThreshold.Milliseconds()

	var ops []Op
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
	return ops
}

// Tick is a no-op until the heartbeat rule lands.
func (m *Merger) Tick(nowMS int64) []Op { return nil }

// Flush closes the open span at the last input time, if any.
func (m *Merger) Flush() []Op {
	return m.closeCur(m.lastTS, reasonFlush)
}
