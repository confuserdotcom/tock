package span

// key is the identity of a span: a change in any component starts a new span.
type key struct {
	bundle  string
	title   string
	url     string
	private bool
}

// Merger turns inputs into storage ops. It is pure: no clock, no I/O (D-15).
// Tracer slice: only the key-change rule is implemented; plans 01-02 and 01-03
// add events, idle, clock-jump, fold and heartbeat.
type Merger struct {
	cfg    Config
	nextID int64

	open   *Span
	openK  key
	lastTS int64
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

// Feed consumes one input and returns the ops to apply, in order.
func (m *Merger) Feed(in Input) []Op {
	s := in.Sample
	if s == nil || s.BundleID == "" {
		return nil
	}
	k := m.keyOf(s)
	m.lastTS = s.TS

	var ops []Op
	if m.open != nil {
		if k == m.openK {
			return nil
		}
		ops = append(ops, Op{Kind: OpClose, ID: m.open.ID, TS: s.TS})
	}

	sp := Span{
		ID:       m.nextID,
		Start:    s.TS,
		BundleID: s.BundleID,
		App:      s.App,
		Title:    s.Title,
		URL:      s.URL,
		Private:  s.Private,
	}
	if s.Private {
		sp.Title, sp.URL = nil, nil
	}
	m.nextID++
	m.open = &sp
	m.openK = k
	ops = append(ops, Op{Kind: OpOpen, Span: sp})
	return ops
}

// Tick is a no-op in the tracer slice.
func (m *Merger) Tick(nowMS int64) []Op { return nil }

// Flush closes the open span at the last input time, if any.
func (m *Merger) Flush() []Op {
	if m.open == nil {
		return nil
	}
	op := Op{Kind: OpClose, ID: m.open.ID, TS: m.lastTS}
	m.open = nil
	return []Op{op}
}
