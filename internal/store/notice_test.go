package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestNoticeQueue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const day = int64(24 * 3600_000)
	now := 10 * day
	ancient, _ := s.AddNotice(Notice{AtMS: now - 8*day, Topic: "break", Title: "t", Body: "old"})
	stale, _ := s.AddNotice(Notice{AtMS: now - 5*60_000, Topic: "break", Title: "t", Body: "late"})
	fresh, err := s.AddNotice(Notice{AtMS: now - 30_000, Topic: "done", Title: "t", Body: "b", Data: `{"minutes":25}`})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TakeNotices(now, now-2*60_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != fresh || got[0].Data != `{"minutes":25}` || got[0].ShownMS != now {
		t.Fatalf("taken = %+v", got)
	}
	if got, err := s.TakeNotices(now+1000, 0); err != nil || len(got) != 0 {
		t.Fatalf("taken twice: %+v, %v", got, err)
	}
	if n, err := s.Notice(stale); err != nil || n.ShownMS != now || n.Body != "late" {
		t.Fatalf("a late notice is marked taken, not shown later: %+v, %v", n, err)
	}
	if n, err := s.Notice(fresh); err != nil || n.Topic != "done" {
		t.Fatalf("Notice = %+v, %v", n, err)
	}
	if _, err := s.Notice(ancient); !errors.Is(err, ErrNoNotice) {
		t.Fatalf("a taken notice older than a week must be deleted: %v", err)
	}
	if id, _ := s.AddNotice(Notice{AtMS: now, Topic: "recap", Title: "t", Body: "b"}); id <= fresh {
		t.Fatalf("ids must not be reused: %d after %d", id, fresh)
	}
	if n, _ := s.Notice(fresh + 1); n.Data != "{}" {
		t.Fatalf("empty data defaults to {}: %q", n.Data)
	}
}
