package stats

import (
	"tock/internal/classify"
	"tock/internal/span"
)

const (
	sec = int64(1000)
	mn  = 60 * sec
)

// seg is one span of a synthetic day. kind is a classify kind, "" for
// unlabelled or "idle" for an idle span. gap is the time with no span before
// it.
type seg struct {
	kind    string
	ms, gap int64
}

// at builds back-to-back views from a fixed instant.
func at(segs ...seg) ([]classify.SpanView, []classify.Label) {
	var vs []classify.SpanView
	var ls []classify.Label
	t := int64(1_791_000_000_000)
	for i, s := range segs {
		t += s.gap
		v := classify.SpanView{ID: int64(i + 1), Start: t, End: t + s.ms, BundleID: "app", App: "App"}
		l := classify.Label{Kind: s.kind}
		if s.kind == "idle" {
			v.BundleID, v.App, l = span.IdleBundleID, "Idle", classify.Label{}
		}
		vs = append(vs, v)
		ls = append(ls, l)
		t += s.ms
	}
	return vs, ls
}

func deep(ms int64) seg       { return seg{kind: classify.KindDeep, ms: ms} }
func shallow(ms int64) seg    { return seg{kind: classify.KindShallow, ms: ms} }
func distract(ms int64) seg   { return seg{kind: classify.KindDistract, ms: ms} }
func unlabelled(ms int64) seg { return seg{kind: "", ms: ms} }
func idle(ms int64) seg       { return seg{kind: "idle", ms: ms} }

// after puts a gap with no span before s.
func after(gap int64, s seg) seg { s.gap = gap; return s }
