package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"tock/internal/pdf"
)

// InvoiceLine is one day of work on the invoiced project.
type InvoiceLine struct {
	Date    time.Time // local midnight
	Seconds int64
	Amount  float64 // rounded to cents
}

// Invoice is everything tock invoice prints.
type Invoice struct {
	Number, Project, Client string
	From                    string   // [invoice] name
	Address                 []string // [invoice] address
	Currency, Note          string
	Issued                  time.Time
	PeriodFrom, PeriodTo    time.Time // local midnights; PeriodTo is the last day
	Rate                    float64
	Lines                   []InvoiceLine
}

// TotalSeconds is the time of every line.
func (inv Invoice) TotalSeconds() int64 {
	var s int64
	for _, l := range inv.Lines {
		s += l.Seconds
	}
	return s
}

// Total is the sum of the line amounts, so the lines add up to it.
func (inv Invoice) Total() float64 {
	var t float64
	for _, l := range inv.Lines {
		t += l.Amount
	}
	return math.Round(t*100) / 100
}

// InvoiceLines groups the rows of project by the local day they start on,
// in date order, pricing each day at rate.
func InvoiceLines(rows []ExportRow, project string, rate float64) []InvoiceLine {
	var out []InvoiceLine
	idx := map[string]int{}
	for _, r := range rows {
		if r.Project != project {
			continue
		}
		day := time.Date(r.Start.Year(), r.Start.Month(), r.Start.Day(), 0, 0, 0, 0, r.Start.Location())
		k := day.Format("2006-01-02")
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, InvoiceLine{Date: day})
		}
		out[i].Seconds += r.DurationS()
	}
	for i := range out {
		out[i].Amount = math.Round(rate*float64(out[i].Seconds)/3600*100) / 100
	}
	// Rows come in start order, so days are already in order.
	return out
}

// hours is seconds as decimal hours: "2.50".
func hours(s int64) string { return strconv.FormatFloat(math.Round(float64(s)/36)/100, 'f', 2, 64) }

func (inv Invoice) money(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	if inv.Currency != "" {
		s += " " + inv.Currency
	}
	return s
}

// Invoice page layout, in points.
const (
	invMargin = 56.0
	invRight  = pdf.PageW - invMargin
	invBottom = pdf.PageH - 90
	invRowH   = 18.0
)

// WriteInvoicePDF lays the invoice out on A4 pages.
func WriteInvoicePDF(w io.Writer, inv Invoice) error {
	var d pdf.Doc
	p := d.AddPage()
	y := invMargin + 10
	if inv.From != "" {
		p.Text(invMargin, y, 15, true, inv.From)
		y += 16
	}
	for _, l := range inv.Address {
		p.Text(invMargin, y, 10, false, l)
		y += 13
	}
	p.TextRight(invRight, invMargin+12, 24, true, "INVOICE")
	ry := invMargin + 34.0
	for _, kv := range [][2]string{
		{"Number", inv.Number},
		{"Date", inv.Issued.Format("2006-01-02")},
		{"Period", inv.PeriodFrom.Format("2006-01-02") + " – " + inv.PeriodTo.Format("2006-01-02")},
	} {
		p.TextRight(invRight-150, ry, 9, false, kv[0])
		p.TextRight(invRight, ry, 10, true, kv[1])
		ry += 14
	}
	y = max(y, ry) + 24
	if inv.Client != "" {
		p.Text(invMargin, y, 9, false, "BILL TO")
		p.Text(invMargin, y+15, 12, true, inv.Client)
		y += 40
	}
	p.Text(invMargin, y, 11, false, "Project ")
	p.Text(invMargin+pdf.Width("Project ", 11, false), y, 11, true, inv.Project)
	p.TextRight(invRight, y, 11, false, inv.money(inv.Rate)+" / hour")
	y += 26

	header := func(y float64) float64 {
		p.Rect(invMargin, y-13, invRight-invMargin, invRowH, 0.93)
		p.Text(invMargin+8, y, 9, true, "DATE")
		p.TextRight(invRight-220, y, 9, true, "HOURS")
		p.TextRight(invRight-110, y, 9, true, "RATE")
		p.TextRight(invRight-8, y, 9, true, "AMOUNT")
		return y + invRowH + 4
	}
	y = header(y)
	for _, l := range inv.Lines {
		if y > invBottom {
			p = d.AddPage()
			y = header(invMargin + 10)
		}
		p.Text(invMargin+8, y, 10, false, l.Date.Format("Mon 2006-01-02"))
		p.TextRight(invRight-220, y, 10, false, hours(l.Seconds))
		p.TextRight(invRight-110, y, 10, false, strconv.FormatFloat(inv.Rate, 'f', 2, 64))
		p.TextRight(invRight-8, y, 10, false, inv.money(l.Amount))
		p.Line(invMargin, y+6, invRight, y+6, 0.4, 0.85)
		y += invRowH
	}
	if y > invBottom-40 {
		p = d.AddPage()
		y = invMargin + 10
	}
	y += 8
	p.TextRight(invRight-110, y, 10, false, "Total hours")
	p.TextRight(invRight-8, y, 10, false, hours(inv.TotalSeconds()))
	y += 20
	p.Line(invRight-220, y-13, invRight, y-13, 0.8, 0.2)
	p.TextRight(invRight-110, y+2, 12, true, "Total")
	p.TextRight(invRight-8, y+2, 12, true, inv.money(inv.Total()))
	if inv.Note != "" {
		ny := max(y+50, invBottom+20)
		for _, l := range wrap(inv.Note, invRight-invMargin, 9) {
			p.Text(invMargin, ny, 9, false, l)
			ny += 12
		}
	}
	if n := d.Pages(); n > 1 {
		// Page numbers go on last, once the count is known.
		for i := range n {
			d.Page(i).TextRight(invRight, pdf.PageH-36, 8, false, fmt.Sprintf("page %d of %d", i+1, n))
		}
	}
	_, err := d.WriteTo(w)
	return err
}

// wrap splits s into lines no wider than width at size.
func wrap(s string, width, size float64) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		next := strings.TrimSpace(cur + " " + word)
		if cur != "" && pdf.Width(next, size, false) > width {
			lines = append(lines, cur)
			next = word
		}
		cur = next
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
