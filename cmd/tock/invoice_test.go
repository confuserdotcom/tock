package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rateRules = "name = \"tock\"\nrate = 60\n"

func TestInvoicePDF(t *testing.T) {
	te := newTestEnv(t, strings.Replace(testRules, "name = \"tock\"\n", rateRules, 1))
	te.seed(t, daySpans()...)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"),
		[]byte("[invoice]\nname = \"Ada Lovelace\"\naddress = [\"London\"]\ncurrency = \"EUR\"\nnote = \"Due in 30 days.\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "inv.pdf")
	code, out := te.run(t, "", "invoice", "--project", "to", "--from", "2026-10-01", "--to", "2026-10-31",
		"--client", "Babbage & Co", "-o", path)
	if code != 0 || out != "wrote "+path+": tock, 1 days, 1.67 h, 100.00 EUR\n" {
		t.Fatalf("code %d out %q err %s", code, out, te.errb)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"%PDF-1.4", "(INVOICE) Tj", "(Ada Lovelace) Tj", "(Babbage & Co) Tj",
		"(202610-tock) Tj", "(Sat 2026-10-10) Tj", "(1.67) Tj", "(100.00 EUR) Tj", "(Due in 30 days.) Tj"} {
		if !strings.Contains(s, want) {
			t.Errorf("pdf lacks %q", want)
		}
	}
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
		{[]string{"invoice", "--project", "nope"}, 2, "nope"},
		{[]string{"invoice", "--project", "tock", "--from", "2026-10-01", "--to", "2026-10-31"}, 1, "has no rate"},
		{[]string{"invoice", "--project", "tock", "--from", "2026-11-02", "--to", "2026-11-01"}, 2, "--from is after --to"},
	} {
		code, _ := te.run(t, "", append(c.args, "-o", filepath.Join(dir, "x.pdf"))...)
		if code != c.code || !strings.Contains(te.errb.String(), c.err) {
			t.Errorf("%v: code %d err %q, want %d %q", c.args, code, te.errb, c.code, c.err)
		}
	}
	te2 := newTestEnv(t, strings.Replace(testRules, "name = \"tock\"\n", rateRules, 1))
	te2.seed(t, daySpans()...)
	if code, _ := te2.run(t, "", "invoice", "--project", "tock", "-o", filepath.Join(dir, "y.pdf")); code != 1 ||
		!strings.Contains(te2.errb.String(), "no time on tock from 2026-09-01 to 2026-09-30") {
		t.Errorf("empty month: code %d err %q", code, te2.errb)
	}
}
