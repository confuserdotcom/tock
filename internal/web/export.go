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
	o, err := report.ParseExport(q.Get("from"), q.Get("to"), q.Get("format"), q.Get("by"), s.now())
	if err != nil {
		return nil, badRequest(err.Error())
	}
	if exportTooLong(o.From, o.To) {
		return nil, badRequest("an export covers at most a year")
	}
	rows, err := s.a.Export(o.From, o.End())
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := report.WriteExport(&b, rows, o); err != nil {
		return nil, err
	}
	return file{o.Name(), o.ContentType(), b.Bytes()}, nil
}
