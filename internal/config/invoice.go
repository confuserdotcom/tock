package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Invoice is the [invoice] section: who the invoices tock invoice writes
// are from.
type Invoice struct {
	Name     string
	Address  []string // lines
	Currency string   // printed after amounts; "" = none
	Note     string   // printed at the bottom, e.g. payment terms
	TaxID    string   // the issuer's tax or VAT number; "" = none

	// RoundStepS rounds each day's billed time to this many seconds
	// (round = "15m"); 1 = no rounding. RoundMode is "nearest" (a half
	// step rounds up) or "up".
	RoundStepS int64
	RoundMode  string
	// MinorUnits is how many decimals amounts have, 0 to 3.
	MinorUnits int

	// TaxLabel ("VAT"), TaxBP (tax_rate in basis points: "20" is 2000; 0
	// = no tax line) and TaxNote (e.g. reverse charge).
	TaxLabel string
	TaxBP    int64
	TaxNote  string

	// Clients are [[invoice.client]] details, found by a project's client
	// name.
	Clients []InvoiceClient
}

// InvoiceClient is one [[invoice.client]]: who an invoice is to.
type InvoiceClient struct {
	Name    string
	Address []string
	TaxID   string
}

// DefaultInvoice is [invoice] when config.toml sets nothing: no rounding,
// two decimals, no tax.
func DefaultInvoice() Invoice {
	return Invoice{RoundStepS: 1, RoundMode: "nearest", MinorUnits: 2}
}

// Client returns the [[invoice.client]] named name, ignoring case.
func (inv Invoice) Client(name string) (InvoiceClient, bool) {
	for _, c := range inv.Clients {
		if strings.EqualFold(c.Name, strings.TrimSpace(name)) {
			return c, true
		}
	}
	return InvoiceClient{}, false
}

// MaxRoundStepS is the longest [invoice] round.
const MaxRoundStepS = 24 * 3600

type invoiceTOML struct {
	Name       *string             `toml:"name"`
	Address    *[]string           `toml:"address"`
	Currency   *string             `toml:"currency"`
	Note       *string             `toml:"note"`
	TaxID      *string             `toml:"tax_id"`
	Round      *string             `toml:"round"`
	RoundMode  *string             `toml:"round_mode"`
	MinorUnits *int                `toml:"minor_units"`
	TaxLabel   *string             `toml:"tax_label"`
	TaxRate    *string             `toml:"tax_rate"`
	TaxNote    *string             `toml:"tax_note"`
	Client     []invoiceClientTOML `toml:"client"`
}

type invoiceClientTOML struct {
	Name    *string   `toml:"name"`
	Address *[]string `toml:"address"`
	TaxID   *string   `toml:"tax_id"`
}

// decodeInvoice reads the [invoice] section.
func decodeInvoice(f invoiceTOML, c *Invoice) error {
	for _, t := range []struct {
		key string
		v   *string
		dst *string
		max int
	}{{"name", f.Name, &c.Name, 100}, {"currency", f.Currency, &c.Currency, 8}, {"note", f.Note, &c.Note, 300},
		{"tax_id", f.TaxID, &c.TaxID, 50}, {"tax_label", f.TaxLabel, &c.TaxLabel, 20}, {"tax_note", f.TaxNote, &c.TaxNote, 200}} {
		if t.v == nil {
			continue
		}
		v, err := invoiceText(t.key, *t.v, t.max)
		if err != nil {
			return err
		}
		*t.dst = v
	}
	if f.Address != nil {
		a, err := invoiceAddress("address", *f.Address)
		if err != nil {
			return err
		}
		c.Address = a
	}
	if f.Round != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*f.Round))
		if err != nil || d < time.Second || d > MaxRoundStepS*time.Second || d%time.Second != 0 {
			return fmt.Errorf("[invoice] round = %q, want whole seconds from 1s to 24h, e.g. \"6m\" or \"15m\"", *f.Round)
		}
		c.RoundStepS = int64(d / time.Second)
	}
	if f.RoundMode != nil {
		switch m := strings.TrimSpace(*f.RoundMode); m {
		case "nearest", "up":
			c.RoundMode = m
		default:
			return fmt.Errorf("[invoice] round_mode = %q, want \"nearest\" or \"up\"", *f.RoundMode)
		}
	}
	if f.MinorUnits != nil {
		if u := *f.MinorUnits; u < 0 || u > 3 {
			return fmt.Errorf("[invoice] minor_units = %d, want 0 to 3", u)
		}
		c.MinorUnits = *f.MinorUnits
	}
	if f.TaxRate != nil {
		bp, err := ParseTaxRate(*f.TaxRate)
		if err != nil {
			return fmt.Errorf("[invoice] tax_rate: %w", err)
		}
		c.TaxBP = bp
	}
	c.Clients = nil
	for i, fc := range f.Client {
		if fc.Name == nil || strings.TrimSpace(*fc.Name) == "" {
			return fmt.Errorf("[[invoice.client]] #%d has no name", i+1)
		}
		var cl InvoiceClient
		var err error
		if cl.Name, err = invoiceText("client name", *fc.Name, 100); err != nil {
			return err
		}
		if _, dup := c.Client(cl.Name); dup {
			return fmt.Errorf("[[invoice.client]] %q is listed twice", cl.Name)
		}
		if fc.Address != nil {
			if cl.Address, err = invoiceAddress("client address", *fc.Address); err != nil {
				return err
			}
		}
		if fc.TaxID != nil {
			if cl.TaxID, err = invoiceText("client tax_id", *fc.TaxID, 50); err != nil {
				return err
			}
		}
		c.Clients = append(c.Clients, cl)
	}
	return nil
}

func invoiceText(key, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > max {
		return "", fmt.Errorf("[invoice] %s is longer than %d characters", key, max)
	}
	return v, nil
}

func invoiceAddress(key string, lines []string) ([]string, error) {
	if len(lines) > 6 {
		return nil, fmt.Errorf("[invoice] %s has more than 6 lines", key)
	}
	var out []string
	for _, l := range lines {
		if len([]rune(l)) > 100 {
			return nil, fmt.Errorf("[invoice] %s has a line longer than 100 characters", key)
		}
		out = append(out, strings.TrimSpace(l))
	}
	return out, nil
}

// ParseTaxRate reads a tax rate in percent, a decimal string with at most
// two decimals from "0" to "100", as basis points: "20" is 2000, "5.5" is
// 550.
func ParseTaxRate(s string) (int64, error) {
	s = strings.TrimSpace(s)
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 || !allDigits(whole) || (frac != "" && !allDigits(frac)) || len(whole) > 3 {
		return 0, fmt.Errorf("%q, want a percentage such as \"20\" or \"5.5\" (at most two decimals)", s)
	}
	bp, _ := strconv.ParseInt(whole+frac+strings.Repeat("0", 2-len(frac)), 10, 64)
	if bp > 10000 {
		return 0, fmt.Errorf("%q is over 100", s)
	}
	return bp, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
