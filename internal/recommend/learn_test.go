package recommend

import (
	"slices"
	"testing"
	"time"
)

func TestLearn(t *testing.T) {
	now := day0.UnixMilli() + 12*3600_000
	row := func(kind string, hour int, reply string, daysAgo int) Rec {
		return Rec{Kind: kind, Hour: hour, Reply: reply, AtMS: now - int64(daysAgo)*DayMS}
	}
	dis, acc := ReplyDismiss, ReplyAccept
	rows := func(kind string, hour int, replies ...string) []Rec {
		var out []Rec
		for i, r := range replies {
			out = append(out, row(kind, hour, r, i+1))
		}
		return out
	}

	l := Learn(rows(KindFocus, 9, dis, dis, dis), now)
	if !l.Muted(KindFocus, 9) || l.Strict(KindFocus, 9) || l.Muted(KindFocus, 10) {
		t.Errorf("three dismisses: muted %v strict %v", l.Muted(KindFocus, 9), l.Strict(KindFocus, 9))
	}
	if l.Muted(KindBreak, 9) {
		t.Error("focus replies muted the break kind")
	}
	if l := Learn(rows(KindFocus, 9, dis, dis), now); l.Muted(KindFocus, 9) || l.Strict(KindFocus, 9) {
		t.Error("two dismisses are muted or strict")
	}
	// 1 of 4 is 25 %: not under 25 %.
	if l := Learn(rows(KindFocus, 9, dis, dis, dis, acc), now); l.Muted(KindFocus, 9) || l.Strict(KindFocus, 9) {
		t.Error("1 of 4 accepted is muted or strict")
	}
	// 1 of 5 is 20 %.
	if l := Learn(rows(KindFocus, 9, dis, dis, dis, dis, acc), now); l.Muted(KindFocus, 9) || !l.Strict(KindFocus, 9) {
		t.Error("1 of 5 accepted is not strict")
	}
	// 3 answered, one accepted: neither (needs 4 for strict).
	if l := Learn(rows(KindFocus, 9, dis, dis, acc), now); l.Muted(KindFocus, 9) || l.Strict(KindFocus, 9) {
		t.Error("1 of 3 accepted is muted or strict")
	}
	// Unanswered rows never count.
	if l := Learn(rows(KindFocus, 9, dis, dis, ""), now); l.Muted(KindFocus, 9) {
		t.Error("an unanswered row counted")
	}
	// Rows sent more than 30 days ago age out; exactly 30 days still counts.
	old := []Rec{row(KindFocus, 9, dis, 31), row(KindFocus, 9, dis, 32), row(KindFocus, 9, dis, 33)}
	if Learn(old, now).Muted(KindFocus, 9) {
		t.Error("31 day old replies still mute")
	}
	edge := []Rec{row(KindFocus, 9, dis, 30), row(KindFocus, 9, dis, 30), row(KindFocus, 9, dis, 30)}
	if !Learn(edge, now).Muted(KindFocus, 9) {
		t.Error("replies exactly 30 days old do not count")
	}
	// Kinds are kept apart.
	mixed := append(rows(KindBreak, 15, dis, dis, dis), rows(KindFocus, 16, acc, dis)...)
	l = Learn(mixed, now)
	if !l.Muted(KindBreak, 15) || l.Muted(KindFocus, 15) {
		t.Error("break rows muted wrongly")
	}
	if a, n := l.Totals(KindFocus); a != 1 || n != 2 {
		t.Errorf("Totals(focus) = %d of %d, want 1 of 2", a, n)
	}
	if a, n := l.Totals(KindBreak); a != 0 || n != 3 {
		t.Errorf("Totals(break) = %d of %d, want 0 of 3", a, n)
	}
	if a, n := l.Totals("nap"); a != 0 || n != 0 {
		t.Errorf("Totals(nap) = %d of %d", a, n)
	}

	// MutedHours in clock order.
	all := append(rows(KindFocus, 16, dis, dis, dis), rows(KindFocus, 9, dis, dis, dis, dis)...)
	all = append(all, rows(KindFocus, 12, acc, dis, dis)...)
	if got := Learn(all, now).MutedHours(KindFocus); !slices.Equal(got, []int{9, 16}) {
		t.Errorf("MutedHours = %v, want [9 16]", got)
	}
	if got := Learn(nil, now).MutedHours(KindFocus); len(got) != 0 {
		t.Errorf("MutedHours of nothing = %v", got)
	}
	// An out of range hour in a row is ignored rather than a panic.
	_ = Learn([]Rec{{Kind: KindFocus, Hour: 99, Reply: dis, AtMS: now}, {Kind: KindFocus, Hour: -1, Reply: dis, AtMS: now}}, now)
}

func TestFocusHour(t *testing.T) {
	now := day0.UnixMilli() + 12*3600_000
	p := Profile{}
	p.Top[time.Saturday] = []int{10, 9}
	rows := func(hour int, replies ...string) Learning {
		var rs []Rec
		for _, r := range replies {
			rs = append(rs, Rec{Kind: KindFocus, Hour: hour, Reply: r, AtMS: now - DayMS})
		}
		return Learn(rs, now)
	}
	dis, acc := ReplyDismiss, ReplyAccept
	strict := rows(9, dis, dis, dis, dis, acc)
	if !FocusHour(p, Learning{}, time.Saturday, 9) {
		t.Error("rank 2, no replies: want true")
	}
	if FocusHour(p, strict, time.Saturday, 9) {
		t.Error("rank 2 and strict: want false")
	}
	if !FocusHour(p, rows(10, dis, dis, dis, dis, acc), time.Saturday, 10) {
		t.Error("rank 1 and strict: want true")
	}
	if FocusHour(p, rows(10, dis, dis, dis), time.Saturday, 10) {
		t.Error("muted: want false")
	}
	if FocusHour(p, Learning{}, time.Saturday, 11) {
		t.Error("rank 0: want false")
	}
	if FocusHour(p, Learning{}, time.Sunday, 10) {
		t.Error("another weekday: want false")
	}
}

// Break reminders repeat during one run, so several Not now taps in one hour of
// one day are one answer; muting needs the hour to be dismissed on 3 days.
func TestLearnBreakCountsOneAnswerPerDayAndHour(t *testing.T) {
	now := day0.UnixMilli() + 12*3600_000
	dis, acc := ReplyDismiss, ReplyAccept
	at := func(daysAgo int, minutes int64) int64 {
		return now - int64(daysAgo)*DayMS + minutes*min1
	}
	brk := func(daysAgo int, minutes int64, reply string) Rec {
		return Rec{Kind: KindBreak, Hour: 14, Reply: reply, AtMS: at(daysAgo, minutes)}
	}

	// Three taps in half an hour of one afternoon do not mute the hour.
	busy := []Rec{brk(1, 0, dis), brk(1, 10, dis), brk(1, 20, dis)}
	l := Learn(busy, now)
	if l.Muted(KindBreak, 14) {
		t.Error("three Not now in one run muted the hour")
	}
	if a, n := l.Totals(KindBreak); a != 0 || n != 1 {
		t.Errorf("Totals(break) = %d of %d, want 0 of 1", a, n)
	}

	// Three different days do.
	days := []Rec{brk(1, 0, dis), brk(1, 10, dis), brk(2, 0, dis), brk(3, 0, dis), brk(3, 10, dis)}
	if !Learn(days, now).Muted(KindBreak, 14) {
		t.Error("Not now on three different days did not mute the hour")
	}

	// A run that ended in taking the break counts as taken.
	taken := []Rec{brk(1, 0, dis), brk(1, 10, acc), brk(2, 0, dis), brk(3, 0, dis)}
	l = Learn(taken, now)
	if l.Muted(KindBreak, 14) {
		t.Error("an hour with a taken break is muted")
	}
	if a, n := l.Totals(KindBreak); a != 1 || n != 3 {
		t.Errorf("Totals(break) = %d of %d, want 1 of 3", a, n)
	}

	// Rows given newest first are grouped the same way.
	rev := []Rec{brk(3, 10, dis), brk(3, 0, dis), brk(2, 0, dis), brk(1, 10, dis), brk(1, 0, dis)}
	if !Learn(rev, now).Muted(KindBreak, 14) {
		t.Error("newest-first rows were not grouped")
	}

	// Focus suggestions are at most 2 a day and stay counted one by one.
	f := []Rec{{Kind: KindFocus, Hour: 14, Reply: dis, AtMS: at(1, 0)}, {Kind: KindFocus, Hour: 14, Reply: dis, AtMS: at(1, 10)}, {Kind: KindFocus, Hour: 14, Reply: dis, AtMS: at(1, 20)}}
	if !Learn(f, now).Muted(KindFocus, 14) {
		t.Error("focus replies were collapsed")
	}
}
