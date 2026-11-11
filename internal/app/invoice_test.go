package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

const billRules = apptest.Rules + `
[[project]]
name = "Acme"
rate = 95.5
client = "Acme Corp"
`

const billConfig = `[invoice]
name = "Ada"
address = ["1 Way"]
tax_id = "GB1"
round = "15m"
round_mode = "up"
tax_label = "VAT"
tax_rate = "20"

[[invoice.client]]
name = "Acme Corp"
address = ["2 Road", "Paris"]
tax_id = "FR9"
`

// billEnv bills span 1 (09:00-10:40, 100 min) to Acme.
func billEnv(t *testing.T) (*apptest.Env, *App, time.Time) {
	t.Helper()
	e := apptest.New(t, strings.Replace(billRules, "title = '^tock │'\nproject = \"tock\"", "title = '^tock │'\nproject = \"Acme\"", 1))
	e.WriteConfig(t, billConfig)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	day := time.Date(e.Clock.Year(), e.Clock.Month(), e.Clock.Day(), 0, 0, 0, 0, e.Clock.Location())
	return e, a, day
}

func countInvoices(t *testing.T, a *App) int {
	t.Helper()
	l, err := a.Invoices(0)
	if err != nil {
		t.Fatal(err)
	}
	return len(l)
}

func TestInvoiceDraft(t *testing.T) {
	_, a, day := billEnv(t)
	d, err := a.DraftInvoice(InvoiceSpec{Project: "acme", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	r := d.Record
	// 100 min rounds up to 105 min: 6300 s × 9550 / 3600 = 16712.5 → 16713.
	if r.Project != "Acme" || r.SecondsBilled != 6300 || r.SubtotalMinor != 16713 || r.TaxMinor != 3343 || r.TotalMinor != 20056 ||
		r.RateMinor != 9550 || r.RoundStepS != 900 || r.RoundMode != "up" || r.TaxBP != 2000 || r.Number != "" {
		t.Fatalf("draft %+v", r)
	}
	if r.ClientName != "Acme Corp" || !reflect.DeepEqual(r.ClientAddress, []string{"2 Road", "Paris"}) || r.ClientTaxID != "FR9" ||
		r.IssuerName != "Ada" || r.IssuerTaxID != "GB1" {
		t.Fatalf("parties %+v", r)
	}
	if len(r.Lines) != 1 || r.Lines[0].Day != day.Format("2006-01-02") || len(r.Items) != 1 || r.Items[0].Origin != "span" || r.Items[0].OriginID != 1 {
		t.Fatalf("lines %+v items %+v", r.Lines, r.Items)
	}
	if !d.Doc.Draft || d.Doc.TotalMinor != 20056 {
		t.Fatalf("doc %+v", d.Doc)
	}
	// AC9: a preview writes nothing.
	if countInvoices(t, a) != 0 {
		t.Fatal("draft stored an invoice")
	}
	if _, err := os.Stat(store.InvoiceDir(a.paths.DB)); !os.IsNotExist(err) {
		t.Fatal("draft wrote files")
	}
}

// AC1, AC7, AC13: an issued invoice does not move when labels, rates and
// config change; check shows the drift.
func TestInvoiceIssueAndCheck(t *testing.T) {
	e, a, day := billEnv(t)
	inv, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day, Generator: "tock test"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Number != "2026-0001" || inv.TotalMinor != 20056 || inv.Generator != "tock test" || inv.TZ == "" {
		t.Fatalf("issued %+v", inv)
	}
	dir := store.InvoiceDir(a.paths.DB)
	if checks, err := a.VerifyInvoices(""); err != nil || len(checks) != 1 || !checks[0].OK() {
		t.Fatalf("files after issue: %+v %v", checks, err)
	}
	c, err := a.CheckInvoice(inv.Number)
	if err != nil || c.Drift() {
		t.Fatalf("check before changes: %+v %v", c, err)
	}

	// Relabel span 2 (34 min) to Acme, raise the rate, change the address
	// and the tax.
	if err := a.Tag([]int64{2}, classify.Want{Project: "Acme"}); err != nil {
		t.Fatal(err)
	}
	rate := 200.0
	if err := a.EditProject(ProjectSpec{Name: "Acme", Rate: &rate}); err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, strings.Replace(strings.Replace(billConfig, "1 Way", "9 Elsewhere", 1), `tax_rate = "20"`, `tax_rate = "5"`, 1))
	a = reopen(t, a, e)

	got, err := a.InvoiceByNumber(inv.Number)
	if err != nil || !reflect.DeepEqual(got, inv) {
		t.Fatalf("issued invoice moved:\n%+v\nwant\n%+v (%v)", got, inv, err)
	}
	if pdf, _ := os.ReadFile(filepath.Join(dir, inv.Number+".pdf")); string(pdf) != string(inv.PDF) {
		t.Fatal("pdf file changed")
	}
	c, err = a.CheckInvoice(inv.Number)
	if err != nil || !c.Drift() {
		t.Fatalf("check after relabel: %+v %v", c, err)
	}
	// 134 min now rounds up to 135; the stored rate still prices it.
	if len(c.Days) != 1 || c.Days[0].Day != day.Format("2006-01-02") || c.Days[0].IssuedS != 6300 || c.Days[0].NowS != 8100 ||
		c.IssuedS != 6300 || c.NowS != 8100 {
		t.Fatalf("drift %+v", c)
	}
}

func reopen(t *testing.T, a *App, e *apptest.Env) *App {
	t.Helper()
	a.Close()
	return open(t, e)
}

func TestInvoiceOverlapAndSupersede(t *testing.T) {
	_, a, day := billEnv(t)
	first, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day}); err == nil ||
		!strings.Contains(err.Error(), first.Number) || !strings.Contains(err.Error(), "--allow-overlap") {
		t.Fatalf("overlap: %v", err)
	}
	d, err := a.DraftInvoice(InvoiceSpec{Project: "Acme", From: day, To: day})
	if err != nil || len(d.Overlaps) != 1 || d.Overlaps[0] != first.Number {
		t.Fatalf("draft overlaps %+v %v", d.Overlaps, err)
	}
	// Superseding the overlapping invoice is not an overlap.
	second, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day, Supersedes: first.Number, VoidOldReason: "wrong client"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Supersedes != first.Number {
		t.Fatalf("second %+v", second)
	}
	old, _ := a.InvoiceByNumber(first.Number)
	if old.Status != store.InvoiceVoid {
		t.Fatalf("old %+v", old)
	}
	if _, err := os.Stat(filepath.Join(store.InvoiceDir(a.paths.DB), first.Number+".void.json")); err != nil {
		t.Fatal("void file not written:", err)
	}
	if v, err := a.VoidInvoice(second.Number, "client cancelled"); err != nil || v.Status != store.InvoiceVoid {
		t.Fatalf("void %+v %v", v, err)
	}
	// Void time is not billed: a new invoice over the same days is fine.
	if _, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day}); err != nil {
		t.Fatal(err)
	}
}

// AC6 and the old checks: no rate, not billable, no time, bad dates.
func TestInvoiceRefusals(t *testing.T) {
	e, a, day := billEnv(t)
	for _, c := range []struct {
		spec InvoiceSpec
		want string
	}{
		{InvoiceSpec{Project: "tock", From: day, To: day}, "no rate"},
		{InvoiceSpec{Project: "Acme", From: day.AddDate(0, 0, -3), To: day.AddDate(0, 0, -2)}, "no time"},
		{InvoiceSpec{Project: "Acme", From: day.AddDate(0, 0, 1), To: day}, "after"},
		{InvoiceSpec{Project: "Acme", From: day, To: day, Number: "2026-0001"}, "YYYY-NNNN"},
	} {
		if _, err := a.DraftInvoice(c.spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want %q", c.spec, err, c.want)
		}
	}
	no := false
	if err := a.EditProject(ProjectSpec{Name: "Acme", Billable: &no}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DraftInvoice(InvoiceSpec{Project: "Acme", From: day, To: day}); err == nil || !strings.Contains(err.Error(), "billable") {
		t.Fatalf("not billable: %v", err)
	}
	rows, err := a.Export(day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range report.Bill(rows) {
		if b.Project == "Acme" && b.Amount() != nil {
			t.Fatal("non-billable project priced in export")
		}
	}
	// FR19: a rate finer than minor_units is refused at issue.
	yes, fine := true, 95.555
	if err := a.EditProject(ProjectSpec{Name: "Acme", Billable: &yes, Rate: &fine}); err != nil {
		t.Fatal(err)
	}
	a = reopen(t, a, e)
	if _, err := a.DraftInvoice(InvoiceSpec{Project: "Acme", From: day, To: day}); err == nil || !strings.Contains(err.Error(), "decimals") {
		t.Fatalf("fine rate: %v", err)
	}
	// The project's client is used unless one is given.
	two := 95.5
	if err := a.EditProject(ProjectSpec{Name: "Acme", Rate: &two}); err != nil {
		t.Fatal(err)
	}
	if d, err := a.DraftInvoice(InvoiceSpec{Project: "Acme", From: day, To: day, Client: "Other"}); err != nil || d.Record.ClientName != "Other" || d.Record.ClientAddress != nil {
		t.Fatalf("given client: %+v %v", d.Record, err)
	}
}

func TestInvoiceRecover(t *testing.T) {
	_, a, day := billEnv(t)
	inv, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	// Files lost: any invoice command puts them back.
	os.Remove(filepath.Join(store.InvoiceDir(a.paths.DB), inv.Number+".json"))
	fixed, err := a.RepairInvoiceFiles()
	if err != nil || len(fixed) != 1 {
		t.Fatalf("repair %v %v", fixed, err)
	}
	if got, err := a.RecoverInvoices(); err != nil || len(got) != 0 {
		t.Fatalf("recover with all present: %v %v", got, err)
	}
}

// FR8: billed time is known per view; a relabel names the invoice.
func TestInvoiceBilledMarkers(t *testing.T) {
	_, a, day := billEnv(t)
	inv, err := a.IssueInvoice(InvoiceSpec{Project: "Acme", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	from, to := day.UnixMilli(), day.AddDate(0, 0, 1).UnixMilli()
	b, err := a.Billed(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if b.Number(1) != inv.Number || b.Number(2) != "" || b.MS(from, to) != 100*60_000 {
		t.Fatalf("billed: %q %q %d", b.Number(1), b.Number(2), b.MS(from, to))
	}
	if got, err := a.BilledOn([]int64{2, 1}); err != nil || !reflect.DeepEqual(got, []string{inv.Number}) {
		t.Fatalf("billed on %v %v", got, err)
	}
	if n := BilledNote([]string{inv.Number}); n != "On invoice 2026-0001; the invoice stays as issued." {
		t.Fatal(n)
	}
	if n := BilledNote([]string{"a", "b", "c"}); n != "On invoices a, b and c; the invoices stay as issued." {
		t.Fatal(n)
	}
	// Voided: no longer billed.
	if _, err := a.VoidInvoice(inv.Number, "wrong"); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.BilledOn([]int64{1}); len(got) != 0 {
		t.Fatalf("void still billed: %v", got)
	}
}
