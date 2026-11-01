package store

import (
	"errors"
	"testing"
)

func TestRecommendRows(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a, err := s.AddRecommend(Recommend{Kind: "focus", AtMS: 2000, Weekday: 6, Hour: 9, LengthMS: 1500000})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddRecommend(Recommend{Kind: "focus", AtMS: 1000, Weekday: 6, Hour: 9, LengthMS: 1500000})
	if err != nil || b <= a {
		t.Fatalf("ids %d, %d (err %v), want increasing", a, b, err)
	}
	r, err := s.Recommend(a)
	if err != nil || r.ID != a || r.Kind != "focus" || r.AtMS != 2000 || r.Weekday != 6 || r.Hour != 9 || r.LengthMS != 1500000 || r.Reply != "" || r.ReplyMS != 0 {
		t.Fatalf("Recommend = %+v (err %v)", r, err)
	}
	if _, err := s.Recommend(999); !errors.Is(err, ErrRecommendGone) {
		t.Fatalf("Recommend(999) err = %v, want ErrRecommendGone", err)
	}
	rs, err := s.Recommends(1500)
	if err != nil || len(rs) != 1 || rs[0].ID != a {
		t.Fatalf("Recommends(1500) = %+v (err %v), want only row %d", rs, err, a)
	}
	rs, err = s.Recommends(0)
	if err != nil || len(rs) != 2 || rs[0].ID != b || rs[1].ID != a {
		t.Fatalf("Recommends(0) = %+v (err %v), want (at_ms, id) order", rs, err)
	}

	ok, err := s.ReplyRecommend(a, "accept", 2500, 1000)
	if err != nil || !ok {
		t.Fatalf("first reply = %v (err %v), want true", ok, err)
	}
	ok, err = s.ReplyRecommend(a, "dismiss", 2600, 1000)
	if err != nil || ok {
		t.Fatalf("second reply = %v (err %v), want false", ok, err)
	}
	r, _ = s.Recommend(a)
	if r.Reply != "accept" || r.ReplyMS != 2500 {
		t.Fatalf("row = %+v, want accept at 2500", r)
	}
	ok, err = s.ReplyRecommend(b, "accept", 2500, 1000)
	if err != nil || ok {
		t.Fatalf("reply to a row at_ms <= since = %v (err %v), want false", ok, err)
	}

	if err := s.UnreplyRecommend(a, "dismiss"); err != nil {
		t.Fatal(err)
	}
	if r, _ = s.Recommend(a); r.Reply != "accept" {
		t.Fatalf("unreply of the wrong reply changed the row: %+v", r)
	}
	if err := s.UnreplyRecommend(a, "accept"); err != nil {
		t.Fatal(err)
	}
	if r, _ = s.Recommend(a); r.Reply != "" || r.ReplyMS != 0 {
		t.Fatalf("row after unreply = %+v, want reply '' and reply_ms 0", r)
	}
}

func TestRecommendPrune(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	now := int64(100) * 24 * 3600_000
	old, _ := s.AddRecommend(Recommend{Kind: "focus", AtMS: now - RecommendKeepMS - 1000})
	keep, _ := s.AddRecommend(Recommend{Kind: "focus", AtMS: now - 59*24*3600_000})
	if _, err := s.AddRecommend(Recommend{Kind: "focus", AtMS: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recommend(old); !errors.Is(err, ErrRecommendGone) {
		t.Errorf("row older than 60 days: err = %v, want pruned", err)
	}
	if _, err := s.Recommend(keep); err != nil {
		t.Errorf("row 59 days old: err = %v, want kept", err)
	}
}
