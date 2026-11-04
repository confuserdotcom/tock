package app

import (
	"testing"

	"tock/internal/classify"
	"tock/internal/store"
)

// rowsFor keeps every row that reaches the day or the pause before it,
// a long one included, and drops the rest.
func TestRowsFor(t *testing.T) {
	end := func(ms int64) *int64 { return &ms }
	day, afk := int64(1_000_000_000), int64(classify.MaxAFKMS)
	rows := []store.SpanRow{
		{ID: 1, Start: 0, End: end(10)},                          // long before
		{ID: 2, Start: 20, End: end(day + 5)},                    // a long span into the day
		{ID: 3, Start: day - afk - 50, End: end(day - afk - 10)}, // ends before the pause window
		{ID: 4, Start: day - afk/2, End: end(day - 1)},           // in the pause window
		{ID: 5, Start: day + 10, End: end(day + 20)},
		{ID: 6, Start: 2 * day, End: nil}, // after the day, open
	}
	got := rowsFor(rows, rowEnds(rows, 3*day), day, day+100)
	var ids []int64
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	// 3 stays: it lies between two rows that are needed.
	if want := []int64{2, 3, 4, 5}; len(ids) != len(want) || ids[0] != 2 || ids[len(ids)-1] != 5 {
		t.Fatalf("rows %v, want %v", ids, want)
	}
}
