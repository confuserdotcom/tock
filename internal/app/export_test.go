package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/report"
)

func exportEnv(t *testing.T) (*App, *apptest.Env) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t,
		apptest.Span{ID: 9, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ z", Day: -3},
		apptest.Span{ID: 10, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "old", Day: -8})
	return open(t, e), e
}

func midnightOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func TestExportWeek(t *testing.T) {
	a, e := exportEnv(t)
	today := midnightOf(e.Clock)
	rows, err := a.Export(today.AddDate(0, 0, -6), today.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 {
		t.Fatalf("rows = %d, want 9", len(rows))
	}
	byID := map[int64]report.ExportRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if _, ok := byID[10]; ok {
		t.Error("span from 8 days ago exported")
	}
	if r := byID[1]; r.Category != "Code" || r.Project != "tock" || r.CategorySource != "rule" || r.Kind != "deep" {
		t.Errorf("span 1 = %+v", r)
	}
	if r := byID[6]; r.Category != "" || r.CategorySource != "none" {
		t.Errorf("idle = %+v", r)
	}
	if r := byID[8]; !r.End.Equal(e.Clock) {
		t.Errorf("open span ends %v, want now", r.End)
	}
	if r := byID[9]; r.Start.Location() != e.Clock.Location() || r.Title != "tock │ z" {
		t.Errorf("span 9 = %+v", r)
	}
}

func TestExportClips(t *testing.T) {
	a, e := exportEnv(t)
	today := midnightOf(e.Clock)
	rows, err := a.Export(today.Add(10*time.Hour), today.Add(11*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Start.Hour() != 10 || rows[1].End.Hour() != 11 {
		t.Fatalf("clipped rows = %+v", rows)
	}
}

// Phase 7 criterion 3: a week exported as csv or json parses back to the
// same rows.
func TestExportRoundTrip(t *testing.T) {
	a, e := exportEnv(t)
	today := midnightOf(e.Clock)
	rows, err := a.Export(today.AddDate(0, 0, -6), today.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}

	var b bytes.Buffer
	if err := report.WriteCSV(&b, rows); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&b).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var fromCSV []report.ExportRow
	for _, rec := range recs[1:] {
		id, _ := strconv.ParseInt(rec[0], 10, 64)
		start, err1 := time.Parse(time.RFC3339, rec[1])
		end, err2 := time.Parse(time.RFC3339, rec[2])
		priv, err3 := strconv.ParseBool(rec[8])
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatal(err1, err2, err3)
		}
		if d, _ := strconv.ParseInt(rec[3], 10, 64); d != int64(end.Sub(start)/time.Second) {
			t.Errorf("duration_s %d disagrees with start/end", d)
		}
		fromCSV = append(fromCSV, report.ExportRow{ID: id, Start: start, End: end, App: rec[4], BundleID: rec[5],
			Title: rec[6], URL: rec[7], Private: priv, Category: rec[9], Kind: rec[10], Project: rec[11],
			CategorySource: rec[12], ProjectSource: rec[13]})
	}
	sameRows(t, "csv", fromCSV, rows)

	b.Reset()
	if err := report.WriteJSON(&b, rows); err != nil {
		t.Fatal(err)
	}
	var js []struct {
		ID             int64  `json:"id"`
		Start          string `json:"start"`
		End            string `json:"end"`
		App            string `json:"app"`
		BundleID       string `json:"bundle_id"`
		Title          string `json:"title"`
		URL            string `json:"url"`
		Private        bool   `json:"private"`
		Category       string `json:"category"`
		Kind           string `json:"kind"`
		Project        string `json:"project"`
		CategorySource string `json:"category_source"`
		ProjectSource  string `json:"project_source"`
	}
	if err := json.Unmarshal(b.Bytes(), &js); err != nil {
		t.Fatal(err)
	}
	var fromJSON []report.ExportRow
	for _, j := range js {
		start, _ := time.Parse(time.RFC3339, j.Start)
		end, _ := time.Parse(time.RFC3339, j.End)
		fromJSON = append(fromJSON, report.ExportRow{ID: j.ID, Start: start, End: end, App: j.App, BundleID: j.BundleID,
			Title: j.Title, URL: j.URL, Private: j.Private, Category: j.Category, Kind: j.Kind, Project: j.Project,
			CategorySource: j.CategorySource, ProjectSource: j.ProjectSource})
	}
	sameRows(t, "json", fromJSON, rows)
}

func sameRows(t *testing.T, name string, got, want []report.ExportRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d rows, want %d", name, len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.Start.Equal(w.Start) || !g.End.Equal(w.End) {
			t.Errorf("%s row %d times %v-%v, want %v-%v", name, i, g.Start, g.End, w.Start, w.End)
		}
		g.Start, g.End, w.Start, w.End = time.Time{}, time.Time{}, time.Time{}, time.Time{}
		if g != w {
			t.Errorf("%s row %d = %+v, want %+v", name, i, g, w)
		}
	}
}
