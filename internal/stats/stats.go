// Package stats turns one day of labelled spans into focus numbers: focus
// blocks, context switches, the focus score and the adaptive daily target.
// It is pure: no I/O and no clock.
package stats

import (
	"tock/internal/classify"
	"tock/internal/span"
)

const (
	BlockMinMS      = 5 * 60_000  // a focus block holds at least this much deep time
	LongBlockMS     = 25 * 60_000 // blocks this long count for the blocks term
	ToleranceMS     = 60_000      // a non-deep stretch shorter than this does not break a block
	SwitchMinMS     = 30_000      // a kind stretch this long can be a context switch
	ResetGapMS      = 5 * 60_000  // a gap this long resets switch counting, like idle
	WorkdayMinMS    = 60 * 60_000 // active time that makes a day a workday
	DefaultTargetMS = 4 * 3_600_000
	MinTargetMS     = 2 * 3_600_000
	MaxTargetMS     = 6 * 3_600_000
	TargetDays      = 14 // workdays in the median
	MinTargetDays   = 7  // workdays needed before the target adapts
	LookbackDays    = 42 // calendar days searched for workdays
	CalmZeroPerHour = 20 // switches per active hour that make calm 0
	PenaltyCap      = 15
	PenaltyPerMin   = 4           // distraction minutes per penalty point
	StretchFullMS   = 90 * 60_000 // a run this long makes the stretch term 1
)

// Block is one focus block.
type Block struct {
	Start, End int64 // first deep ms, last deep ms
	DeepMS     int64 // deep time inside [Start, End); the block's length
}

// Day is the focus numbers of one day.
type Day struct {
	ActiveMS  int64   // non-idle time
	DeepMS    int64   // all deep time
	FocusMS   int64   // sum of Blocks[i].DeepMS
	LongMS    int64   // sum of DeepMS of blocks with DeepMS >= LongBlockMS
	Blocks    []Block // chronological
	LongestMS int64   // largest block DeepMS, 0 with no blocks
	Switches  int
	// LongestRunMS is the longest unbroken stretch: activity with no context
	// switch, idle or long gap in it.
	LongestRunMS int64
	DistractMS   int64
}

func isIdle(v classify.SpanView) bool { return v.BundleID == span.IdleBundleID }

// Compute reads one day's views (clipped to the day, in start order; idle
// spans have span.IdleBundleID) and their labels (labels[i] labels views[i]).
func Compute(views []classify.SpanView, labels []classify.Label) Day {
	var d Day
	for i, v := range views {
		if isIdle(v) {
			continue
		}
		ms := v.DurMS()
		d.ActiveMS += ms
		switch labels[i].Kind {
		case classify.KindDeep:
			d.DeepMS += ms
		case classify.KindDistract:
			d.DistractMS += ms
		}
	}
	d.Blocks = blocks(views, labels)
	for _, b := range d.Blocks {
		d.FocusMS += b.DeepMS
		if b.DeepMS >= LongBlockMS {
			d.LongMS += b.DeepMS
		}
		d.LongestMS = max(d.LongestMS, b.DeepMS)
	}
	d.Switches, d.LongestRunMS = switches(views, labels)
	return d
}

// blocks finds the focus blocks: runs of deep views where the stretch
// between one deep view's end and the next one's start is shorter than
// ToleranceMS and holds no idle view. Runs with less than BlockMinMS of deep
// time are dropped.
func blocks(views []classify.SpanView, labels []classify.Label) []Block {
	var out []Block
	var cur *Block
	closeBlock := func() {
		if cur != nil && cur.DeepMS >= BlockMinMS {
			out = append(out, *cur)
		}
		cur = nil
	}
	for i, v := range views {
		switch {
		case isIdle(v):
			closeBlock()
		case labels[i].Kind == classify.KindDeep:
			if cur != nil && v.Start-cur.End >= ToleranceMS {
				closeBlock()
			}
			if cur == nil {
				cur = &Block{Start: v.Start}
			}
			cur.End = v.End
			cur.DeepMS += v.DurMS()
		}
	}
	closeBlock()
	return out
}
