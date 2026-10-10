package report

import (
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

func bv(from, to int, bundle string) classify.SpanView {
	return classify.SpanView{Start: int64(from) * 60_000, End: int64(to) * 60_000, BundleID: bundle, App: bundle}
}

func TestBlocks(t *testing.T) {
	code := classify.Label{Category: "Code", Kind: classify.KindDeep, Project: "tock"}
	atlas := classify.Label{Category: "Code", Kind: classify.KindDeep, Project: "ATLAS"}
	yt := classify.Label{Category: "Video", Kind: classify.KindDistract}
	vs := []classify.SpanView{
		bv(0, 40, "term"),   // tock
		bv(40, 45, "arc"),   // a short detour: stays in the tock block
		bv(45, 70, "term"),  // tock
		bv(70, 100, "term"), // ATLAS, long: its own block
		bv(100, 130, span.IdleBundleID),
		bv(130, 135, "term"), // tock, after a break, too short alone
		bv(200, 230, "arc"),  // video after a gap: a block of its own
	}
	ls := []classify.Label{code, yt, code, atlas, {}, code, yt}
	bs := Blocks(vs, ls)
	if len(bs) != 3 {
		t.Fatalf("blocks = %+v", bs)
	}
	if b := bs[0]; b.Name != "tock" || b.Category != "Code" || b.Start != 0 || b.End != 70*60_000 ||
		b.ActiveMS != 70*60_000 || b.DistractMS != 5*60_000 || b.DeepMS != 65*60_000 || b.Top[0].Name != "term" {
		t.Fatalf("first = %+v", b)
	}
	if b := bs[1]; b.Name != "ATLAS" || b.ActiveMS != 30*60_000 {
		t.Fatalf("second = %+v", b)
	}
	if b := bs[2]; b.Name != "Video" || b.Kind != classify.KindDistract || b.Project != "" {
		t.Fatalf("third = %+v", b)
	}
	if got := BlockLine(bs[0], time.UTC); got != "00:00–01:10   1h10  tock (Code) · deep 93% · distracted 5m · term, arc" {
		t.Fatalf("line = %q", got)
	}
}
