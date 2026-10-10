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

func TestExportRates(t *testing.T) {
	rate := 80.0
	rows := exRows()
	rows[0].Rate = &rate // 100 min at 80 = 133.333…
	var b bytes.Buffer
	if err := WriteCSV(&b, rows); err != nil {
		t.Fatal(err)
	}
	recs, _ := csv.NewReader(&b).ReadAll()
	if h := recs[0]; h[14] != "rate" || h[15] != "amount" {
		t.Fatalf("header = %v", h)
	}
	if recs[1][14] != "80" || recs[1][15] != "133.33" || recs[2][14] != "" || recs[2][15] != "" {
		t.Fatalf("rows = %v / %v", recs[1][14:], recs[2][14:])
	}
	b.Reset()
	if err := WriteJSON(&b, rows); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got[0]["rate"] != 80.0 || got[0]["amount"] != 133.33 || got[1]["rate"] != nil || got[1]["amount"] != nil {
		t.Fatalf("json = %v", got)
	}
	if _, ok := got[1]["amount"]; !ok {
		t.Error("amount key missing; want null")
	}
}

func TestBill(t *testing.T) {
	r80, r0 := 80.0, 0.0
	t0 := time.Date(2026, 10, 10, 9, 0, 0, 0, exZone)
	row := func(p string, min int, rate *float64) ExportRow {
		return ExportRow{Start: t0, End: t0.Add(time.Duration(min) * time.Minute), Project: p, Rate: rate}
	}
	// Three 7-minute ATLAS rows: each rounds to 9.33, the total of 21 min is 28.00.
	bill := Bill([]ExportRow{row("tock", 30, &r0), row("ATLAS", 7, &r80), row("", 60, nil),
		row("ATLAS", 7, &r80), row("ATLAS", 7, &r80), row("free", 15, nil)})
	if len(bill) != 3 || bill[0].Project != "ATLAS" || bill[1].Project != "free" || bill[2].Project != "tock" {
		t.Fatalf("bill = %+v", bill)
	}
	if a := bill[0].Amount(); bill[0].DurationS != 1260 || bill[0].Hours() != 0.35 || a == nil || *a != 28 {
		t.Fatalf("ATLAS = %+v amount %v", bill[0], a)
	}
	if bill[1].Amount() != nil || *bill[2].Amount() != 0 {
		t.Fatal("no rate must be nil, rate 0 must be 0")
	}
	var b bytes.Buffer
	if err := WriteBillCSV(&b, bill); err != nil {
		t.Fatal(err)
	}
	want := "project,duration_s,hours,rate,amount\nATLAS,1260,0.35,80,28.00\nfree,900,0.25,,\ntock,1800,0.50,0,0.00\n"
	if b.String() != want {
		t.Fatalf("csv =\n%s\nwant\n%s", b.String(), want)
	}
	b.Reset()
	if err := WriteBillJSON(&b, bill); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || len(got) != 3 || got[0]["amount"] != 28.0 ||
		got[0]["hours"] != 0.35 || got[1]["rate"] != nil {
		t.Fatalf("json = %s (%v)", b.String(), err)
	}
}
