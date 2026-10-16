package span

import (
	"testing"
	"time"
)

// videoPolls feeds 2 s polls of a video playing in bundle from `from`
// through `to`, with no input since `activity`.
func videoPolls(m *Merger, bundle string, from, to, activity int64) []Op {
	var ops []Op
	for ts := from; ts <= to; ts += 2000 {
		in := S(ts, bundle, "", float64(max(ts-activity, 0))/1000)
		in.Sample.Video = true
		ops = append(ops, m.Feed(*in)...)
	}
	return ops
}

func TestVideoKeepsSpanPastIdleThreshold(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	ops := videoPolls(m, "Arc", t0, a+40*60_000, a)
	assertSpans(t, replay(t, ops), []Span{WO(t0, "Arc")})
}

func TestVideoGoesIdleAtCap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VideoIdleMax = 20 * time.Minute
	m := New(cfg)
	a := t0 + 8000
	ops := videoPolls(m, "Arc", t0, a+21*60_000, a)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Arc"),
		WO(a, IdleBundleID),
	})
}

func TestVideoStoppingGoesIdleBackdated(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	ops := videoPolls(m, "Arc", t0, a+10*60_000, a)
	// The video ends; no input for 10 min already: idle from the last input.
	ops = append(ops, m.Feed(*S(a+10*60_000+2000, "Arc", "", float64(10*60_000+2000)/1000))...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Arc"),
		WO(a, IdleBundleID),
	})
}

func TestVideoStartingWhileIdleStaysIdle(t *testing.T) {
	m := New(DefaultConfig())
	a := t0 + 8000
	ops := polls(m, "Arc", t0, a+6*60_000, a)
	// An autoplaying video, still no input: the user is still away.
	ops = append(ops, videoPolls(m, "Arc", a+6*60_000+2000, a+12*60_000, a)...)
	assertSpans(t, replay(t, ops), []Span{
		W(t0, a, "Arc"),
		WO(a, IdleBundleID),
	})
}
