package web

import (
	"bytes"
	"net/http"
	"time"

	"tock/internal/report"
)

// maxExportDays bounds one export, so a mistyped year can't stall the page.
const maxExportDays = 366

// exportTooLong reports whether from..to, both included, spans more than
// maxExportDays calendar days. It counts days, not hours, so a DST shift
// inside the range can't let an extra day through.
func exportTooLong(from, to time.Time) bool {
	return !to.Before(from.AddDate(0, 0, maxExportDays))
}

// file is a handler result sent as a download rather than JSON.
type file struct {
	name, ctype string
	body        []byte
}

func writeFile(w http.ResponseWriter, f file) {
	h := w.Header()
	h.Set("Content-Type", f.ctype)
	h.Set("Content-Disposition", `attachment; filename="`+f.name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.body)
}

// report is the week page `tock report` writes, for the week holding ?date=.
func (s *Server) report(r *http.Request) (any, error) {
	date, err := s.date(r)
	if err != nil {
		return nil, err
	}
	wk, err := s.a.Week(date)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := report.WriteWeekHTML(&b, wk, s.now()); err != nil {
		return nil, err
	}
	return file{"tock-week-" + wk.Monday.Format(dateLayout) + ".html", "text/html; charset=utf-8", b.Bytes()}, nil
}

// export is `tock export`: ?from= and ?to= (days, both included, default
// the last 7), ?format=csv|json, ?by=project for one row per project.
func (s *Server) export(r *http.Request) (any, error) {
	q := r.URL.Query()
	format, by := q.Get("format"), q.Get("by")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		return nil, badRequest("format must be csv or json")
	}
	if by != "" && by != "project" {
		return nil, badRequest("by must be project")
	}
	now := s.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day := func(k string, def time.Time) (time.Time, error) {
		v := q.Get(k)
		if v == "" {
			return def, nil
		}
		d, err := time.ParseInLocation(dateLayout, v, now.Location())
		if err != nil {
			return time.Time{}, badRequest(k + " must be YYYY-MM-DD")
		}
		return d, nil
	}
	from, err := day("from", today.AddDate(0, 0, -6))
	if err != nil {
		return nil, err
	}
	to, err := day("to", today)
	if err != nil {
		return nil, err
	}
	if from.After(to) {
		return nil, badRequest("from is after to")
	}
	if exportTooLong(from, to) {
		return nil, badRequest("an export covers at most a year")
	}
	rows, err := s.a.Export(from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	name := "tock-" + from.Format(dateLayout) + "-to-" + to.Format(dateLayout)
	switch {
	case by == "project" && format == "json":
		err = report.WriteBillJSON(&b, report.Bill(rows))
	case by == "project":
		err = report.WriteBillCSV(&b, report.Bill(rows))
	case format == "json":
		err = report.WriteJSON(&b, rows)
	default:
		err = report.WriteCSV(&b, rows)
	}
	if err != nil {
		return nil, err
	}
	if by == "project" {
		name += "-projects"
	}
	ctype := "text/csv; charset=utf-8"
	if format == "json" {
		ctype = "application/json; charset=utf-8"
	}
	return file{name + "." + format, ctype, b.Bytes()}, nil
}
