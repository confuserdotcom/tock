package focus

import (
	"math"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/stats"
)

// LevelHalfLifeMS is how fast the focus level forgets: a moment this long
// ago weighs half as much as now, so five focused minutes after a drop win
// back half the gap.
const LevelHalfLifeMS = 5 * 60_000

// TrendMS is how far back the trend compares the level with.
const TrendMS = 60_000

// Live is how focused a running session is now: Level in percent, -1 when
// nothing scored yet, and Trend, the change in points over the last
// TrendMS (0 when the session is younger).
type Live struct {
	Level int
	Trend int
}

// seg is a scored stretch of a session.
type seg struct {
	from, to int64
	score    float64
	deep     bool
	idle     bool
}

// kindScore is what a moment of kind counts towards the level: deep work
// fully, light or unknown work half, browsing a quarter, distraction
// nothing.
func kindScore(kind string) float64 {
	switch kind {
	case classify.KindDeep:
		return 1
	case classify.KindBrowse:
		return 0.25
	case classify.KindDistract:
		return 0
	}
	return 0.5
}

// Level measures the focus of the views clipped to [from, now): each moment
// scores by its kind (idle 0, meetings skipped), a non-deep stretch shorter
// than stats.ToleranceMS between deep ones counts as deep, and the scores
// are averaged with weights halving every LevelHalfLifeMS back from now.
func Level(views []classify.SpanView, labels []classify.Label, from, now int64, cfg config.Focus) Live {
	segs := forgive(scored(views, labels, from, now, cfg))
	lv := Live{Level: level(segs, now)}
	if then := now - TrendMS; then > from && lv.Level >= 0 {
		if before := level(segs, then); before >= 0 {
			lv.Trend = lv.Level - before
		}
	}
	return lv
}

// scored turns the views into scored stretches clipped to [from, to).
func scored(views []classify.SpanView, labels []classify.Label, from, to int64, cfg config.Focus) []seg {
	var out []seg
	for i, v := range views {
		s, e := max(v.Start, from), min(v.End, to)
		if e <= s || IsMeeting(v, cfg) {
			continue
		}
		if isIdle(v) {
			out = append(out, seg{s, e, 0, false, true})
			continue
		}
		k := labels[i].Kind
		out = append(out, seg{s, e, kindScore(k), k == classify.KindDeep, false})
	}
	return out
}

// forgive scores as deep each run of non-idle, non-deep stretches shorter
// than stats.ToleranceMS with deep work right before and right after it.
func forgive(segs []seg) []seg {
	for i := 0; i < len(segs); {
		if segs[i].deep {
			i++
			continue
		}
		j, idle := i, false
		for j < len(segs) && !segs[j].deep {
			idle = idle || segs[j].idle
			j++
		}
		if i > 0 && j < len(segs) && !idle && segs[j-1].to-segs[i].from < stats.ToleranceMS {
			for k := i; k < j; k++ {
				segs[k].score = 1
			}
		}
		i = j
	}
	return segs
}

// level is the weighted average score of segs up to at in percent, -1 when
// nothing before at scored.
func level(segs []seg, at int64) int {
	h := float64(LevelHalfLifeMS)
	w := func(t int64) float64 { return math.Exp2(-float64(at-t) / h) }
	var sum, weight float64
	for _, s := range segs {
		to := min(s.to, at)
		if to <= s.from {
			continue
		}
		part := w(to) - w(s.from) // ∫ 2^(−(at−t)/h) dt over the stretch, times ln2/h
		sum += part * s.score
		weight += part
	}
	if weight <= 0 {
		return -1
	}
	return int(math.Round(100 * sum / weight))
}
