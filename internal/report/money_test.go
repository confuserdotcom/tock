package report

import (
	"testing"
	"time"
)

func TestRoundSeconds(t *testing.T) {
	const m15 = 15 * 60
	for _, c := range []struct {
		name    string
		s, step int64
		mode    RoundMode
		want    int64
	}{
		{"no rounding", 3661, 1, RoundNearest, 3661},
		{"zero step is no rounding", 3661, 0, RoundUp, 3661},
		{"61 min up", 61 * 60, m15, RoundUp, 75 * 60},
		{"61 min nearest", 61 * 60, m15, RoundNearest, 60 * 60},
		{"7m30s nearest rounds half up", 450, m15, RoundNearest, m15},
		{"7m29s nearest", 449, m15, RoundNearest, 0},
		{"exact step up stays", 60 * 60, m15, RoundUp, 60 * 60},
		{"1s up", 1, m15, RoundUp, m15},
		{"zero stays zero up", 0, m15, RoundUp, 0},
		{"6 min nearest", 9 * 60, 6 * 60, RoundNearest, 12 * 60},
		{"odd step below half", 3, 7, RoundNearest, 0},
		{"odd step above half", 4, 7, RoundNearest, 7},
	} {
		if got := RoundSeconds(c.s, c.step, c.mode); got != c.want {
			t.Errorf("%s: RoundSeconds(%d, %d, %s) = %d, want %d", c.name, c.s, c.step, c.mode, got, c.want)
		}
	}
}

func TestRateMinor(t *testing.T) {
	for _, c := range []struct {
		rate  float64
		units int
		want  int64
		ok    bool
	}{
		{95.50, 2, 9550, true},
		{95.5, 2, 9550, true},
		{95, 0, 95, true},
		{95, 3, 95000, true},
		{0.1, 2, 10, true},
		{0.29, 2, 29, true}, // 0.29*100 is 28.999… in float; the decimal path is exact
		{1.005, 3, 1005, true},
		{100000, 3, 100000000, true},
		{95.555, 2, 0, false}, // more decimals than minor units
		{95.5, 0, 0, false},
		{-1, 2, 0, false},
		{10, 4, 0, false}, // minor units out of range
		{10, -1, 0, false},
	} {
		got, err := RateMinor(c.rate, c.units)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("RateMinor(%v, %d) = %d, %v; want %d, ok=%v", c.rate, c.units, got, err, c.want, c.ok)
		}
	}
}

func TestAmountMinor(t *testing.T) {
	for _, c := range []struct {
		seconds, rate, want int64
	}{
		{3*3600 + 20*60, 9550, 31833}, // 318.333… → 318.33
		{3600, 9550, 9550},
		{1800, 1, 1}, // 0.5 rounds away from zero
		{1799, 1, 0}, // 0.4997
		{5400, 3, 5}, // 4.5 → 5
		{0, 9550, 0},
		{10 * 365 * 86400, 100000000, 8760000000000}, // no overflow at the largest rate
	} {
		if got := AmountMinor(c.seconds, c.rate); got != c.want {
			t.Errorf("AmountMinor(%d, %d) = %d, want %d", c.seconds, c.rate, got, c.want)
		}
	}
}

func TestTaxMinor(t *testing.T) {
	for _, c := range []struct {
		subtotal, bp, want int64
	}{
		{31833, 2000, 6367}, // 6366.6 → 6367
		{31833, 0, 0},
		{100, 2050, 21}, // 20.5 → 21
		{100, 2049, 20}, // 20.49
		{1, 5000, 1},    // 0.5 → 1
	} {
		if got := TaxMinor(c.subtotal, c.bp); got != c.want {
			t.Errorf("TaxMinor(%d, %d) = %d, want %d", c.subtotal, c.bp, got, c.want)
		}
	}
	if sub, tax := int64(31833), TaxMinor(31833, 2000); sub+tax != 38200 {
		t.Errorf("total %d, want 38200", sub+tax)
	}
}

func TestFormatMinor(t *testing.T) {
	for _, c := range []struct {
		v     int64
		units int
		want  string
	}{
		{31833, 2, "318.33"},
		{5, 2, "0.05"},
		{0, 2, "0.00"},
		{31833, 0, "31833"},
		{1005, 3, "1.005"},
		{-250, 2, "-2.50"},
	} {
		if got := FormatMinor(c.v, c.units); got != c.want {
			t.Errorf("FormatMinor(%d, %d) = %q, want %q", c.v, c.units, got, c.want)
		}
	}
}

func TestInvoiceNumbering(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tz data:", err)
	}
	lastSecond := time.Date(2026, 12, 31, 23, 59, 59, 0, paris).UnixMilli()
	newYear := time.Date(2027, 1, 1, 0, 0, 0, 0, paris).UnixMilli()
	if y := InvoiceYear(lastSecond, paris); y != 2026 {
		t.Errorf("31 Dec 23:59:59 local: year %d, want 2026", y)
	}
	if y := InvoiceYear(newYear, paris); y != 2027 {
		t.Errorf("1 Jan 00:00 local: year %d, want 2027", y)
	}
	// 23:30 UTC on 31 Dec is already the new year in Paris.
	if y := InvoiceYear(time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC).UnixMilli(), paris); y != 2027 {
		t.Errorf("year from UTC, not local: %d", y)
	}

	for _, c := range []struct {
		year, maxSeq int
		want         string
		seq          int
	}{
		{2027, 0, "2027-0001", 1}, // first of the year
		{2026, 1, "2026-0002", 2},
		{2026, 2, "2026-0003", 3}, // after a void 2026-0002: never reused
		{2026, 9999, "2026-10000", 10000},
	} {
		got, seq := NextInvoiceNumber(c.year, c.maxSeq)
		if got != c.want || seq != c.seq {
			t.Errorf("NextInvoiceNumber(%d, %d) = %q, %d; want %q, %d", c.year, c.maxSeq, got, seq, c.want, c.seq)
		}
	}

	for _, c := range []struct {
		in        string
		year, seq int
		ok        bool
	}{
		{"2026-0004", 2026, 4, true},
		{"2026-10000", 2026, 10000, true},
		{"2026-004", 0, 0, false},
		{"2026-0000", 0, 0, false},
		{"202610-acme", 0, 0, false},
		{"ACME-1", 0, 0, false},
	} {
		y, s, ok := ParseInvoiceNumber(c.in)
		if y != c.year || s != c.seq || ok != c.ok {
			t.Errorf("ParseInvoiceNumber(%q) = %d, %d, %v", c.in, y, s, ok)
		}
	}
}

func TestBillLines(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tz data:", err)
	}
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, paris) }
	row := func(project string, from, to time.Time) ExportRow {
		return ExportRow{Project: project, Start: from, End: to}
	}
	rows := []ExportRow{
		row("Acme", at(5, 9, 0), at(5, 10, 1)),   // 61 min
		row("Other", at(5, 10, 0), at(5, 12, 0)), // other project: skipped
		row("Acme", at(6, 23, 30), at(7, 0, 30)), // crosses midnight: 30 + 30
		row("Acme", at(7, 9, 0), at(7, 9, 7)),    // 7 min
		// 25 Oct 2026 is the autumn DST change in Paris: 02:00–03:00 runs
		// twice, so 00:00 to 04:00 local is five real hours.
		row("Acme", at(25, 0, 0), at(25, 4, 0)),
	}
	p := BillPolicy{StepS: 15 * 60, Mode: RoundUp, RateMinor: 9550}
	got := BillLines(rows, "Acme", paris, p)
	want := []BillLine{
		{Day: "2026-10-05", SecondsRaw: 61 * 60, SecondsBilled: 75 * 60, AmountMinor: AmountMinor(75*60, 9550)},
		{Day: "2026-10-06", SecondsRaw: 30 * 60, SecondsBilled: 30 * 60, AmountMinor: AmountMinor(30*60, 9550)},
		{Day: "2026-10-07", SecondsRaw: 37 * 60, SecondsBilled: 45 * 60, AmountMinor: AmountMinor(45*60, 9550)},
		{Day: "2026-10-25", SecondsRaw: 5 * 3600, SecondsBilled: 5 * 3600, AmountMinor: 5 * 9550},
	}
	if len(got) != len(want) {
		t.Fatalf("lines %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	secs, sub := SumLines(got)
	if secs != (75+30+45)*60+5*3600 {
		t.Errorf("billed seconds %d", secs)
	}
	var wantSub int64
	for _, l := range want {
		wantSub += l.AmountMinor
	}
	if sub != wantSub {
		t.Errorf("subtotal %d, want %d", sub, wantSub)
	}

	// Rounding is per day, not per row: two 5-minute rows on one day bill
	// 15 minutes, not 30.
	two := []ExportRow{row("Acme", at(8, 9, 0), at(8, 9, 5)), row("Acme", at(8, 14, 0), at(8, 14, 5))}
	if l := BillLines(two, "Acme", paris, p); len(l) != 1 || l[0].SecondsBilled != 15*60 {
		t.Errorf("per-day rounding: %+v", l)
	}

	// Sub-second remainders add up per day before truncating to seconds.
	half := func(d, h, m, s int) time.Time { return time.Date(2026, 10, d, h, m, s, 500e6, paris) }
	ms := []ExportRow{row("Acme", at(9, 9, 0), half(9, 9, 0, 0)), row("Acme", at(9, 10, 0), half(9, 10, 0, 0))}
	if l := BillLines(ms, "Acme", paris, BillPolicy{StepS: 1, RateMinor: 3600}); len(l) != 1 || l[0].SecondsRaw != 1 {
		t.Errorf("ms sum: %+v", l)
	}
}

func TestParseRoundMode(t *testing.T) {
	for in, want := range map[string]RoundMode{"": RoundNearest, "nearest": RoundNearest, "up": RoundUp} {
		if got, err := ParseRoundMode(in); err != nil || got != want {
			t.Errorf("ParseRoundMode(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseRoundMode("down"); err == nil {
		t.Error("down accepted")
	}
}
