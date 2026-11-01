package recommend

import "testing"

func TestRecs(t *testing.T) {
	now := int64(10_000_000)
	open := Rec{ID: 1, Kind: KindFocus, AtMS: now - 10*min1}
	if !open.Open(now) {
		t.Error("unanswered row 10 min old is not open")
	}
	if open.Open(open.AtMS + PendingMS) {
		t.Error("row exactly 30 min old is open")
	}
	if !open.Open(open.AtMS + PendingMS - 1) {
		t.Error("row 30 min minus 1 ms old is not open")
	}
	if open.Open(open.AtMS - 1) {
		t.Error("row from the future is open")
	}
	answered := Rec{ID: 2, Kind: KindFocus, AtMS: now - min1, Reply: ReplyDismiss}
	if answered.Open(now) {
		t.Error("answered row is open")
	}
	older := Rec{ID: 3, Kind: KindBreak, AtMS: now - 20*min1}
	recs := []Rec{older, open, answered}
	if p := Pending(recs, now); p == nil || p.ID != 1 {
		t.Errorf("Pending = %+v, want row 1 (newest open)", p)
	}
	if p := Pending([]Rec{answered}, now); p != nil {
		t.Errorf("Pending of answered = %+v, want nil", p)
	}
	if CountSince(recs, KindFocus, now-10*min1) != 2 || CountSince(recs, KindBreak, now-30*min1) != 1 || CountSince(recs, KindBreak, now) != 0 {
		t.Errorf("CountSince wrong")
	}
	if Last(recs, KindFocus) != answered.AtMS || Last(recs, KindBreak) != older.AtMS || Last(nil, KindFocus) != 0 {
		t.Errorf("Last wrong")
	}
}
