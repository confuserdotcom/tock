package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"tock/internal/classify"
)

func TestItemOrigin(t *testing.T) {
	for _, c := range []struct {
		id     int64
		origin string
		oid    int64
	}{
		{42, "span", 42},
		{-7, "away", 7},
		{-(classify.MeetingIDBase + 3), "meeting", 3},
		{-(classify.RemoteIDBase + 9), "remote", 9},
		{-(classify.EditIDBase + 5), "edit", 5},
	} {
		o, id := ItemOrigin(c.id)
		if o != c.origin || id != c.oid {
			t.Errorf("ItemOrigin(%d) = %s %d, want %s %d", c.id, o, id, c.origin, c.oid)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	for bp, want := range map[int64]string{2000: "20", 550: "5.5", 2025: "20.25", 0: "0", 10000: "100"} {
		if got := FormatPercent(bp); got != want {
			t.Errorf("FormatPercent(%d) = %q, want %q", bp, got, want)
		}
	}
}

func testInvoice() Invoice {
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	return Invoice{
		Number: "2026-0004", Project: "Acme site",
		Issuer:   Party{Name: "Ada Lovelace", Address: []string{"1 Way", "London"}, TaxID: "GB123"},
		Client:   Party{Name: "Babbage & Co", Address: []string{"2 Road", "Paris"}, TaxID: "FR999"},
		Currency: "EUR", Note: "Due in 30 days.",
		Issued:     time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
		PeriodFrom: day, PeriodTo: day.AddDate(0, 0, 26),
		MinorUnits: 2, RateMinor: 9550,
		Lines:         []BillLine{{Day: "2026-10-05", SecondsRaw: 11900, SecondsBilled: 12000, AmountMinor: 31833}},
		SecondsBilled: 12000, SubtotalMinor: 31833,
		TaxLabel: "VAT", TaxBP: 2000, TaxNote: "Reverse charge", TaxMinor: 6367, TotalMinor: 38200,
		Supersedes: "2026-0003",
	}
}

func TestWriteInvoicePDF(t *testing.T) {
	var b bytes.Buffer
	if err := WriteInvoicePDF(&b, testInvoice()); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, want := range []string{"%PDF-1.4", "(INVOICE) Tj", "(Ada Lovelace) Tj", "(GB123) Tj", "(Babbage & Co) Tj",
		"(Paris) Tj", "(FR999) Tj", "(2026-0004) Tj", "(Mon 2026-10-05) Tj", "(3.33) Tj", "(95.50) Tj", "(318.33 EUR) Tj",
		"(VAT 20%) Tj", "(63.67 EUR) Tj", "(382.00 EUR) Tj", "(Reverse charge) Tj", "(Replaces invoice 2026-0003) Tj",
		"(Due in 30 days.) Tj"} {
		if !strings.Contains(s, want) {
			t.Errorf("pdf lacks %q", want)
		}
	}
	if strings.Contains(s, "DRAFT") {
		t.Error("issued invoice marked DRAFT")
	}

	d := testInvoice()
	d.Draft, d.Number, d.TaxBP, d.TaxLabel, d.TaxMinor, d.TotalMinor = true, "", 0, "", 0, 31833
	b.Reset()
	if err := WriteInvoicePDF(&b, d); err != nil {
		t.Fatal(err)
	}
	s = b.String()
	if !strings.Contains(s, "(DRAFT) Tj") || strings.Contains(s, "VAT") || !strings.Contains(s, "(318.33 EUR) Tj") {
		t.Errorf("draft pdf:\n%s", s)
	}
}
