package app

import (
	"fmt"
	"time"

	"tock/internal/report"
)

// InvoiceSpec is one invoice to make: the project (a unique prefix will
// do), the first and last day billed (both included, local midnight), who
// it is to ("" = the project's client) and its number ("" =
// YYYYMM-project).
type InvoiceSpec struct {
	Project        string
	From, To       time.Time
	Client, Number string
}

// Invoice bills a project's time from s.From to s.To at its rate, with the
// sender, address, currency and note of config [invoice], issued now.
func (a *App) Invoice(s InvoiceSpec, now time.Time) (report.Invoice, error) {
	if s.From.After(s.To) {
		return report.Invoice{}, fmt.Errorf("the first day is after the last")
	}
	name, err := a.rs.ResolveProject(s.Project)
	if err != nil {
		return report.Invoice{}, err
	}
	rate := a.rs.Rate(name)
	if rate == nil {
		return report.Invoice{}, fmt.Errorf("project %s has no rate: set one in Library, or rate = … on it in rules.toml", name)
	}
	if !a.rs.Billable(name) {
		return report.Invoice{}, fmt.Errorf("project %s is not billable: remove billable = false on it in rules.toml", name)
	}
	if s.Client == "" {
		s.Client = a.rs.Client(name)
	}
	rows, err := a.Export(s.From, s.To.AddDate(0, 0, 1))
	if err != nil {
		return report.Invoice{}, err
	}
	lines := report.InvoiceLines(rows, name, *rate)
	if len(lines) == 0 {
		return report.Invoice{}, fmt.Errorf("no time on %s from %s to %s", name, s.From.Format("2006-01-02"), s.To.Format("2006-01-02"))
	}
	cfg := a.Config().Invoice
	inv := report.Invoice{Number: s.Number, Project: name, Client: s.Client, From: cfg.Name, Address: cfg.Address,
		Currency: cfg.Currency, Note: cfg.Note, Issued: now, PeriodFrom: s.From, PeriodTo: s.To, Rate: *rate, Lines: lines}
	if inv.Number == "" {
		inv.Number = s.From.Format("200601") + "-" + report.Slug(name)
	}
	return inv, nil
}
