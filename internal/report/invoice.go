package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/pdf"
)

// Party is who an invoice is from or to.
type Party struct {
	Name    string
	Address []string // lines
	TaxID   string
}

// Invoice is what an invoice document shows. Amounts are minor units
// (ADR 0001); nothing here is computed again from live data.
type Invoice struct {
	Number, Project      string // Number "" on a draft
	Issuer, Client       Party
	Currency, Note       string
	Issued               time.Time
	PeriodFrom, PeriodTo time.Time // local midnights; PeriodTo is the last day
	MinorUnits           int
	RateMinor            int64
	Lines                []BillLine
	SecondsBilled        int64
	SubtotalMinor        int64
	TaxLabel, TaxNote    string
	TaxBP                int64 // 0 = no tax line
	TaxMinor, TotalMinor int64
	Supersedes           string // the number of the invoice this one replaces
	// Draft marks a preview: DRAFT on the page, and no number.
	Draft bool
}

// ItemOrigin tells what a view id from App.Export is: a span of this
// machine, away time, a calendar meeting, another machine's span or a
// timeline edit, with that row's id.
func ItemOrigin(id int64) (string, int64) {
	if id > 0 {
		return "span", id
	}
	if e, ok := classify.EditRowID(id); ok {
		return "edit", e
	}
	if r, ok := classify.RemoteRowID(id); ok {
		return "remote", r
	}
	if m, ok := classify.MeetingEventID(id); ok {
		return "meeting", m
	}
	return "away", -id
}

// FormatPercent writes basis points as a percentage: 2000 is "20", 550 is
// "5.5".
func FormatPercent(bp int64) string {
	s := FormatMinor(bp, 2)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// hours is seconds as decimal hours: "2.50".
func hours(s int64) string { return strconv.FormatFloat(math.Round(float64(s)/36)/100, 'f', 2, 64) }

func (inv Invoice) money(v int64) string {
	s := FormatMinor(v, inv.MinorUnits)
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
	if inv.Issuer.Name != "" {
		p.Text(invMargin, y, 15, true, inv.Issuer.Name)
		y += 16
	}
	for _, l := range inv.Issuer.Address {
		p.Text(invMargin, y, 10, false, l)
		y += 13
	}
	if inv.Issuer.TaxID != "" {
		p.Text(invMargin, y, 9, false, "Tax ID ")
		p.Text(invMargin+pdf.Width("Tax ID ", 9, false), y, 9, false, inv.Issuer.TaxID)
		y += 13
	}
	title, number := "INVOICE", inv.Number
	if inv.Draft {
		title = "DRAFT"
		if number == "" {
			number = "not issued"
		}
	}
	p.TextRight(invRight, invMargin+12, 24, true, title)
	ry := invMargin + 34.0
	for _, kv := range [][2]string{
		{"Number", number},
		{"Date", inv.Issued.Format("2006-01-02")},
		{"Period", inv.PeriodFrom.Format("2006-01-02") + " – " + inv.PeriodTo.Format("2006-01-02")},
	} {
		p.TextRight(invRight-150, ry, 9, false, kv[0])
		p.TextRight(invRight, ry, 10, true, kv[1])
		ry += 14
	}
	y = max(y, ry) + 24
	if inv.Client.Name != "" {
		p.Text(invMargin, y, 9, false, "BILL TO")
		p.Text(invMargin, y+15, 12, true, inv.Client.Name)
		y += 30
		for _, l := range inv.Client.Address {
			p.Text(invMargin, y, 10, false, l)
			y += 13
		}
		if inv.Client.TaxID != "" {
			p.Text(invMargin, y, 9, false, "Tax ID ")
			p.Text(invMargin+pdf.Width("Tax ID ", 9, false), y, 9, false, inv.Client.TaxID)
			y += 13
		}
		y += 10
	}
	if inv.Supersedes != "" {
		p.Text(invMargin, y, 9, false, "Replaces invoice "+inv.Supersedes)
		y += 18
	}
	rate := FormatMinor(inv.RateMinor, inv.MinorUnits)
	p.Text(invMargin, y, 11, false, "Project ")
	p.Text(invMargin+pdf.Width("Project ", 11, false), y, 11, true, inv.Project)
	p.TextRight(invRight, y, 11, false, inv.money(inv.RateMinor)+" / hour")
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
		day := l.Day
		if t, err := time.Parse("2006-01-02", l.Day); err == nil {
			day = t.Format("Mon 2006-01-02")
		}
		p.Text(invMargin+8, y, 10, false, day)
		p.TextRight(invRight-220, y, 10, false, hours(l.SecondsBilled))
		p.TextRight(invRight-110, y, 10, false, rate)
		p.TextRight(invRight-8, y, 10, false, inv.money(l.AmountMinor))
		p.Line(invMargin, y+6, invRight, y+6, 0.4, 0.85)
		y += invRowH
	}
	if y > invBottom-80 {
		p = d.AddPage()
		y = invMargin + 10
	}
	y += 8
	p.TextRight(invRight-110, y, 10, false, "Total hours")
	p.TextRight(invRight-8, y, 10, false, hours(inv.SecondsBilled))
	if inv.TaxBP > 0 {
		y += 16
		p.TextRight(invRight-110, y, 10, false, "Subtotal")
		p.TextRight(invRight-8, y, 10, false, inv.money(inv.SubtotalMinor))
		y += 16
		label := strings.TrimSpace(inv.TaxLabel + " " + FormatPercent(inv.TaxBP) + "%")
		p.TextRight(invRight-110, y, 10, false, label)
		p.TextRight(invRight-8, y, 10, false, inv.money(inv.TaxMinor))
	}
	y += 20
	p.Line(invRight-220, y-13, invRight, y-13, 0.8, 0.2)
	p.TextRight(invRight-110, y+2, 12, true, "Total")
	p.TextRight(invRight-8, y+2, 12, true, inv.money(inv.TotalMinor))
	ny := max(y+50, invBottom+8)
	for _, text := range []string{inv.TaxNote, inv.Note} {
		for _, l := range wrap(text, invRight-invMargin, 9) {
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

// Slug keeps letters, digits, - and _ for a file name; anything else is -.
func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "invoice"
	}
	return b.String()
}
