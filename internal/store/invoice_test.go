package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"tock/internal/testfs"
)

var paris = func() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}()

func draftInvoice(issued time.Time) Invoice {
	return Invoice{
		IssuedMS:   issued.UnixMilli(),
		IssuerName: "Ada", IssuerAddress: []string{"1 Way", "London"}, IssuerTaxID: "GB1",
		ClientName: "Acme", ClientAddress: []string{"2 Road"}, ClientTaxID: "FR9",
		Project: "Acme site", PeriodFromMS: issued.AddDate(0, -1, 0).UnixMilli(), PeriodToMS: issued.UnixMilli(), TZ: "Europe/Paris",
		Currency: "EUR", MinorUnits: 2, RateMinor: 9550, RoundStepS: 900, RoundMode: "up",
		TaxLabel: "VAT", TaxBP: 2000, TaxNote: "",
		SecondsBilled: 12000, SubtotalMinor: 31833, TaxMinor: 6367, TotalMinor: 38200,
		Note: "Due in 30 days", Generator: "tock test",
		Lines: []InvoiceLine{{Day: "2026-10-05", SecondsRaw: 11900, SecondsBilled: 12000, AmountMinor: 31833}},
		Items: []InvoiceItem{{Origin: "span", OriginID: 7, StartMS: 1, EndMS: 11_900_001, Seconds: 11900,
			Category: "Code", Project: "Acme site", CategorySource: "rule", ProjectSource: "model"}},
	}
}

// fakePDF stands in for the renderer: its bytes depend on the number, so a
// test can tell the PDF was made for the allocated number.
func fakePDF(inv Invoice) ([]byte, error) {
	return []byte("%PDF fake " + inv.Number + " " + inv.ClientName), nil
}

func issue(t *testing.T, s *Store, sp IssueSpec) Invoice {
	t.Helper()
	if sp.Loc == nil {
		sp.Loc = paris
	}
	if sp.Render == nil {
		sp.Render = fakePDF
	}
	inv, err := s.IssueInvoice(sp)
	if err != nil {
		t.Fatalf("IssueInvoice: %v", err)
	}
	return inv
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestIssueInvoiceStoresSnapshot(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, paris)
	inv := issue(t, s, IssueSpec{Draft: draftInvoice(at)})
	if inv.Number != "2026-0001" || inv.Year != 2026 || inv.Seq != 1 || inv.Status != InvoiceIssued || inv.ID == 0 {
		t.Fatalf("issued %+v", inv)
	}
	if !bytes.Equal(inv.PDF, []byte("%PDF fake 2026-0001 Acme")) || inv.PDFSHA256 != sum(inv.PDF) || inv.JSONSHA256 != sum(inv.CanonicalJSON) {
		t.Fatalf("document %q %s %s", inv.PDF, inv.PDFSHA256, inv.JSONSHA256)
	}
	got, err := s.InvoiceByNumber("2026-0001")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, inv) {
		t.Fatalf("read back\n%+v\nwant\n%+v", got, inv)
	}
	// The canonical JSON carries every frozen field and the lines, and
	// round-trips to the same bytes.
	for _, want := range []string{`"number":"2026-0001"`, `"rate_minor":9550`, `"total_minor":38200`, `"day":"2026-10-05"`, `"pdf_sha256":"` + inv.PDFSHA256 + `"`} {
		if !strings.Contains(string(inv.CanonicalJSON), want) {
			t.Errorf("canonical JSON lacks %s:\n%s", want, inv.CanonicalJSON)
		}
	}
	if _, err := s.InvoiceByNumber("2026-0002"); !errors.Is(err, ErrNoInvoice) {
		t.Fatalf("missing invoice: %v", err)
	}
}

// No connection can change or delete an issued invoice.
func TestInvoiceTriggers(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	defer s.Close()
	inv := issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2026, 11, 3, 10, 0, 0, 0, paris))})
	for _, q := range []string{
		`UPDATE invoices SET total_minor = 0`,
		`UPDATE invoices SET client_name = 'Other'`,
		`UPDATE invoices SET pdf = x'00'`,
		`UPDATE invoices SET status = 'void'`, // no reason
		`UPDATE invoices SET status = 'void', void_reason = ' ', void_ms = 1`,
		`UPDATE invoices SET status = 'void', void_reason = 'x', void_ms = 1, total_minor = 0`,
		`DELETE FROM invoices`,
		`DELETE FROM invoice_items`,
		`DELETE FROM invoice_lines`,
		`UPDATE invoice_items SET seconds = 0`,
		`UPDATE invoice_lines SET amount_minor = 0`,
	} {
		if _, err := s.db.Exec(q); err == nil || !strings.Contains(err.Error(), "invoices:") {
			t.Errorf("%s: err %v", q, err)
		}
	}
	if _, err := s.VoidInvoice(inv.Number, "wrong client", 5); err != nil {
		t.Fatal(err)
	}
	// Void happens once: a second void, or un-voiding, is refused.
	for _, q := range []string{
		`UPDATE invoices SET void_reason = 'again', void_ms = 6`,
		`UPDATE invoices SET status = 'issued', void_reason = NULL, void_ms = NULL`,
	} {
		if _, err := s.db.Exec(q); err == nil {
			t.Errorf("%s allowed after void", q)
		}
	}
	if _, err := s.VoidInvoice(inv.Number, "again", 6); err == nil {
		t.Error("voided twice")
	}
	// A second, separate connection is bound by the same triggers.
	raw := openAt(t, path)
	defer raw.Close()
	if _, err := raw.db.Exec(`UPDATE invoices SET total_minor = 0`); err == nil {
		t.Error("second connection changed an invoice")
	}
}

func TestInvoiceVoidAndSupersede(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, paris)
	first := issue(t, s, IssueSpec{Draft: draftInvoice(at)})

	// The old invoice must be void, or voided in the same transaction.
	d := draftInvoice(at.Add(time.Hour))
	if _, err := s.IssueInvoice(IssueSpec{Draft: d, Loc: paris, Render: fakePDF, Supersedes: first.Number}); err == nil || !strings.Contains(err.Error(), "void") {
		t.Fatalf("superseded an issued invoice: %v", err)
	}
	if _, err := s.IssueInvoice(IssueSpec{Draft: d, Loc: paris, Render: fakePDF, Supersedes: "2026-0099", VoidOldReason: "x"}); !errors.Is(err, ErrNoInvoice) {
		t.Fatalf("superseded a missing invoice: %v", err)
	}
	second := issue(t, s, IssueSpec{Draft: d, Supersedes: first.Number, VoidOldReason: "wrong rate"})
	if second.Number != "2026-0002" || second.SupersedesID != first.ID || second.Supersedes != first.Number {
		t.Fatalf("second %+v", second)
	}
	old, _ := s.InvoiceByNumber(first.Number)
	if old.Status != InvoiceVoid || old.VoidReason != "wrong rate" || old.VoidMS != d.IssuedMS {
		t.Fatalf("old %+v", old)
	}
	// The old number can never be issued again.
	if _, err := s.IssueInvoice(IssueSpec{Draft: withNumber(d, first.Number), Loc: paris, Render: fakePDF}); err == nil || !strings.Contains(err.Error(), first.Number) {
		t.Fatalf("reused a number: %v", err)
	}
	// After a void the sequence goes on, it never fills the gap.
	if _, err := s.VoidInvoice(second.Number, "dup", d.IssuedMS+1); err != nil {
		t.Fatal(err)
	}
	if third := issue(t, s, IssueSpec{Draft: draftInvoice(at.Add(2 * time.Hour))}); third.Number != "2026-0003" {
		t.Fatalf("third %s", third.Number)
	}
	if _, err := s.VoidInvoice("2026-0003", "  ", 1); err == nil {
		t.Error("voided without a reason")
	}
	list, err := s.Invoices(2026)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, i := range list {
		got = append(got, i.Number+" "+i.Status+" "+i.Supersedes)
		if i.PDF != nil || i.Lines != nil {
			t.Error("list carries the document")
		}
	}
	if want := []string{"2026-0001 void ", "2026-0002 void 2026-0001", "2026-0003 issued "}; !slices.Equal(got, want) {
		t.Fatalf("list %q", got)
	}
	if l, _ := s.Invoices(2025); len(l) != 0 {
		t.Fatalf("2025: %v", l)
	}
}

func withNumber(d Invoice, n string) Invoice { d.Number = n; return d }

func TestInvoiceNumbers(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	// The year is the local issue date's: 23:30 UTC on 31 Dec is 2027 in
	// Paris.
	last := issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2026, 12, 31, 23, 59, 59, 0, paris))})
	first := issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC))})
	if last.Number != "2026-0001" || first.Number != "2027-0001" {
		t.Fatalf("rollover %s %s", last.Number, first.Number)
	}
	// A chosen number takes no sequence and does not move it.
	own := issue(t, s, IssueSpec{Draft: withNumber(draftInvoice(time.Date(2027, 2, 1, 9, 0, 0, 0, paris)), "ACME-7")})
	if own.Seq != 0 || own.Year != 2027 {
		t.Fatalf("own number %+v", own)
	}
	if next := issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2027, 2, 2, 9, 0, 0, 0, paris))}); next.Number != "2027-0002" {
		t.Fatalf("after own number: %s", next.Number)
	}
	// A chosen number in the default form would collide with the
	// sequence later.
	for _, n := range []string{"2027-0009", "ACME-7", ".hidden", "a/b", "a b"} {
		if _, err := s.IssueInvoice(IssueSpec{Draft: withNumber(draftInvoice(time.Date(2027, 2, 3, 9, 0, 0, 0, paris)), n), Loc: paris, Render: fakePDF}); err == nil {
			t.Errorf("number %q accepted", n)
		}
	}
}

// Two writers racing get distinct numbers, never an error or a
// duplicate. Each goroutine has its own Store, so its own connection.
func TestInvoiceNumberRace(t *testing.T) {
	path := dbPath(t)
	setup := openAt(t, path)
	setup.Close()
	const writers, each = 4, 5
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, writers*each)
	nums := make(chan string, writers*each)
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer s.Close()
			<-start
			for i := range each {
				inv, err := s.IssueInvoice(IssueSpec{Draft: draftInvoice(time.Date(2026, 5, 1, 9, w, i, 0, paris)), Loc: paris, Render: fakePDF})
				if err != nil {
					errs <- err
					continue
				}
				nums <- inv.Number
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(nums)
	for err := range errs {
		t.Error(err)
	}
	var got []string
	for n := range nums {
		got = append(got, n)
	}
	slices.Sort(got)
	var want []string
	for i := 1; i <= writers*each; i++ {
		want = append(want, fmt.Sprintf("2026-%04d", i))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("numbers %v", got)
	}
}

// A failure before commit stores nothing and the number is free
// again; a render failure does the same.
func TestInvoiceIssueFailureRollsBack(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, paris)
	first := issue(t, s, IssueSpec{Draft: draftInvoice(at)})

	boom := errors.New("boom")
	invoiceHook = func(stage string) error {
		if stage == "before-commit" {
			return boom
		}
		return nil
	}
	_, err := s.IssueInvoice(IssueSpec{Draft: draftInvoice(at.Add(time.Hour)), Loc: paris, Render: fakePDF, Supersedes: first.Number, VoidOldReason: "x"})
	invoiceHook = nil
	if !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	if _, err := s.IssueInvoice(IssueSpec{Draft: draftInvoice(at), Loc: paris, Render: func(Invoice) ([]byte, error) { return nil, boom }}); !errors.Is(err, boom) {
		t.Fatalf("render err %v", err)
	}
	for table, want := range map[string]int{"invoices": 1, "invoice_lines": 1, "invoice_items": 1} {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Errorf("%s rows %d, %v", table, n, err)
		}
	}
	if old, _ := s.InvoiceByNumber(first.Number); old.Status != InvoiceIssued {
		t.Error("the void of the old invoice survived the rollback")
	}
	if next := issue(t, s, IssueSpec{Draft: draftInvoice(at.Add(time.Hour))}); next.Number != "2026-0002" {
		t.Fatalf("number after rollback %s", next.Number)
	}
}

// Billing records hold no title or URL.
func TestInvoiceItemsHaveNoTitles(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	for _, table := range []string{"invoices", "invoice_lines", "invoice_items"} {
		rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var col string
			rows.Scan(&col)
			if strings.Contains(col, "title") || strings.Contains(col, "url") {
				t.Errorf("%s.%s", table, col)
			}
		}
		rows.Close()
	}
}

func TestInvoiceFilesRepairVerifyRecover(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	dir := InvoiceDir(path)
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, paris)
	a := issue(t, s, IssueSpec{Draft: draftInvoice(at)})
	if err := WriteInvoiceFiles(dir, a); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"2026-0001.json", "2026-0001.pdf"} {
		fi, err := os.Stat(filepath.Join(dir, f))
		if err != nil || !testfs.Perm(fi, 0o600) {
			t.Fatalf("%s: %v %v", f, fi, err)
		}
	}
	// Issued but the files were never written (crash after commit).
	b := issue(t, s, IssueSpec{Draft: draftInvoice(at.Add(time.Hour)), Supersedes: a.Number, VoidOldReason: "typo"})
	if checks, _ := s.VerifyInvoices(dir, ""); okAll(checks) {
		t.Fatal("missing files verified")
	}
	fixed, err := s.RepairInvoiceFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-0001.void.json", "2026-0002.json", "2026-0002.pdf"}; !slices.Equal(fixed, want) {
		t.Fatalf("repaired %v", fixed)
	}
	if pdf, _ := os.ReadFile(filepath.Join(dir, "2026-0002.pdf")); sum(pdf) != b.PDFSHA256 {
		t.Fatal("repaired PDF differs")
	}
	if checks, err := s.VerifyInvoices(dir, ""); err != nil || !okAll(checks) || len(checks) != 2 {
		t.Fatalf("verify %+v %v", checks, err)
	}
	// One changed byte on disk is found; repair never overwrites it.
	pdfPath := filepath.Join(dir, "2026-0002.pdf")
	pdf, _ := os.ReadFile(pdfPath)
	pdf[0] ^= 1
	os.WriteFile(pdfPath, pdf, 0o600)
	checks, _ := s.VerifyInvoices(dir, "2026-0002")
	if len(checks) != 1 || checks[0].OK() || !strings.Contains(strings.Join(checks[0].Problems, " "), "pdf") {
		t.Fatalf("altered pdf: %+v", checks)
	}
	if fixed, _ := s.RepairInvoiceFiles(dir); len(fixed) != 0 {
		t.Fatalf("repair overwrote %v", fixed)
	}
	if err := WriteInvoiceFiles(dir, b); err != nil {
		t.Fatal(err)
	}
	if checks, _ := s.VerifyInvoices(dir, ""); !okAll(checks) {
		t.Fatalf("after rewrite %+v", checks)
	}
	if _, err := s.VerifyInvoices(dir, "2026-0042"); !errors.Is(err, ErrNoInvoice) {
		t.Fatalf("verify missing: %v", err)
	}
	want, _ := s.Invoices(0)
	wantA, _ := s.InvoiceByNumber(a.Number)
	wantB, _ := s.InvoiceByNumber(b.Number)
	s.Close()

	// A database from before the invoices recovers identical rows
	// from the files.
	fresh := openAt(t, dbPath(t))
	defer fresh.Close()
	got, err := fresh.RecoverInvoices(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"2026-0001", "2026-0002"}) {
		t.Fatalf("recovered %v", got)
	}
	gotList, _ := fresh.Invoices(0)
	if !reflect.DeepEqual(gotList, want) {
		t.Fatalf("recovered list\n%+v\nwant\n%+v", gotList, want)
	}
	for _, w := range []Invoice{wantA, wantB} {
		g, err := fresh.InvoiceByNumber(w.Number)
		if err != nil || !reflect.DeepEqual(g, w) {
			t.Fatalf("recovered %s\n%+v\nwant\n%+v (%v)", w.Number, g, w, err)
		}
	}
	// Recover never overwrites a number it has.
	if again, err := fresh.RecoverInvoices(dir); err != nil || len(again) != 0 {
		t.Fatalf("second recover %v %v", again, err)
	}
	// A JSON whose PDF does not match is refused.
	other := t.TempDir()
	js, _ := os.ReadFile(filepath.Join(dir, "2026-0001.json"))
	os.WriteFile(filepath.Join(other, "2026-0001.json"), js, 0o600)
	os.WriteFile(filepath.Join(other, "2026-0001.pdf"), []byte("not it"), 0o600)
	empty := openAt(t, dbPath(t))
	defer empty.Close()
	if _, err := empty.RecoverInvoices(other); err == nil || !strings.Contains(err.Error(), "2026-0001") {
		t.Fatalf("bad pdf recovered: %v", err)
	}
	if l, _ := empty.Invoices(0); len(l) != 0 {
		t.Fatal("a refused file was imported")
	}
}

func okAll(cs []InvoiceCheck) bool {
	for _, c := range cs {
		if !c.OK() {
			return false
		}
	}
	return len(cs) > 0
}

// A snapshot keeps the rows and the triggers.
func TestInvoiceSnapshotKeepsTriggers(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	inv := issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2026, 11, 3, 10, 0, 0, 0, paris))})
	snap := filepath.Join(t.TempDir(), "snap.db")
	if err := s.BackupTo(snap); err != nil {
		t.Fatal(err)
	}
	c := openAt(t, snap)
	defer c.Close()
	if got, err := c.InvoiceByNumber(inv.Number); err != nil || !reflect.DeepEqual(got, inv) {
		t.Fatalf("snapshot row %+v %v", got, err)
	}
	if triggerSQL(t, s) != triggerSQL(t, c) || !strings.Contains(triggerSQL(t, c), "invoices_frozen") {
		t.Fatal("triggers differ in the snapshot")
	}
	if _, err := c.db.Exec(`DELETE FROM invoices`); err == nil {
		t.Fatal("snapshot lost its triggers")
	}
}

func triggerSQL(t *testing.T, s *Store) string {
	t.Helper()
	rows, err := s.db.Query(`SELECT name || ':' || sql FROM sqlite_master WHERE type = 'trigger' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var x string
		rows.Scan(&x)
		b.WriteString(x + "\n")
	}
	return b.String()
}

func TestInvoiceNumbersIn(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	issue(t, s, IssueSpec{Draft: draftInvoice(time.Date(2026, 11, 3, 10, 0, 0, 0, paris))})
	s.Close()
	got, err := InvoiceNumbersIn(path)
	if err != nil || !slices.Equal(got, []string{"2026-0001"}) {
		t.Fatalf("%v %v", got, err)
	}
	// A database from before the invoices has none, and no error.
	old := filepath.Join(t.TempDir(), "old.db")
	gunzipAny(t, old)
	if got, err := InvoiceNumbersIn(old); err != nil || len(got) != 0 {
		t.Fatalf("old db: %v %v", got, err)
	}
}

// gunzipAny unpacks the oldest upgrade fixture to path.
func gunzipAny(t *testing.T, path string) {
	t.Helper()
	fx, _ := filepath.Glob("testdata/upgrade/v*.db.gz")
	if len(fx) == 0 {
		t.Skip("no fixtures")
	}
	slices.Sort(fx)
	gunzip(t, fx[0], path)
}
