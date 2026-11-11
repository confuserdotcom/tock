package app

import (
	"slices"

	"tock/internal/report"
	"tock/internal/store"
)

// Billed is the time of issued invoices over a range: which views are on
// an invoice, and how much of a stretch is.
type Billed struct {
	items []store.BilledItem
	on    map[[2]any]string // (origin, origin id) -> invoice number
}

// Billed reads the issued invoices' items that overlap [fromMS, toMS). With
// Billing off it is empty: nothing shows as billed.
func (a *App) Billed(fromMS, toMS int64) (Billed, error) {
	b := Billed{on: map[[2]any]string{}}
	if !a.cfg.Billing.Enabled {
		return b, nil
	}
	items, err := a.st.BilledItems(fromMS, toMS)
	if err != nil {
		return b, err
	}
	b.items = items
	for _, it := range items {
		k := [2]any{it.Origin, it.OriginID}
		if _, ok := b.on[k]; !ok {
			b.on[k] = it.Number
		}
	}
	return b, nil
}

// Number is the invoice that bills view id, or "".
func (b Billed) Number(id int64) string {
	o, oid := report.ItemOrigin(id)
	return b.on[[2]any{o, oid}]
}

// MS is how much billed time falls in [fromMS, toMS).
func (b Billed) MS(fromMS, toMS int64) int64 {
	var ms int64
	for _, it := range b.items {
		if lo, hi := max(it.StartMS, fromMS), min(it.EndMS, toMS); hi > lo {
			ms += hi - lo
		}
	}
	return ms
}

// Numbers lists, once each and sorted, the invoices billing time in
// [fromMS, toMS).
func (b Billed) Numbers(fromMS, toMS int64) []string {
	var out []string
	for _, it := range b.items {
		if it.StartMS < toMS && it.EndMS > fromMS {
			out = append(out, it.Number)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// BilledOn lists the issued invoices that bill any of the views ids, so a
// relabel can say the invoice stays as issued. None with Billing off.
func (a *App) BilledOn(ids []int64) ([]string, error) {
	if !a.cfg.Billing.Enabled {
		return nil, nil
	}
	by := map[string][]int64{}
	for _, id := range ids {
		o, oid := report.ItemOrigin(id)
		by[o] = append(by[o], oid)
	}
	var out []string
	for o, l := range by {
		for len(l) > 0 {
			n := min(len(l), 500) // under SQLite's variable limit
			got, err := a.st.BilledOn(o, l[:n])
			if err != nil {
				return nil, err
			}
			out = append(out, got...)
			l = l[n:]
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// BilledNote is what a relabel of billed time says, "" when none is.
func BilledNote(numbers []string) string {
	switch len(numbers) {
	case 0:
		return ""
	case 1:
		return "On invoice " + numbers[0] + "; the invoice stays as issued."
	}
	l := numbers[0]
	for _, n := range numbers[1 : len(numbers)-1] {
		l += ", " + n
	}
	return "On invoices " + l + " and " + numbers[len(numbers)-1] + "; the invoices stay as issued."
}
