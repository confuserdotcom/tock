package stats

import "tock/internal/classify"

// stretch is a run of consecutive non-idle views of one kind.
type stretch struct {
	kind  string
	ms    int64
	reset bool // an idle view or a long gap came before it
}

// stretches merges consecutive same-kind views. A different kind, an idle
// view or a gap of ResetGapMS or more ends a stretch; the last two also mark
// the next stretch as following a reset.
func stretches(views []classify.SpanView, labels []classify.Label) []stretch {
	var out []stretch
	reset := true
	var lastEnd int64
	for i, v := range views {
		if isBreak(v) {
			reset = true
			continue
		}
		if len(out) > 0 && v.Start-lastEnd >= ResetGapMS {
			reset = true
		}
		k := labels[i].Kind
		if n := len(out); n > 0 && !reset && out[n-1].kind == k {
			out[n-1].ms += v.DurMS()
		} else {
			out = append(out, stretch{kind: k, ms: v.DurMS(), reset: reset})
		}
		reset = false
		lastEnd = v.End
	}
	return out
}

// switches counts context switches: a stretch of at least SwitchMinMS whose
// kind differs from the last such stretch. Shorter stretches are ignored; a
// reset forgets the last kind. It also returns the longest run: the most
// activity between two switches or resets, short stretches included.
func switches(views []classify.SpanView, labels []classify.Label) (n int, longest int64) {
	var last string
	have := false
	var run int64
	for _, s := range stretches(views, labels) {
		if s.reset {
			have, run = false, 0
		}
		if s.ms >= SwitchMinMS {
			if have && s.kind != last {
				n++
				run = 0
			}
			last, have = s.kind, true
		}
		run += s.ms
		longest = max(longest, run)
	}
	return n, longest
}
