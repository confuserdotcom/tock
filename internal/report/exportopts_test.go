package report

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseExport(t *testing.T) {
	now := time.Date(2026, 10, 14, 15, 30, 0, 0, exZone)
	day := func(s string) time.Time {
		d, _ := time.ParseInLocation("2006-01-02", s, exZone)
		return d
	}

	o, err := ParseExport("", "", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if o.Format != "csv" || o.By != "" || !o.From.Equal(day("2026-10-08")) || !o.To.Equal(day("2026-10-14")) {
		t.Fatalf("defaults: %+v", o)
	}
	if !o.End().Equal(day("2026-10-15")) {
		t.Fatalf("end %v", o.End())
	}
	if o.Name() != "tock-2026-10-08-to-2026-10-14.csv" || !strings.HasPrefix(o.ContentType(), "text/csv") {
		t.Fatalf("csv name %q type %q", o.Name(), o.ContentType())
	}

	o, err = ParseExport("2026-10-10", "2026-10-10", "json", "project", now)
	if err != nil {
		t.Fatal(err)
	}
	if o.Name() != "tock-2026-10-10-to-2026-10-10-projects.json" || !strings.HasPrefix(o.ContentType(), "application/json") {
		t.Fatalf("json name %q type %q", o.Name(), o.ContentType())
	}

	for _, c := range []struct{ from, to, format, by, want string }{
		{"", "", "xml", "", "format"},
		{"", "", "", "app", "by"},
		{"yesterday", "", "", "", "from"},
		{"", "10/10", "", "", "to"},
		{"2026-10-11", "2026-10-05", "", "", "after"},
	} {
		_, err := ParseExport(c.from, c.to, c.format, c.by, now)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want an error about %q", c, err, c.want)
		}
	}
}

func TestWriteExport(t *testing.T) {
	rows := exRows()
	for _, c := range []struct{ format, by string }{{"csv", ""}, {"json", ""}, {"csv", "project"}, {"json", "project"}} {
		var got, want bytes.Buffer
		if err := WriteExport(&got, rows, ExportOpts{Format: c.format, By: c.by}); err != nil {
			t.Fatal(err)
		}
		switch c {
		case struct{ format, by string }{"csv", ""}:
			_ = WriteCSV(&want, rows)
		case struct{ format, by string }{"json", ""}:
			_ = WriteJSON(&want, rows)
		case struct{ format, by string }{"csv", "project"}:
			_ = WriteBillCSV(&want, Bill(rows))
		default:
			_ = WriteBillJSON(&want, Bill(rows))
		}
		if got.String() != want.String() {
			t.Errorf("%+v: got %.80q, want %.80q", c, got.String(), want.String())
		}
	}
}
