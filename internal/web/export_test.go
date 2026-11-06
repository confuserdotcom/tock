package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Downloads come back as attachments with the token check of every other
// API call.
func TestReportDownload(t *testing.T) {
	f := newFixture(t)
	code, h, b := f.do("GET", "/api/report?date=2026-10-10", testToken, nil)
	if code != 200 {
		t.Fatalf("report: %d %s", code, b)
	}
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type %q", ct)
	}
	if cd := h.Get("Content-Disposition"); cd != `attachment; filename="tock-week-2026-10-05.html"` {
		t.Fatalf("disposition %q", cd)
	}
	if !strings.Contains(string(b), "<html") || strings.Contains(string(b), "tock │ ✳ x") {
		t.Fatalf("want the week page without titles, got %.200s", b)
	}
	if code, _, _ := f.do("GET", "/api/report", "-", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, _, _ := f.do("GET", "/api/report?date=10/10", testToken, nil); code != 400 {
		t.Fatalf("bad date: %d", code)
	}
}

func TestExportDownload(t *testing.T) {
	f := newFixture(t)
	code, h, b := f.do("GET", "/api/export?from=2026-10-05&to=2026-10-11", testToken, nil)
	if code != 200 {
		t.Fatalf("csv: %d %s", code, b)
	}
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content type %q", ct)
	}
	if cd := h.Get("Content-Disposition"); cd != `attachment; filename="tock-2026-10-05-to-2026-10-11.csv"` {
		t.Fatalf("disposition %q", cd)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 2 || !strings.Contains(string(b), "WezTerm") {
		t.Fatalf("csv rows: %q", b)
	}

	code, h, b = f.do("GET", "/api/export?from=2026-10-10&to=2026-10-10&format=json", testToken, nil)
	var rows []map[string]any
	if code != 200 || json.Unmarshal(b, &rows) != nil || len(rows) == 0 {
		t.Fatalf("json: %d %.200s", code, b)
	}
	if cd := h.Get("Content-Disposition"); cd != `attachment; filename="tock-2026-10-10-to-2026-10-10.json"` {
		t.Fatalf("disposition %q", cd)
	}

	code, h, b = f.do("GET", "/api/export?from=2026-10-10&to=2026-10-10&by=project", testToken, nil)
	if code != 200 || !strings.HasSuffix(h.Get("Content-Disposition"), `-projects.csv"`) {
		t.Fatalf("by project: %d %q %s", code, h.Get("Content-Disposition"), b)
	}

	for _, q := range []string{"format=xml", "by=app", "from=2026-10-11&to=2026-10-05", "from=yesterday", "from=2025-01-01&to=2026-10-10"} {
		if code, _, b := f.do("GET", "/api/export?"+q, testToken, nil); code != 400 {
			t.Errorf("%s: %d %s", q, code, b)
		}
	}
	if code, _, _ := f.do("GET", "/api/export", "-", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
}

// The cap counts calendar days, so a DST shift inside the range can't
// stretch it by a day.
func TestExportTooLong(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	d := func(s string) time.Time {
		v, err := time.ParseInLocation(dateLayout, s, ny)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		from, to string
		want     bool
	}{
		{"2025-03-09", "2026-03-09", false}, // 366 days, spring shift in range
		{"2025-03-08", "2026-03-09", true},  // 367 days, under 367*24h of wall time
		{"2025-11-01", "2026-11-01", false},
		{"2025-10-31", "2026-11-01", true},
		{"2026-10-10", "2026-10-10", false},
	} {
		if got := exportTooLong(d(c.from), d(c.to)); got != c.want {
			t.Errorf("%s..%s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
