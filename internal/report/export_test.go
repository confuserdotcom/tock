package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"testing"
	"time"
)

var exZone = time.FixedZone("T", 2*3600)

func exRows() []ExportRow {
	t0 := time.Date(2026, 10, 10, 9, 0, 0, 0, exZone)
	return []ExportRow{
		{ID: 1, Start: t0, End: t0.Add(100 * time.Minute), App: "WezTerm", BundleID: "term",
			Title: `tock │ "x", y`, Category: "Code", Kind: "deep", Project: "tock",
			CategorySource: "rule", ProjectSource: "rule"},
		{ID: 2, Start: t0.Add(2 * time.Hour), End: t0.Add(2*time.Hour + 90*time.Second), App: "Arc", BundleID: "arc",
			Title: "line1\nline2", URL: "example.com/a", Private: true, CategorySource: "none", ProjectSource: "none"},
	}
}

func TestWriteCSV(t *testing.T) {
	var b bytes.Buffer
	if err := WriteCSV(&b, exRows()); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&b).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("records = %d, want header + 2", len(recs))
	}
	if got := recs[0]; len(got) != len(ExportHeader) || got[0] != "id" || got[13] != "project_source" {
		t.Fatalf("header = %v", got)
	}
	want := []string{"1", "2026-10-10T09:00:00+02:00", "2026-10-10T10:40:00+02:00", "6000", "WezTerm", "term",
		`tock │ "x", y`, "", "false", "Code", "deep", "tock", "rule", "rule"}
	for i := range want {
		if recs[1][i] != want[i] {
			t.Errorf("row1[%s] = %q, want %q", ExportHeader[i], recs[1][i], want[i])
		}
	}
	if recs[2][3] != "90" || recs[2][6] != "line1\nline2" || recs[2][8] != "true" {
		t.Errorf("row2 = %v", recs[2])
	}
}

func TestWriteJSON(t *testing.T) {
	var b bytes.Buffer
	if err := WriteJSON(&b, exRows()); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["duration_s"] != float64(6000) || got[1]["private"] != true ||
		got[0]["start"] != "2026-10-10T09:00:00+02:00" || got[0]["category_source"] != "rule" {
		t.Fatalf("json = %v", got)
	}
	if b.Bytes()[b.Len()-1] != '\n' {
		t.Error("no trailing newline")
	}
	b.Reset()
	if err := WriteJSON(&b, nil); err != nil || b.String() != "[]\n" {
		t.Errorf("empty = %q, %v", b.String(), err)
	}
}
