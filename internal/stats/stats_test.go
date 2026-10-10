package stats

import (
	"reflect"
	"testing"
)

func TestTotals(t *testing.T) {
	vs, ls := at(deep(10*mn), shallow(5*mn), distract(3*mn), unlabelled(2*mn), idle(20*mn), after(mn, deep(mn)))
	d := Compute(vs, ls)
	if d.ActiveMS != 21*mn || d.DeepMS != 11*mn || d.DistractMS != 3*mn {
		t.Fatalf("active %d deep %d distract %d", d.ActiveMS/mn, d.DeepMS/mn, d.DistractMS/mn)
	}
}

// blockLens returns each block's DeepMS.
func blockLens(d Day) []int64 {
	var out []int64
	for _, b := range d.Blocks {
		out = append(out, b.DeepMS)
	}
	return out
}

func TestBlocks(t *testing.T) {
	tests := []struct {
		name string
		segs []seg
		want []int64 // DeepMS per block
	}{
		{"one deep run", []seg{deep(30 * mn)}, []int64{30 * mn}},
		{"adjacent deep views join", []seg{deep(3 * mn), deep(3 * mn)}, []int64{6 * mn}},
		{"interruption just under 60 s is tolerated",
			[]seg{deep(10 * mn), distract(60*sec - 1), deep(10 * mn)}, []int64{20 * mn}},
		{"interruption of 60 s ends the block",
			[]seg{deep(10 * mn), distract(60 * sec), deep(10 * mn)}, []int64{10 * mn, 10 * mn}},
		{"several kinds in one short interruption",
			[]seg{deep(10 * mn), shallow(20 * sec), unlabelled(20 * sec), deep(10 * mn)}, []int64{20 * mn}},
		{"gap with no span is interruption",
			[]seg{deep(10 * mn), after(59*sec, deep(10*mn))}, []int64{20 * mn}},
		{"gap of 60 s ends the block",
			[]seg{deep(10 * mn), after(60*sec, deep(10*mn))}, []int64{10 * mn, 10 * mn}},
		{"idle ends the block however short",
			[]seg{deep(10 * mn), idle(sec), deep(10 * mn)}, []int64{10 * mn, 10 * mn}},
		{"block under 5 min is dropped", []seg{deep(5*mn - 1)}, nil},
		{"block of exactly 5 min is kept", []seg{deep(5 * mn)}, []int64{5 * mn}},
		{"short runs joined by tolerance make a block",
			[]seg{deep(3 * mn), shallow(30 * sec), deep(2 * mn)}, []int64{5 * mn}},
		{"open block at the end of the input counts",
			[]seg{shallow(mn), deep(7 * mn)}, []int64{7 * mn}},
		{"no deep time", []seg{shallow(30 * mn), distract(10 * mn)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, ls := at(tt.segs...)
			if got := blockLens(Compute(vs, ls)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("blocks = %v min, want %v min", mins(got), mins(tt.want))
			}
		})
	}
}

func mins(ms []int64) []float64 {
	var out []float64
	for _, m := range ms {
		out = append(out, float64(m)/float64(mn))
	}
	return out
}

func TestBlockBoundsAndTotals(t *testing.T) {
	vs, ls := at(deep(10*mn), shallow(30*sec), deep(20*mn), idle(10*mn), deep(25*mn), distract(2*mn), deep(6*mn))
	d := Compute(vs, ls)
	if len(d.Blocks) != 3 {
		t.Fatalf("blocks = %v", mins(blockLens(d)))
	}
	b := d.Blocks[0]
	if b.Start != vs[0].Start || b.End != vs[2].End || b.DeepMS != 30*mn {
		t.Fatalf("first block = %+v, want start of view 1, end of view 3, 30 min", b)
	}
	if d.FocusMS != 61*mn || d.LongMS != 55*mn || d.LongestMS != 30*mn {
		t.Fatalf("focus %v long %v longest %v", float64(d.FocusMS)/float64(mn), float64(d.LongMS)/float64(mn), float64(d.LongestMS)/float64(mn))
	}
}

func TestLongBlockThreshold(t *testing.T) {
	for _, tt := range []struct {
		ms, long int64
	}{{25*mn - 1, 0}, {25 * mn, 25 * mn}} {
		vs, ls := at(deep(tt.ms))
		if d := Compute(vs, ls); d.LongMS != tt.long {
			t.Errorf("block of %d ms: LongMS = %d, want %d", tt.ms, d.LongMS, tt.long)
		}
	}
}

func TestSwitches(t *testing.T) {
	tests := []struct {
		name string
		segs []seg
		want int
	}{
		{"one kind", []seg{deep(30 * mn), deep(30 * mn)}, 0},
		{"two kinds", []seg{deep(mn), shallow(mn)}, 1},
		{"stretch just under 30 s is ignored", []seg{deep(mn), distract(30*sec - 1), deep(mn)}, 0},
		{"stretch of 30 s counts", []seg{deep(mn), distract(30 * sec), deep(mn)}, 2},
		{"short stretch does not change the remembered kind",
			[]seg{deep(mn), shallow(10 * sec), distract(mn)}, 1},
		{"same-kind views merge before the threshold",
			[]seg{deep(mn), distract(20 * sec), distract(20 * sec), deep(mn)}, 2},
		{"unlabelled is a kind", []seg{deep(mn), unlabelled(mn)}, 1},
		{"idle forgets the kind", []seg{deep(mn), idle(10 * mn), shallow(mn)}, 0},
		{"5 min gap forgets the kind", []seg{deep(mn), after(5*mn, shallow(mn))}, 0},
		{"shorter gap keeps the kind", []seg{deep(mn), after(5*mn-1, shallow(mn))}, 1},
		{"first stretch is never a switch", []seg{distract(mn)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, ls := at(tt.segs...)
			if got := Compute(vs, ls).Switches; got != tt.want {
				t.Fatalf("switches = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEmptyDay(t *testing.T) {
	if d := Compute(nil, nil); !reflect.DeepEqual(d, Day{}) {
		t.Fatalf("Compute(nil) = %+v", d)
	}
}
