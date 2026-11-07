package app

import (
	"testing"
	"time"

	"tock/internal/apptest"
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
}
