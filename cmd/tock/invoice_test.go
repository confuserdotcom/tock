package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/store"
)

const rateRules = "name = \"tock\"\nrate = 60\n"

const invoiceConfig = "[invoice]\nname = \"Ada Lovelace\"\naddress = [\"London\"]\ncurrency = \"EUR\"\nnote = \"Due in 30 days.\"\n"

func invoiceEnv(t *testing.T) *tenv {
	t.Helper()
	te := newTestEnv(t, strings.Replace(testRules, "name = \"tock\"\n", rateRules, 1))
	te.seed(t, daySpans()...)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte(invoiceConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return te
}

var day = []string{"--project", "to", "--from", "2026-10-10", "--to", "2026-10-10"}

func TestInvoicePreview(t *testing.T) {
	te := invoiceEnv(t)
	path := filepath.Join(t.TempDir(), "inv.pdf")
	code, out := te.run(t, "", append(append([]string{"invoice"}, day...), "--client", "Babbage & Co", "-o", path)...)
	if code != 0 || !strings.Contains(out, "tock, 2026-10-10 to 2026-10-10: 1 days, 1.67 h, total 100.00 EUR\n") ||
		!strings.Contains(out, "to Babbage & Co\n") || !strings.Contains(out, "draft: "+path+" (nothing stored") {
		t.Fatalf("code %d out %q err %s", code, out, te.errb)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%PDF-1.4", "(DRAFT) Tj", "(Ada Lovelace) Tj", "(Babbage & Co) Tj", "(Sat 2026-10-10) Tj",
		"(1.67) Tj", "(100.00 EUR) Tj", "(Due in 30 days.) Tj"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("pdf lacks %q", want)
		}
	}
	// AC9: nothing stored.
	if code, out := te.run(t, "", "invoice", "list"); code != 0 || out != "no invoices issued yet\n" {
		t.Fatalf("list after preview: %d %q", code, out)
	}
}

func TestInvoiceLifecycle(t *testing.T) {
	te := invoiceEnv(t)
	code, out := te.run(t, "", append([]string{"invoice", "issue"}, day...)...)
	dir := store.InvoiceDir(te.dbPath())
	if code != 0 || !strings.Contains(out, "issued 2026-0001: 100.00 EUR\n") || !strings.Contains(out, filepath.Join(dir, "2026-0001.pdf")) {
		t.Fatalf("issue: %d %q %s", code, out, te.errb)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "2026-0001.pdf"))
	if err != nil || !strings.Contains(string(stored), "(2026-0001) Tj") || strings.Contains(string(stored), "DRAFT") {
		t.Fatalf("stored pdf: %v", err)
	}
	code, out = te.run(t, "", "invoice", "show", "2026-0001", "--json")
	if code != 0 || !strings.Contains(out, `"number":"2026-0001"`) || !strings.Contains(out, `"total_minor":10000`) || !strings.HasSuffix(out, "{\"status\":\"issued\"}\n") {
		t.Fatalf("show --json: %d %q", code, out)
	}
	shown := out
	code, out = te.run(t, "", "invoice", "show", "2026-0001")
	for _, want := range []string{"2026-0001  issued ", "tock, 2026-10-10 to 2026-10-10", "from Ada Lovelace",
		"2026-10-10  1:40", "total 100.00 EUR", "rate 60.00 EUR/h"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("show lacks %q: %d\n%s", want, code, out)
		}
	}
	if code, out := te.run(t, "", "invoice", "show", "2026-0001", "--pdf"); code != 0 || out != string(stored) {
		t.Fatalf("show --pdf: %d", code)
	}
	if code, out := te.run(t, "", "invoice", "check", "2026-0001"); code != 0 || !strings.Contains(out, "issued 1.67 h / now 1.67 h") {
		t.Fatalf("check: %d %q", code, out)
	}

	// AC1 and AC7: label span 2 (ATLAS, 34 min) as tock and change the
	// rate; the invoice stays, check finds the day.
	rules := strings.Replace(testRules, "name = \"tock\"\n", "name = \"tock\"\nrate = 90\n", 1) +
		"\n[[rule]]\nbundle = [\"term\"]\ntitle = '^ATLAS'\nproject = \"tock\"\n"
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "rules.toml"), []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, again := te.run(t, "", "invoice", "show", "2026-0001", "--json"); code != 0 || again != shown {
		t.Fatalf("show changed after relabel: %q", again)
	}
	code, out = te.run(t, "", "invoice", "check", "2026-0001")
	if code != 3 || !strings.Contains(out, "issued 1.67 h / now 2.23 h") || !strings.Contains(out, "2026-10-10  issued 1.67 h / now 2.23 h") {
		t.Fatalf("check after relabel: %d %q", code, out)
	}
	if h := sha256.Sum256(stored); te.mustShowPDF(t) != h {
		t.Fatal("stored pdf changed")
	}

	// An overlapping invoice needs --allow-overlap; a replacement does not.
	if code, _ := te.run(t, "", append([]string{"invoice", "issue"}, day...)...); code != 1 || !strings.Contains(te.errb.String(), "--allow-overlap") {
		t.Fatalf("overlap: %d %s", code, te.errb)
	}
	if code, _ := te.run(t, "", append([]string{"invoice", "issue", "--supersedes", "2026-0001"}, day...)...); code != 1 || !strings.Contains(te.errb.String(), "not void") {
		t.Fatalf("supersede issued: %d %s", code, te.errb)
	}
	code, out = te.run(t, "", append([]string{"invoice", "issue", "--supersedes", "2026-0001", "--void-old", "--reason", "relabelled"}, day...)...)
	if code != 0 || !strings.Contains(out, "issued 2026-0002: 201.00 EUR") || !strings.Contains(out, "replaces 2026-0001, now void") {
		t.Fatalf("supersede: %d %q %s", code, out, te.errb)
	}
	code, out = te.run(t, "", "invoice", "list")
	for _, want := range []string{"2026-0001  void  ", "void: relabelled", "replaced by 2026-0002", "2026-0002  issued", "replaces 2026-0001"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if code, _ := te.run(t, "", "invoice", "void", "2026-0001", "--reason", "again"); code != 1 || !strings.Contains(te.errb.String(), "already void") {
		t.Fatalf("void twice: %d %s", code, te.errb)
	}
	if code, out := te.run(t, "", "invoice", "void", "2026-0002", "--reason", "client left"); code != 0 || !strings.Contains(out, "voided 2026-0002") {
		t.Fatalf("void: %d %q %s", code, out, te.errb)
	}

	// AC15: verify finds one changed byte; writing the stored PDF back fixes it.
	pdfPath := filepath.Join(dir, "2026-0001.pdf")
	b, _ := os.ReadFile(pdfPath)
	b[10] ^= 1
	os.WriteFile(pdfPath, b, 0o600)
	if code, out := te.run(t, "", "invoice", "verify"); code != 4 || !strings.Contains(out, "2026-0001  pdf file differs") || !strings.Contains(out, "2026-0002  ok") {
		t.Fatalf("verify altered: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "invoice", "show", "2026-0001", "--pdf", "-o", pdfPath); code != 0 {
		t.Fatal(te.errb.String())
	}
	if code, out := te.run(t, "", "invoice", "verify", "2026-0001"); code != 0 || out != "2026-0001  ok\n" {
		t.Fatalf("verify fixed: %d %q", code, out)
	}

	// AC11 (second half): lost files come back on the next invoice command.
	os.Remove(filepath.Join(dir, "2026-0002.json"))
	if code, _ := te.run(t, "", "invoice", "list"); code != 0 || !strings.Contains(te.errb.String(), "rewrote 1 invoice file(s)") {
		t.Fatalf("repair: %d %s", code, te.errb)
	}
	if code, out := te.run(t, "", "invoice", "recover"); code != 0 || !strings.Contains(out, "nothing to recover") {
		t.Fatalf("recover: %d %q", code, out)
	}
}

func (te *tenv) mustShowPDF(t *testing.T) [32]byte {
	t.Helper()
	code, out := te.run(t, "", "invoice", "show", "2026-0001", "--pdf")
	if code != 0 {
		t.Fatal(te.errb.String())
	}
	return sha256.Sum256([]byte(out))
}

func TestInvoiceErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		code int
		err  string
	}{
		{[]string{"invoice"}, 2, "--project is required"},
		{[]string{"invoice", "--project", "nope"}, 1, "nope"},
		{[]string{"invoice", "--project", "tock", "--from", "2026-10-01", "--to", "2026-10-31"}, 1, "has no rate"},
		{[]string{"invoice", "--project", "tock", "--from", "2026-11-02", "--to", "2026-11-01"}, 2, "--from is after --to"},
		{[]string{"invoice", "bogus"}, 2, "invoice: one of"},
		{[]string{"invoice", "show", "2026-0009"}, 1, "no such invoice"},
		{[]string{"invoice", "void", "2026-0009"}, 2, "--reason"},
		{[]string{"invoice", "issue", "--project", "tock", "--void-old"}, 2, "--void-old and --reason"},
	} {
		code, _ := te.run(t, "", append(c.args, "-o", filepath.Join(dir, "x.pdf"))...)
		if len(c.args) > 1 && (c.args[1] == "show" || c.args[1] == "void" || c.args[1] == "bogus") {
			code, _ = te.run(t, "", c.args...)
		}
		if code != c.code || !strings.Contains(te.errb.String(), c.err) {
			t.Errorf("%v: code %d err %q, want %d %q", c.args, code, te.errb, c.code, c.err)
		}
	}
	te2 := invoiceEnv(t)
	if code, _ := te2.run(t, "", "invoice", "--project", "tock", "-o", filepath.Join(dir, "y.pdf")); code != 1 ||
		!strings.Contains(te2.errb.String(), "no time on tock from 2026-09-01 to 2026-09-30") {
		t.Errorf("empty month: code %d err %q", code, te2.errb)
	}
}

// FR19: a rate finer than [invoice] minor_units fails tock rules check.
func TestRulesCheckRateDecimals(t *testing.T) {
	te := newTestEnv(t, strings.Replace(testRules, "name = \"tock\"\n", "name = \"tock\"\nrate = 95.555\n", 1))
	if code, _ := te.run(t, "", "rules", "check"); code != 1 || !strings.Contains(te.errb.String(), `project "tock": rate 95.555 has more decimals than [invoice] minor_units = 2`) {
		t.Fatalf("code %d err %s", code, te.errb)
	}
	os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[invoice]\nminor_units = 3\n"), 0o600)
	if code, _ := te.run(t, "", "rules", "check"); code != 0 {
		t.Fatalf("minor_units 3: code %d err %s", code, te.errb)
	}
}

// FR16: doctor puts missing invoice files back and reports altered ones.
func TestDoctorInvoices(t *testing.T) {
	te := invoiceEnv(t)
	if code, _ := te.run(t, "", append([]string{"invoice", "issue"}, day...)...); code != 0 {
		t.Fatal(te.errb.String())
	}
	dir := store.InvoiceDir(te.dbPath())
	os.Remove(filepath.Join(dir, "2026-0001.pdf"))
	_, out := te.run(t, "", "doctor")
	if !strings.Contains(out, "1 invoice, files match (rewrote 2026-0001.pdf)") {
		t.Fatalf("doctor:\n%s", out)
	}
	os.WriteFile(filepath.Join(dir, "2026-0001.json"), []byte("{}"), 0o600)
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "2026-0001 differ from their record") {
		t.Fatalf("doctor altered:\n%s", out)
	}
}

// AC6: with Billing off nothing is issued; issued invoices stay readable.
func TestInvoiceBillingOff(t *testing.T) {
	te := invoiceEnv(t)
	if code, _ := te.run(t, "", append([]string{"invoice", "issue"}, day...)...); code != 0 {
		t.Fatalf("issue: %d %s", code, te.errb)
	}
	cfg := filepath.Join(te.ConfigDir, "config.toml")
	if err := os.WriteFile(cfg, []byte(invoiceConfig+"\n[billing]\nenabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	te.errb.Reset()
	if code, _ := te.run(t, "", append([]string{"invoice", "issue", "--allow-overlap"}, day...)...); code == 0 || !strings.Contains(te.errb.String(), "tock choice billing on") {
		t.Fatalf("issue with billing off: %d %s", code, te.errb)
	}
	if code, out := te.run(t, "", "invoice", "list"); code != 0 || !strings.Contains(out, "2026-0001") {
		t.Fatalf("list with billing off: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "invoice", "check", "2026-0001"); code != 0 {
		t.Fatalf("check with billing off: %d %s", code, te.errb)
	}
}

// FR8: relabelling billed time says the invoice stays as issued.
func TestInvoiceRelabelNote(t *testing.T) {
	te := invoiceEnv(t)
	if code, _ := te.run(t, "", append([]string{"invoice", "issue"}, day...)...); code != 0 {
		t.Fatalf("issue: %d %s", code, te.errb)
	}
	code, out := te.run(t, "", "edit", "label", "09:00-09:30", "Code")
	if code != 0 || !strings.Contains(out, "On invoice 2026-0001; the invoice stays as issued.") {
		t.Fatalf("edit billed: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "edit", "label", "13:00-13:10", "Code"); code != 0 || strings.Contains(out, "invoice") {
		t.Fatalf("edit unbilled: %d %q", code, out)
	}
}

// An issued invoice never changes, so one without the issuer's name would
// have to be voided: issue refuses it, and the preview says so.
func TestInvoiceIssueNeedsIssuerName(t *testing.T) {
	te := invoiceEnv(t)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[invoice]\ncurrency = \"EUR\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", append([]string{"invoice"}, day...)...)
	if code != 0 || !strings.Contains(out, "your name is empty") {
		t.Fatalf("preview: %d %q %s", code, out, te.errb)
	}
	code, _ = te.run(t, "", append([]string{"invoice", "issue"}, day...)...)
	if code != 1 || !strings.Contains(te.errb.String(), "[invoice] name") {
		t.Fatalf("issue without a name: %d %s", code, te.errb)
	}
	if _, out := te.run(t, "", "invoice", "list"); out != "no invoices issued yet\n" {
		t.Fatalf("issued anyway: %q", out)
	}
}

// check must not print equal hours for a drift it reports: below a
// hundredth of an hour it shows minutes and seconds.
func TestInvoiceCheckShowsSmallDrift(t *testing.T) {
	for _, c := range []struct {
		a, b   int64
		wa, wb string
	}{
		{6000, 9000, "1.67", "2.50"},
		{6, 9, "0:00:06", "0:00:09"},
		{6000, 6010, "1:40:00", "1:40:10"},
		{6000, 6000, "1.67", "1.67"},
	} {
		if a, b := hoursPair(c.a, c.b); a != c.wa || b != c.wb {
			t.Errorf("hoursPair(%d, %d) = %q, %q; want %q, %q", c.a, c.b, a, b, c.wa, c.wb)
		}
	}
}

// Without -o the draft goes to tock's own state dir, never a shared temp
// dir under a guessable name (another user's symlink there would make the
// preview overwrite a file of yours).
func TestInvoiceDraftPath(t *testing.T) {
	te := invoiceEnv(t)
	code, out := te.run(t, "", append([]string{"invoice"}, day...)...)
	want := filepath.Join(te.StateDir, "drafts", "tock-draft-tock.pdf")
	if code != 0 || !strings.Contains(out, "draft: "+want) {
		t.Fatalf("%d %q %s", code, out, te.errb)
	}
	if fi, err := os.Lstat(want); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Fatalf("draft file: %v %v", fi, err)
	}
	// A symlink in its place is replaced, not followed.
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep"), 0o600)
	os.Remove(want)
	if err := os.Symlink(victim, want); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", append([]string{"invoice"}, day...)...); code != 0 {
		t.Fatalf("second preview: %s", te.errb)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatal("preview wrote through a symlink")
	}
}
