package report

import (
	"fmt"
	"io"
	"time"
)

// ExportOpts is one export as tock export and the dashboard's Export ask
// for it: the days From..To (both included, local midnights), csv or
// json, and By "project" for one row per project.
type ExportOpts struct {
	Format, By string
	From, To   time.Time
}

// ParseExport reads an export's options as given: dates YYYY-MM-DD in now's
// zone (default the last 7 days up to today), format csv (default) or json,
// by empty or "project". Errors name the option at fault.
func ParseExport(from, to, format, by string, now time.Time) (ExportOpts, error) {
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		return ExportOpts{}, fmt.Errorf("format %q, want csv or json", format)
	}
	if by != "" && by != "project" {
		return ExportOpts{}, fmt.Errorf("by %q, want project", by)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day := func(k, v string, def time.Time) (time.Time, error) {
		if v == "" {
			return def, nil
		}
		d, err := time.ParseInLocation("2006-01-02", v, now.Location())
		if err != nil {
			return time.Time{}, fmt.Errorf("%s %q, want YYYY-MM-DD", k, v)
		}
		return d, nil
	}
	f, err := day("from", from, today.AddDate(0, 0, -6))
	if err != nil {
		return ExportOpts{}, err
	}
	t, err := day("to", to, today)
	if err != nil {
		return ExportOpts{}, err
	}
	if f.After(t) {
		return ExportOpts{}, fmt.Errorf("from is after to")
	}
	return ExportOpts{Format: format, By: by, From: f, To: t}, nil
}

// End is the midnight after the last day, the exclusive end to query.
func (o ExportOpts) End() time.Time { return o.To.AddDate(0, 0, 1) }

// Name is the export's file name, as the dashboard saves it.
func (o ExportOpts) Name() string {
	n := "tock-" + o.From.Format("2006-01-02") + "-to-" + o.To.Format("2006-01-02")
	if o.By == "project" {
		n += "-projects"
	}
	return n + "." + o.Format
}

// ContentType is the export's media type.
func (o ExportOpts) ContentType() string {
	if o.Format == "json" {
		return "application/json; charset=utf-8"
	}
	return "text/csv; charset=utf-8"
}

// WriteExport writes rows to w as o asks.
func WriteExport(w io.Writer, rows []ExportRow, o ExportOpts) error {
	switch {
	case o.By == "project" && o.Format == "json":
		return WriteBillJSON(w, Bill(rows))
	case o.By == "project":
		return WriteBillCSV(w, Bill(rows))
	case o.Format == "json":
		return WriteJSON(w, rows)
	default:
		return WriteCSV(w, rows)
	}
}
