package snapshot

import (
	"testing"
	"time"
)

func TestNames(t *testing.T) {
	at := time.Date(2026, 11, 7, 3, 12, 0, 0, time.UTC).UnixMilli()
	if got := DailyName(at, 23); got != "daily-20261107T031200Z-v0023.db.gz" {
		t.Fatalf("DailyName = %q", got)
	}
	if got := PreName(23, 24); got != "pre-v0023-to-v0024.db.gz" {
		t.Fatalf("PreName = %q", got)
	}
	d, ok := Parse("daily-20261107T031200Z-v0023.db.gz")
	if !ok || d.Kind != Daily || d.AtMS != at || d.Schema != 23 || d.To != 0 {
		t.Fatalf("Parse daily = %+v %v", d, ok)
	}
	p, ok := Parse("pre-v0023-to-v0024.db.gz")
	if !ok || p.Kind != Pre || p.Schema != 23 || p.To != 24 || p.AtMS != 0 {
		t.Fatalf("Parse pre = %+v %v", p, ok)
	}
	for _, bad := range []string{"", "tock.db", "daily-2026-v0023.db.gz", "pre-v23-to-v24.db.gz", "daily-20261107T031200Z-v0023.db.gz.tmp", ".daily-20261107T031200Z-v0023.db.tmp"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}
