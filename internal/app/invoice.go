package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

// InvoiceSpec is one invoice to preview or issue: the project (a unique
// prefix will do), the first and last day billed (both included, local
// midnights), who it is to ("" = the project's client), its number ("" =
// the next of the year), the invoice it replaces and, when that one is not
// void yet, the reason to void it now.
type InvoiceSpec struct {
	Project                   string
	From, To                  time.Time
	Client, Number            string
	Supersedes, VoidOldReason string
	// AllowOverlap issues even when an issued invoice of the project
	// covers some of the same days.
	AllowOverlap bool
	// Generator is the tock version stored with the invoice.
	Generator string
}

// InvoiceDraft is an invoice computed from today's labels and settings,
// not stored. Record is the row IssueInvoice would store (no number yet
// unless one was given), Doc what its PDF shows (marked DRAFT).
type InvoiceDraft struct {
	Record store.Invoice
	Doc    report.Invoice
	// ModelOnlyS is billed time whose project only the local model chose:
	// no rule, tag or edit says so. Worth a look before issuing.
	ModelOnlyS int64
	// Overlaps are issued invoices of the project over some of the same
	// days.
	Overlaps []string
}

// DraftInvoice computes an invoice without writing anything.
func (a *App) DraftInvoice(s InvoiceSpec) (InvoiceDraft, error) {
	if s.From.After(s.To) {
		return InvoiceDraft{}, errors.New("the first day is after the last")
	}
	if n := strings.TrimSpace(s.Number); n != "" {
		if _, _, ok := report.ParseInvoiceNumber(n); ok {
			return InvoiceDraft{}, fmt.Errorf("number %s is in the YYYY-NNNN form of the yearly sequence; leave --number out to get the next one", n)
		}
	}
	name, err := a.rs.ResolveProject(s.Project)
	if err != nil {
		return InvoiceDraft{}, err
	}
	rate := a.rs.Rate(name)
	if rate == nil {
		return InvoiceDraft{}, fmt.Errorf("project %s has no rate: set one in Library, or rate = … on it in rules.toml", name)
	}
	if !a.rs.Billable(name) {
		return InvoiceDraft{}, fmt.Errorf("project %s is not billable: remove billable = false on it in rules.toml", name)
	}
	cfg := a.Config().Invoice
	rateMinor, err := report.RateMinor(*rate, cfg.MinorUnits)
	if err != nil {
		return InvoiceDraft{}, fmt.Errorf("project %s: rate %v has more decimals than [invoice] minor_units = %d", name, *rate, cfg.MinorUnits)
	}
	mode, err := report.ParseRoundMode(cfg.RoundMode)
	if err != nil {
		return InvoiceDraft{}, err
	}
	loc := a.now().Location()
	end := s.To.AddDate(0, 0, 1)
	rows, err := a.Export(s.From, end)
	if err != nil {
		return InvoiceDraft{}, err
	}
	lines := report.BillLines(rows, name, loc, report.BillPolicy{StepS: cfg.RoundStepS, Mode: mode, RateMinor: rateMinor})
	if len(lines) == 0 {
		return InvoiceDraft{}, fmt.Errorf("no time on %s from %s to %s", name, s.From.Format("2006-01-02"), s.To.Format("2006-01-02"))
	}
	var d InvoiceDraft
	r := store.Invoice{Number: strings.TrimSpace(s.Number), IssuedMS: a.nowMS(),
		IssuerName: cfg.Name, IssuerAddress: cfg.Address, IssuerTaxID: cfg.TaxID,
		Project: name, PeriodFromMS: s.From.UnixMilli(), PeriodToMS: end.UnixMilli(), TZ: tzName(loc),
		Currency: cfg.Currency, MinorUnits: cfg.MinorUnits, RateMinor: rateMinor, RoundStepS: cfg.RoundStepS, RoundMode: string(mode),
		TaxLabel: cfg.TaxLabel, TaxBP: cfg.TaxBP, TaxNote: cfg.TaxNote, Note: cfg.Note, Generator: s.Generator}
	r.ClientName = strings.TrimSpace(s.Client)
	if r.ClientName == "" {
		r.ClientName = a.rs.Client(name)
	}
	if c, ok := cfg.Client(r.ClientName); ok && r.ClientName != "" {
		r.ClientName, r.ClientAddress, r.ClientTaxID = c.Name, c.Address, c.TaxID
	}
	for _, l := range lines {
		r.Lines = append(r.Lines, store.InvoiceLine{Day: l.Day, SecondsRaw: l.SecondsRaw, SecondsBilled: l.SecondsBilled, AmountMinor: l.AmountMinor})
	}
	for _, row := range rows {
		if row.Project != name || !row.End.After(row.Start) {
			continue
		}
		origin, id := report.ItemOrigin(row.ID)
		secs := (row.End.UnixMilli() - row.Start.UnixMilli()) / 1000
		r.Items = append(r.Items, store.InvoiceItem{Origin: origin, OriginID: id, StartMS: row.Start.UnixMilli(), EndMS: row.End.UnixMilli(),
			Seconds: secs, Category: row.Category, Project: row.Project, CategorySource: row.CategorySource, ProjectSource: row.ProjectSource})
		if row.ProjectSource == string(classify.SourceModel) {
			d.ModelOnlyS += secs
		}
	}
	r.SecondsBilled, r.SubtotalMinor = report.SumLines(lines)
	r.TaxMinor = report.TaxMinor(r.SubtotalMinor, r.TaxBP)
	r.TotalMinor = r.SubtotalMinor + r.TaxMinor
	d.Record = r
	d.Doc = invoiceDoc(r, loc)
	d.Doc.Draft = true
	d.Doc.Supersedes = strings.TrimSpace(s.Supersedes)

	issued, err := a.st.Invoices(0)
	if err != nil {
		return InvoiceDraft{}, err
	}
	for _, i := range issued {
		if i.Status == store.InvoiceIssued && i.Project == name && i.Number != d.Doc.Supersedes &&
			i.PeriodFromMS < r.PeriodToMS && r.PeriodFromMS < i.PeriodToMS {
			d.Overlaps = append(d.Overlaps, i.Number)
		}
	}
	return d, nil
}

// IssueInvoice stores the invoice DraftInvoice computes, renders its PDF
// once and writes its files. A file error after the invoice is stored is
// returned with the invoice: it is issued, and the next invoice command
// writes the files again.
func (a *App) IssueInvoice(s InvoiceSpec) (store.Invoice, error) {
	d, err := a.DraftInvoice(s)
	if err != nil {
		return store.Invoice{}, err
	}
	if len(d.Overlaps) > 0 && !s.AllowOverlap {
		return store.Invoice{}, fmt.Errorf("invoice %s already bills %s over some of these days; give --allow-overlap to bill them again",
			strings.Join(d.Overlaps, ", "), d.Record.Project)
	}
	loc := a.now().Location()
	inv, err := a.st.IssueInvoice(store.IssueSpec{Draft: d.Record, Loc: loc, Supersedes: s.Supersedes, VoidOldReason: s.VoidOldReason,
		Render: func(inv store.Invoice) ([]byte, error) {
			var b bytes.Buffer
			err := report.WriteInvoicePDF(&b, invoiceDoc(inv, loc))
			return b.Bytes(), err
		}})
	if err != nil {
		return store.Invoice{}, err
	}
	if err := store.WriteInvoiceFiles(a.invoiceDir(), inv); err != nil {
		return inv, err
	}
	_, err = a.st.RepairInvoiceFiles(a.invoiceDir())
	return inv, err
}

// invoiceDoc is what an invoice row shows on paper.
func invoiceDoc(r store.Invoice, loc *time.Location) report.Invoice {
	if l, err := time.LoadLocation(r.TZ); err == nil {
		loc = l
	}
	lines := make([]report.BillLine, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = report.BillLine{Day: l.Day, SecondsRaw: l.SecondsRaw, SecondsBilled: l.SecondsBilled, AmountMinor: l.AmountMinor}
	}
	return report.Invoice{Number: r.Number, Project: r.Project,
		Issuer:   report.Party{Name: r.IssuerName, Address: r.IssuerAddress, TaxID: r.IssuerTaxID},
		Client:   report.Party{Name: r.ClientName, Address: r.ClientAddress, TaxID: r.ClientTaxID},
		Currency: r.Currency, Note: r.Note, Issued: time.UnixMilli(r.IssuedMS).In(loc),
		PeriodFrom: time.UnixMilli(r.PeriodFromMS).In(loc), PeriodTo: time.UnixMilli(r.PeriodToMS).In(loc).AddDate(0, 0, -1),
		MinorUnits: r.MinorUnits, RateMinor: r.RateMinor, Lines: lines, SecondsBilled: r.SecondsBilled, SubtotalMinor: r.SubtotalMinor,
		TaxLabel: r.TaxLabel, TaxNote: r.TaxNote, TaxBP: r.TaxBP, TaxMinor: r.TaxMinor, TotalMinor: r.TotalMinor, Supersedes: r.Supersedes}
}

// DraftPDF is the PDF of a draft, marked DRAFT.
func DraftPDF(d InvoiceDraft) ([]byte, error) {
	var b bytes.Buffer
	err := report.WriteInvoicePDF(&b, d.Doc)
	return b.Bytes(), err
}

// tzName is the IANA name of loc. time.Local is named "Local", so it is
// looked up from $TZ or /etc/localtime; "Local" when neither tells.
func tzName(loc *time.Location) string {
	if n := loc.String(); n != "Local" {
		return n
	}
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if p, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(p, "zoneinfo/"); ok {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "Local"
}

func (a *App) invoiceDir() string { return store.InvoiceDir(a.paths.DB) }

// VoidInvoice voids an issued invoice with a reason and writes its void
// file.
func (a *App) VoidInvoice(number, reason string) (store.Invoice, error) {
	inv, err := a.st.VoidInvoice(number, reason, a.nowMS())
	if err != nil {
		return store.Invoice{}, err
	}
	_, err = a.st.RepairInvoiceFiles(a.invoiceDir())
	return inv, err
}

// Invoices lists the invoices of a year (0 = all).
func (a *App) Invoices(year int) ([]store.Invoice, error) { return a.st.Invoices(year) }

// InvoiceByNumber reads one invoice with its document.
func (a *App) InvoiceByNumber(number string) (store.Invoice, error) {
	return a.st.InvoiceByNumber(number)
}

// RepairInvoiceFiles writes missing invoice files from the database.
func (a *App) RepairInvoiceFiles() ([]string, error) { return a.st.RepairInvoiceFiles(a.invoiceDir()) }

// VerifyInvoices compares stored hashes with the stored documents and the
// files, for one invoice or ("") all.
func (a *App) VerifyInvoices(number string) ([]store.InvoiceCheck, error) {
	return a.st.VerifyInvoices(a.invoiceDir(), number)
}

// RecoverInvoices imports invoice files whose number the database lacks.
func (a *App) RecoverInvoices() ([]string, error) { return a.st.RecoverInvoices(a.invoiceDir()) }

// InvoiceDayDrift is one day whose billed time differs between an issued
// invoice and today's labels.
type InvoiceDayDrift struct {
	Day           string
	IssuedS, NowS int64
}

// InvoiceCheck compares an issued invoice with its period billed again now:
// current labels, the stored rounding and rate.
type InvoiceCheck struct {
	Invoice       store.Invoice
	IssuedS, NowS int64
	Days          []InvoiceDayDrift
}

// Drift reports whether today's labels bill other time than the invoice.
func (c InvoiceCheck) Drift() bool { return len(c.Days) > 0 }

// CheckInvoice bills an issued invoice's period again with today's labels.
// The project is followed through renames.
func (a *App) CheckInvoice(number string) (InvoiceCheck, error) {
	inv, err := a.st.InvoiceByNumber(number)
	if err != nil {
		return InvoiceCheck{}, err
	}
	loc := a.now().Location()
	if l, err := time.LoadLocation(inv.TZ); err == nil {
		loc = l
	}
	name := a.rs.CurrentName(classify.NameProject, inv.Project)
	if name == "" {
		name = inv.Project
	}
	rows, err := a.Export(time.UnixMilli(inv.PeriodFromMS), time.UnixMilli(inv.PeriodToMS))
	if err != nil {
		return InvoiceCheck{}, err
	}
	mode, _ := report.ParseRoundMode(inv.RoundMode)
	now := report.BillLines(rows, name, loc, report.BillPolicy{StepS: inv.RoundStepS, Mode: mode, RateMinor: inv.RateMinor})
	c := InvoiceCheck{Invoice: inv}
	byDay := map[string]*InvoiceDayDrift{}
	var days []string
	get := func(day string) *InvoiceDayDrift {
		if d, ok := byDay[day]; ok {
			return d
		}
		byDay[day] = &InvoiceDayDrift{Day: day}
		days = append(days, day)
		return byDay[day]
	}
	for _, l := range inv.Lines {
		get(l.Day).IssuedS = l.SecondsBilled
		c.IssuedS += l.SecondsBilled
	}
	for _, l := range now {
		get(l.Day).NowS = l.SecondsBilled
		c.NowS += l.SecondsBilled
	}
	slices.Sort(days)
	for _, day := range days {
		if d := byDay[day]; d.IssuedS != d.NowS {
			c.Days = append(c.Days, *d)
		}
	}
	return c, nil
}
