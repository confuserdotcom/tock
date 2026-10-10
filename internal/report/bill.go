package report

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strconv"
)

// BillRow is one project's total over an export range.
type BillRow struct {
	Project   string
	DurationS int64
	Rate      *float64 // nil = the project has no rate
}

// BillHeader is the CSV header and the JSON key order of tock export --by
// project.
var BillHeader = []string{"project", "duration_s", "hours", "rate", "amount"}

// Hours is the total in hours, rounded to two decimals.
func (b BillRow) Hours() float64 { return math.Round(float64(b.DurationS)/3600*100) / 100 }

// Amount is the rate times the total time, rounded to cents; nil without a
// rate.
func (b BillRow) Amount() *float64 { return amount(b.Rate, b.DurationS) }

// Bill sums rows per project, sorted by name. Rows without a project are
// left out.
func Bill(rows []ExportRow) []BillRow {
	idx := map[string]int{}
	var out []BillRow
	for _, r := range rows {
		if r.Project == "" {
			continue
		}
		i, ok := idx[r.Project]
		if !ok {
			i = len(out)
			idx[r.Project] = i
			out = append(out, BillRow{Project: r.Project, Rate: r.Rate})
		}
		out[i].DurationS += r.DurationS()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out
}

// WriteBillCSV writes a header and one record per project.
func WriteBillCSV(w io.Writer, rows []BillRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(BillHeader); err != nil {
		return err
	}
	for _, b := range rows {
		rec := []string{b.Project, strconv.FormatInt(b.DurationS, 10), strconv.FormatFloat(b.Hours(), 'f', 2, 64),
			money(b.Rate, -1), money(b.Amount(), 2)}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

type billJSON struct {
	Project   string   `json:"project"`
	DurationS int64    `json:"duration_s"`
	Hours     float64  `json:"hours"`
	Rate      *float64 `json:"rate"`
	Amount    *float64 `json:"amount"`
}

// WriteBillJSON writes the projects as an indented JSON array.
func WriteBillJSON(w io.Writer, rows []BillRow) error {
	out := make([]billJSON, len(rows))
	for i, b := range rows {
		out[i] = billJSON{b.Project, b.DurationS, b.Hours(), b.Rate, b.Amount()}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
