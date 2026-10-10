package report

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"time"
)

// ExportRow is one span as `tock export` writes it.
type ExportRow struct {
	ID                            int64
	Start, End                    time.Time
	App, BundleID, Title, URL     string
	Private                       bool
	Category, Kind, Project       string
	CategorySource, ProjectSource string
}

// ExportHeader is the CSV header and the JSON key order.
var ExportHeader = []string{"id", "start", "end", "duration_s", "app", "bundle_id", "title", "url",
	"private", "category", "kind", "project", "category_source", "project_source"}

// DurationS is the row's length in whole seconds.
func (r ExportRow) DurationS() int64 { return int64(r.End.Sub(r.Start) / time.Second) }

// WriteCSV writes a header and one record per row.
func WriteCSV(w io.Writer, rows []ExportRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ExportHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{strconv.FormatInt(r.ID, 10), r.Start.Format(time.RFC3339), r.End.Format(time.RFC3339),
			strconv.FormatInt(r.DurationS(), 10), r.App, r.BundleID, r.Title, r.URL,
			strconv.FormatBool(r.Private), r.Category, r.Kind, r.Project, r.CategorySource, r.ProjectSource}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

type exportJSON struct {
	ID             int64  `json:"id"`
	Start          string `json:"start"`
	End            string `json:"end"`
	DurationS      int64  `json:"duration_s"`
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

// WriteJSON writes the rows as an indented JSON array.
func WriteJSON(w io.Writer, rows []ExportRow) error {
	out := make([]exportJSON, len(rows))
	for i, r := range rows {
		out[i] = exportJSON{r.ID, r.Start.Format(time.RFC3339), r.End.Format(time.RFC3339), r.DurationS(),
			r.App, r.BundleID, r.Title, r.URL, r.Private, r.Category, r.Kind, r.Project,
			r.CategorySource, r.ProjectSource}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
