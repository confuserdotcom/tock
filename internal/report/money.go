package report

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Issued invoice amounts are int64 minor units (cents for minor_units 2).
// Nothing here uses float64 except RateMinor, which reads a rate from
// rules.toml through its shortest decimal form, so the conversion is exact.

// MaxMinorUnits is the most decimals a currency amount may have.
const MaxMinorUnits = 3

// RoundMode is how a day's time is rounded to the step.
type RoundMode string

const (
	// RoundNearest rounds to the nearest step; a half step rounds up.
	RoundNearest RoundMode = "nearest"
	// RoundUp rounds any part of a step up to a whole step.
	RoundUp RoundMode = "up"
)

// ParseRoundMode reads [invoice] round_mode; "" is nearest.
func ParseRoundMode(s string) (RoundMode, error) {
	switch RoundMode(s) {
	case "", RoundNearest:
		return RoundNearest, nil
	case RoundUp:
		return RoundUp, nil
	}
	return "", fmt.Errorf("report: round mode %q, want nearest or up", s)
}

// RoundSeconds rounds s to a multiple of step. A step of 1 s or less leaves
// s as it is.
func RoundSeconds(s, step int64, mode RoundMode) int64 {
	if step <= 1 || s <= 0 {
		return s
	}
	q, r := s/step, s%step
	switch {
	case r == 0:
	case mode == RoundUp:
		q++
	case 2*r >= step:
		q++
	}
	return q * step
}

// RateMinor converts an hourly rate to minor units. It fails when the rate
// is negative or has more decimals than minorUnits, so every accepted rate
// converts exactly.
func RateMinor(rate float64, minorUnits int) (int64, error) {
	if minorUnits < 0 || minorUnits > MaxMinorUnits {
		return 0, fmt.Errorf("report: minor units %d, want 0 to %d", minorUnits, MaxMinorUnits)
	}
	if !(rate >= 0) {
		return 0, fmt.Errorf("report: rate %v is negative", rate)
	}
	s := strconv.FormatFloat(rate, 'f', -1, 64)
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > minorUnits {
		return 0, fmt.Errorf("report: rate %s has more than %d decimals", s, minorUnits)
	}
	v, err := strconv.ParseInt(whole+frac+strings.Repeat("0", minorUnits-len(frac)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("report: rate %s: %w", s, err)
	}
	return v, nil
}

// divRound is n/d rounded half away from zero, d > 0, with no overflow.
func divRound(n *big.Int, d int64) int64 {
	bd := big.NewInt(d)
	q, r := new(big.Int).QuoRem(n, bd, new(big.Int))
	if r.Sign() != 0 && new(big.Int).Abs(new(big.Int).Lsh(r, 1)).Cmp(bd) >= 0 {
		q.Add(q, big.NewInt(int64(n.Sign())))
	}
	return q.Int64()
}

// AmountMinor is the price of seconds at rateMinor per hour, rounded half
// away from zero.
func AmountMinor(seconds, rateMinor int64) int64 {
	n := new(big.Int).Mul(big.NewInt(seconds), big.NewInt(rateMinor))
	return divRound(n, 3600)
}

// TaxMinor is the tax on subtotal at bp basis points (2000 = 20 %),
// rounded half away from zero.
func TaxMinor(subtotal, bp int64) int64 {
	n := new(big.Int).Mul(big.NewInt(subtotal), big.NewInt(bp))
	return divRound(n, 10000)
}

// FormatMinor writes v minor units as a decimal: 31833, 2 is "318.33".
func FormatMinor(v int64, minorUnits int) string {
	sign := ""
	u := uint64(v)
	if v < 0 {
		sign, u = "-", uint64(-v)
	}
	s := strconv.FormatUint(u, 10)
	if minorUnits <= 0 {
		return sign + s
	}
	if len(s) <= minorUnits {
		s = strings.Repeat("0", minorUnits-len(s)+1) + s
	}
	return sign + s[:len(s)-minorUnits] + "." + s[len(s)-minorUnits:]
}

// InvoiceYear is the local year of an invoice issued at issuedMS: the year
// its default number carries.
func InvoiceYear(issuedMS int64, loc *time.Location) int {
	return time.UnixMilli(issuedMS).In(loc).Year()
}

// NextInvoiceNumber is the default number after maxSeq, the highest
// sequence already used in year (void invoices included; 0 = none):
// "YYYY-NNNN", at least four digits.
func NextInvoiceNumber(year, maxSeq int) (string, int) {
	seq := maxSeq + 1
	return fmt.Sprintf("%04d-%04d", year, seq), seq
}

// ParseInvoiceNumber reads a default number back; ok is false for any
// other form (a number given with --number).
func ParseInvoiceNumber(s string) (year, seq int, ok bool) {
	y, n, found := strings.Cut(s, "-")
	if !found || len(y) != 4 || len(n) < 4 || !digits(y) || !digits(n) {
		return 0, 0, false
	}
	year, _ = strconv.Atoi(y)
	seq, err := strconv.Atoi(n)
	if err != nil || seq == 0 || (len(n) > 4 && n[0] == '0') {
		return 0, 0, false
	}
	return year, seq, true
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// BillPolicy is how an invoice turns time into money: the rounding step
// in seconds, its mode, and the rate in minor units per hour.
type BillPolicy struct {
	StepS     int64
	Mode      RoundMode
	RateMinor int64
}

// BillLine is one local day on an invoice.
type BillLine struct {
	Day                       string // local date, 2006-01-02
	SecondsRaw, SecondsBilled int64
	AmountMinor               int64
}

// BillLines splits the rows of project at local midnights in loc and
// returns one line per local day, in date order. Each day's time is summed
// in milliseconds, truncated to seconds, rounded per p and priced.
func BillLines(rows []ExportRow, project string, loc *time.Location, p BillPolicy) []BillLine {
	msByDay := map[string]int64{}
	var days []string
	add := func(day string, ms int64) {
		if _, ok := msByDay[day]; !ok {
			days = append(days, day)
		}
		msByDay[day] += ms
	}
	for _, r := range rows {
		if r.Project != project || !r.End.After(r.Start) {
			continue
		}
		start, end := r.Start.In(loc), r.End.In(loc)
		for start.Before(end) {
			next := time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, loc)
			stop := end
			if next.Before(end) {
				stop = next
			}
			add(start.Format("2006-01-02"), stop.UnixMilli()-start.UnixMilli())
			start = stop
		}
	}
	slices.Sort(days)
	out := make([]BillLine, len(days))
	for i, d := range days {
		raw := msByDay[d] / 1000
		billed := RoundSeconds(raw, p.StepS, p.Mode)
		out[i] = BillLine{Day: d, SecondsRaw: raw, SecondsBilled: billed, AmountMinor: AmountMinor(billed, p.RateMinor)}
	}
	return out
}

// SumLines is the billed time and the subtotal of lines.
func SumLines(lines []BillLine) (seconds, subtotalMinor int64) {
	for _, l := range lines {
		seconds += l.SecondsBilled
		subtotalMinor += l.AmountMinor
	}
	return seconds, subtotalMinor
}
