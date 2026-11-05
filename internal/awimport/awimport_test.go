package awimport

import (
	"testing"
	"time"
)

const export = `{"buckets": {
 "aw-watcher-window_mac": {"id": "aw-watcher-window_mac", "type": "currentwindow", "hostname": "mac",
  "events": [
   {"timestamp": "2026-10-10T09:00:00Z", "duration": 600, "data": {"app": "Code", "title": "main.go"}},
   {"timestamp": "2026-10-10T09:10:00Z", "duration": 300, "data": {"app": "Google Chrome", "title": "Go docs"}},
   {"timestamp": "2026-10-10T23:50:00Z", "duration": 1200, "data": {"app": "Code", "title": "late"}}
  ]},
 "aw-watcher-afk_mac": {"id": "aw-watcher-afk_mac", "type": "afkstatus", "hostname": "mac",
  "events": [
   {"timestamp": "2026-10-10T09:05:00Z", "duration": 120, "data": {"status": "afk"}},
   {"timestamp": "2026-10-10T09:07:00Z", "duration": 600, "data": {"status": "not-afk"}}
  ]},
 "aw-watcher-web-chrome": {"id": "aw-watcher-web-chrome", "type": "web.tab.current", "hostname": "mac",
  "events": [
   {"timestamp": "2026-10-10T09:10:00Z", "duration": 290, "data": {"url": "https://go.dev/doc?token=x", "title": "Documentation"}}
  ]}
}}`

func TestParse(t *testing.T) {
	days, err := Parse([]byte(export), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Key != "2026-10-10" || days[1].Key != "2026-10-11" {
		t.Fatalf("days = %+v", days)
	}
	at := func(s string) int64 {
		tm, _ := time.Parse(time.RFC3339, s)
		return tm.UnixMilli()
	}
	vs := days[0].Views
	if len(vs) != 4 {
		t.Fatalf("views = %+v", vs)
	}
	// The afk 09:05-09:07 is cut out of the editor's ten minutes.
	if vs[0].App != "Code" || vs[0].StartMS != at("2026-10-10T09:00:00Z") || vs[0].EndMS != at("2026-10-10T09:05:00Z") {
		t.Errorf("first = %+v", vs[0])
	}
	if vs[1].StartMS != at("2026-10-10T09:07:00Z") || vs[1].EndMS != at("2026-10-10T09:10:00Z") {
		t.Errorf("second = %+v", vs[1])
	}
	// The browser view takes the tab's URL and title.
	if vs[2].Bundle != "Google Chrome" || vs[2].URL == nil || *vs[2].URL != "go.dev/doc" || *vs[2].Title != "Documentation" {
		t.Errorf("browser = %+v", vs[2])
	}
	// A view over midnight is split between the two days.
	if vs[3].EndMS != at("2026-10-11T00:00:00Z") || days[1].Views[0].StartMS != at("2026-10-11T00:00:00Z") ||
		days[1].Views[0].EndMS != at("2026-10-11T00:10:00Z") {
		t.Errorf("midnight = %+v / %+v", vs[3], days[1].Views[0])
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{`not json`, `{"buckets": {}}`, `{"x": 1}`} {
		if _, err := Parse([]byte(in), time.UTC); err == nil {
			t.Errorf("Parse(%q) took it", in)
		}
	}
}
