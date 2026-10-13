package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"tock/internal/report"
)

// cmdInvoice writes a PDF invoice for one project's time at its rate.
func cmdInvoice(env Env, args []string) error {
	fs := newFlags("invoice")
	project := fs.String("project", "", "the project to bill (required; a unique prefix will do)")
	fromS := fs.String("from", "", "first day, YYYY-MM-DD (default: first day of last month)")
	toS := fs.String("to", "", "last day, YYYY-MM-DD (default: last day of last month)")
	client := fs.String("client", "", "who the invoice is to")
	number := fs.String("number", "", "invoice number (default YYYYMM-project)")
	out := fs.String("o", "", "the PDF file (default invoice-<number>.pdf)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("invoice: unexpected argument %q", fs.Arg(0))
	}
	if *project == "" {
		return usagef("invoice: --project is required")
	}
	now := env.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	from, to := first.AddDate(0, -1, 0), first.AddDate(0, 0, -1)
	for _, d := range []struct {
		s   string
		dst *time.Time
	}{{*fromS, &from}, {*toS, &to}} {
		if d.s == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", d.s, now.Location())
		if err != nil {
			return usagef("invoice: date %q, want YYYY-MM-DD", d.s)
		}
		*d.dst = t
	}
	if from.After(to) {
		return usagef("invoice: --from is after --to")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	name, err := a.Rules().ResolveProject(*project)
	if err != nil {
		return usagef("invoice: %v", err)
	}
	rate := a.Rules().Rate(name)
	if rate == nil {
		return fmt.Errorf("project %s has no rate: set rate = … on it in rules.toml", name)
	}
	rows, err := a.Export(from, to.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	lines := report.InvoiceLines(rows, name, *rate)
	if len(lines) == 0 {
		return fmt.Errorf("no time on %s from %s to %s", name, from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	cfg := a.Config().Invoice
	inv := report.Invoice{Number: *number, Project: name, Client: *client, From: cfg.Name, Address: cfg.Address,
		Currency: cfg.Currency, Note: cfg.Note, Issued: now, PeriodFrom: from, PeriodTo: to, Rate: *rate, Lines: lines}
	if inv.Number == "" {
		inv.Number = from.Format("200601") + "-" + slug(name)
	}
	path := *out
	if path == "" {
		path = "invoice-" + slug(inv.Number) + ".pdf"
	}
	var b bytes.Buffer
	if err := report.WriteInvoicePDF(&b, inv); err != nil {
		return err
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		return err
	}
	amount := fmt.Sprintf("%.2f", inv.Total())
	if inv.Currency != "" {
		amount += " " + inv.Currency
	}
	fmt.Fprintf(env.Stdout, "wrote %s: %s, %d days, %s h, %s\n", path, name, len(lines),
		fmt.Sprintf("%.2f", float64(inv.TotalSeconds())/3600), amount)
	return nil
}

// slug keeps letters, digits, - and _ for a file name; anything else is -.
func slug(s string) string {
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
