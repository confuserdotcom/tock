package app

import (
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/report"
)

func TestInvoice(t *testing.T) {
	e := apptest.New(t, apptest.Rules+"\n[[project]]\nname = \"Client\"\nrate = 50\n")
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	day := e.Clock
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	if _, err := a.Invoice(InvoiceSpec{Project: "tock", From: from, To: from}, day); err == nil {
		t.Fatal("invoiced a project without a rate")
	}
	if _, err := a.Invoice(InvoiceSpec{Project: "client", From: from, To: from}, day); err == nil {
		t.Fatal("invoiced a project with no time")
	}
	if err := a.EditProject(ProjectSpec{Name: "tock", Rate: func(v float64) *float64 { return &v }(60)}); err != nil {
		t.Fatal(err)
	}
	inv, err := a.Invoice(InvoiceSpec{Project: "TOCK", From: from, To: from}, day)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Project != "tock" || inv.Rate != 60 || inv.TotalSeconds() != 100*60 || inv.Number != from.Format("200601")+"-tock" {
		t.Fatalf("invoice %+v, %ds", inv, inv.TotalSeconds())
	}
	if _, err := a.Invoice(InvoiceSpec{Project: "tock", From: from.AddDate(0, 0, 1), To: from}, day); err == nil {
		t.Fatal("from after to")
	}

	// The project's client is used unless one is given.
	if err := a.EditProject(ProjectSpec{Name: "tock", Client: func(v string) *string { return &v }("Acme")}); err != nil {
		t.Fatal(err)
	}
	if inv, err := a.Invoice(InvoiceSpec{Project: "tock", From: from, To: from}, day); err != nil || inv.Client != "Acme" {
		t.Fatalf("project client: %q, %v", inv.Client, err)
	}
	if inv, err := a.Invoice(InvoiceSpec{Project: "tock", From: from, To: from, Client: "Other"}, day); err != nil || inv.Client != "Other" {
		t.Fatalf("given client: %q, %v", inv.Client, err)
	}
}

// AC6: billable = false refuses an invoice and drops the rate from export,
// while the rate itself stays in rules.toml.
func TestInvoiceNotBillable(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	day := e.Clock
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	no := false
	if err := a.EditProject(ProjectSpec{Name: "tock", Rate: func(v float64) *float64 { return &v }(60), Billable: &no}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoice(InvoiceSpec{Project: "tock", From: from, To: from}, day); err == nil || !strings.Contains(err.Error(), "billable") {
		t.Fatalf("invoiced a non-billable project: %v", err)
	}
	rows, err := a.Export(from, from.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, r := range rows {
		if r.Project == "tock" {
			seen = true
			if r.Rate != nil {
				t.Fatalf("non-billable row has rate %v", *r.Rate)
			}
		}
	}
	if !seen {
		t.Fatal("no tock rows to check")
	}
	for _, b := range report.Bill(rows) {
		if b.Project == "tock" && b.Amount() != nil {
			t.Fatalf("non-billable project billed %v", *b.Amount())
		}
	}
}
